# Migration Strategy & Dual Recovery Risk Analysis

## Critical Findings

### 1. **DUPLICATE TIMER RECOVERY RISK** ⚠️

There are **TWO** separate timer recovery mechanisms that will conflict:

#### Existing Recovery: `RecoverPendingReminders()`

**Location:** `internal/flow/scheduler_tool.go:93`
**Called from:** `internal/api/api.go:567` (during server startup)
**Scope:** Daily prompt reminders ONLY
**Method:** Reads state from `DataKeyDailyPromptPending`, recreates timers in-memory
**Storage:** Uses state_data JSON in flow_states table

#### New Recovery: Database-backed timer persistence

**Location:** Not yet implemented (planned for PHASE 4)
**Scope:** ALL timers (scheduled_prompt, feedback, state transitions, reminders, etc.)
**Method:** Reads from `active_timers` table, recreates all timers
**Storage:** Uses dedicated `active_timers` table

#### **THE PROBLEM:**

When both systems are active, daily prompt reminders will be recovered **TWICE**:

1. Once by `RecoverPendingReminders()` reading from `flow_states.state_data`
2. Once by new system reading from `active_timers` table

**Result:** Users receive duplicate reminders! ❌

---

### 2. **PRODUCTION DATABASE MIGRATION SAFETY** ⚠️

#### Current Migration Approach: UNSAFE for Production

**Current implementation in `migrations_sqlite.sql` and `migrations_postgres.sql`:**

```sql
-- Lines added to existing files:
CREATE TABLE IF NOT EXISTS active_timers (...);
ALTER TABLE conversation_participants ADD COLUMN timezone TEXT;  -- ❌ MISSING!
```

**CRITICAL ISSUES:**

1. **Missing `ALTER TABLE` for timezone column**
   - We added `timezone TEXT` to the `CREATE TABLE IF NOT EXISTS conversation_participants` statement
   - BUT: If the table already exists in production, `CREATE TABLE IF NOT EXISTS` is skipped
   - The `timezone` column will NEVER be added to existing production databases!
   - This will cause **runtime errors** when code tries to read/write timezone

2. **No migration versioning**
   - Current approach: Run entire SQL file on every startup
   - Uses `CREATE TABLE IF NOT EXISTS` (idempotent for new tables)
   - Uses `CREATE INDEX IF NOT EXISTS` (idempotent for indexes)
   - But CANNOT use `ALTER TABLE IF NOT EXISTS` (doesn't exist in SQL)

3. **No rollback mechanism**
   - If migration fails, database is in unknown state
   - No way to roll back to previous schema version

---

## Solution: Comprehensive Migration Strategy

### Phase 1: Fix Schema Migration (IMMEDIATE)

#### Option A: Add Explicit `ALTER TABLE` Statements (Quick Fix)

```sql
-- Add at the end of migrations_sqlite.sql and migrations_postgres.sql

-- Add timezone column if it doesn't exist (production compatibility)
-- SQLite doesn't support IF NOT EXISTS for ALTER TABLE, so we need error handling
-- This will error if column exists, but that's OK (handled in Go code)
ALTER TABLE conversation_participants ADD COLUMN timezone TEXT;

-- Note: This will fail if column exists, need to handle in Go migration runner
```

Then update Go migration code to ignore "duplicate column" errors:

```go
// In sqlite.go and postgres.go NewStore functions:
if _, err := db.Exec(sqliteMigrations); err != nil {
    // Ignore "duplicate column" errors for ALTER TABLE
    if !strings.Contains(err.Error(), "duplicate column") {
        return nil, fmt.Errorf("failed to run migrations: %w", err)
    }
    slog.Debug("Migration warning (safe to ignore)", "error", err)
}
```

#### Option B: Implement Proper Migration Versioning (Better, but more work)

Create new table to track migration versions:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);
```

Split migrations into versioned files:

- `001_initial_schema.sql` - Original tables
- `002_add_active_timers.sql` - New active_timers table
- `003_add_timezone_column.sql` - ALTER TABLE for timezone

Update Go code to track and apply only new migrations.

**Recommendation:** Start with Option A for immediate fix, plan Option B for future.

---

### Phase 2: Resolve Duplicate Recovery (CRITICAL)

#### Strategy: Gradual Migration from Old to New System

**Step 1: Make systems mutually exclusive**

```go
// In internal/api/api.go, modify recoverSchedulerReminders():

func (s *Server) recoverSchedulerReminders(ctx context.Context) error {
    // NEW: Check if database-backed timer recovery is available
    timers, err := s.st.ListTimersByFlowType(models.FlowTypeConversation)
    if err == nil && len(timers) > 0 {
        slog.Info("Database-backed timer recovery available, skipping legacy RecoverPendingReminders",
            "timerCount", len(timers))
        return nil // Skip old recovery, new system will handle it
    }
    
    // Fall back to old recovery if no timers in database
    slog.Info("Using legacy RecoverPendingReminders (no timers in database)")
    
    // ... existing RecoverPendingReminders logic ...
}
```

**Step 2: Update SchedulerTool to persist timers**
When `SchedulerTool.RecoverPendingReminders()` creates timers, also save to database:

```go
// In scheduler_tool.go, after creating timer:
timerID, err := st.timer.ScheduleAfter(delay, callback)

// NEW: Persist to database
timerRecord := models.TimerRecord{
    ID:            timerID,
    ParticipantID: participantID,
    FlowType:      models.FlowTypeConversation,
    TimerType:     "once",
    CallbackType:  models.CallbackTypeReminder,
    // ... other fields ...
}
if err := st.store.SaveTimer(ctx, &timerRecord); err != nil {
    slog.Warn("Failed to persist timer to database", "error", err)
}
```

**Step 3: Migration path**

1. Deploy code with Option A schema fix + mutual exclusion logic
2. Old system continues to work (reads from flow_states)
3. New timers created after deployment are persisted to active_timers
4. On next restart, new system finds timers in active_timers → uses new recovery
5. Gradually all timers migrate to new system

---

### Phase 3: Cleanup Legacy System (FUTURE)

After confirming new system works in production:

1. Remove `RecoverPendingReminders()` method
2. Remove `DataKeyDailyPromptPending` state data
3. Clean up flow_states table data
4. Update tests to use new system

---

## Implementation Checklist

### Immediate (Before Next Deployment)

- [ ] Add `ALTER TABLE conversation_participants ADD COLUMN timezone TEXT;` to migration files
- [ ] Update Go migration runner to handle duplicate column errors gracefully
- [ ] Test migration on copy of production database
- [ ] Verify timezone column added to existing tables

### Phase 2 (Next Sprint)

- [ ] Add mutual exclusion logic to `recoverSchedulerReminders()`
- [ ] Update `SchedulerTool` to persist timers on creation
- [ ] Add timer deletion on timer execution/cancellation
- [ ] Test dual-system compatibility

### Phase 3 (After Production Validation)

- [ ] Implement proper migration versioning (schema_migrations table)
- [ ] Create migration tool for versioned migrations
- [ ] Document migration process

### Phase 4 (After Dual System Validated)

- [ ] Remove legacy `RecoverPendingReminders()` code
- [ ] Clean up old state data
- [ ] Update documentation

---

## Testing Strategy

### Pre-Production Testing

1. **Schema Migration Test:**

   ```bash
   # Test on existing database
   sqlite3 test.db < internal/store/migrations_sqlite.sql
   # Verify no errors, timezone column added
   sqlite3 test.db "PRAGMA table_info(conversation_participants);"
   ```

2. **Dual Recovery Test:**
   - Create database with old state_data records
   - Add active_timers records
   - Start server, verify only ONE timer created per reminder

### Production Rollout

1. Deploy with mutual exclusion logic
2. Monitor logs for "Using legacy RecoverPendingReminders" vs "Database-backed timer recovery"
3. Verify no duplicate reminders reported
4. Wait for full migration cycle (all timers recreated)
5. Confirm 100% on new system

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Duplicate reminders | HIGH | Mutual exclusion logic |
| Missing timezone column | CRITICAL | ALTER TABLE + error handling |
| Migration failure | MEDIUM | IF NOT EXISTS, error handling |
| Data loss on rollback | LOW | No data deletion, only additions |
| Performance degradation | LOW | Indexed queries, tested |

---

## Files Requiring Immediate Changes

1. **`internal/store/migrations_sqlite.sql`** - Add ALTER TABLE
2. **`internal/store/migrations_postgres.sql`** - Add ALTER TABLE  
3. **`internal/store/sqlite.go`** - Update migration error handling
4. **`internal/store/postgres.go`** - Update migration error handling
5. **`internal/api/api.go`** - Add mutual exclusion in `recoverSchedulerReminders()`

---

## Recommended Next Steps

1. **IMMEDIATE:** Fix timezone column migration (Option A)
2. **BEFORE MERGE:** Implement mutual exclusion logic
3. **AFTER MERGE:** Monitor production for dual recovery
4. **NEXT SPRINT:** Implement timer persistence in SchedulerTool
5. **FUTURE:** Plan migration versioning system
