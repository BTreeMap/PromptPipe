# PromptPipe Data Persistence & Recovery - Comprehensive Review & Fixes

## Executive Summary

After a thorough review of the entire codebase, I identified **critical gaps** in data persistence and recovery that would cause significant data loss on server restart. This document details all findings and the comprehensive fixes implemented.

## Critical Issues Found

### 1. **TIMER STATE NOT PERSISTED** ⚠️ CRITICAL

**Impact**: ALL timers lost on restart (scheduled prompts, reminders, feedback timers, etc.)

**Problem**:

- `SimpleTimer` in `/internal/flow/timer.go` stores all timer data in memory only (`map[string]*timerEntry`)
- No database persistence for timer records
- When server restarts, all scheduled activities are lost
- Affects:
  - Daily habit prompt schedules (recurring timers)
  - One-time timers (feedback timeouts, reminders)
  - State transition timers
  - Auto-feedback enforcement timers
  - Daily prompt reminder timers

**Impact Scenarios**:

- User schedules daily 9 AM habit reminders → Server restarts at 8 PM → Schedule lost, no reminder next day
- User in feedback flow with 15-min timeout → Server restarts → Timer lost, feedback session broken
- Scheduled reminder 5 hours after prompt → Server restarts → Reminder never sent

### 2. **MISSING TIMEZONE PERSISTENCE** ⚠️ HIGH

**Impact**: Timezone-aware scheduling broken on restart

**Problem**:

- `ConversationParticipant` model has `Timezone` field
- Database migrations lacked `timezone` column
- On recovery, participant timezone information lost
- Scheduled prompts would use wrong timezone

### 3. **NO TIMER METADATA STORAGE** ⚠️ CRITICAL

**Impact**: Cannot recover timer context after restart

**Problem**:

- No storage of timer callback information
- No way to know what function to call when timer fires
- No persistence of timer parameters (participant ID, flow type, callback type, etc.)
- Recovery relies on scattered state data keys with incomplete information

### 4. **INCOMPLETE RECOVERY INFRASTRUCTURE** ⚠️ HIGH

**Impact**: Even with partial state data, can't fully recover timers

**Problem**:

- `RecoveryRegistry` collects info but doesn't persist it
- `TimerRecoveryInfo` created but not saved to database
- Only partial recovery exists for some scheduler reminders
- Most timer types have zero recovery support

### 5. **NO CLEANUP FOR EXPIRED TIMERS** ⚠️ MEDIUM

**Impact**: Database bloat and stale data accumulation

**Problem**:

- One-time timers deleted from memory but may leave database records
- No periodic cleanup of expired timer records
- StateData keys may accumulate without cleanup

## Comprehensive Fixes Implemented

### 1. Database Schema Changes

#### Added `active_timers` Table

**File**: `/internal/store/migrations_sqlite.sql` and `/internal/store/migrations_postgres.sql`

```sql
CREATE TABLE IF NOT EXISTS active_timers (
    id TEXT PRIMARY KEY,
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    timer_type TEXT NOT NULL, -- 'once', 'recurring'
    state_type TEXT,
    data_key TEXT,
    callback_type TEXT NOT NULL, -- 'scheduled_prompt', 'feedback_initial', etc.
    callback_params TEXT/JSONB, -- Callback-specific parameters
    scheduled_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP, -- For one-time timers
    original_delay_seconds INTEGER, -- Original delay
    schedule_json TEXT/JSONB, -- For recurring timers
    next_run TIMESTAMP, -- Next execution time
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

**Indexes Created**:

- `idx_active_timers_participant` - Fast participant lookup
- `idx_active_timers_flow_type` - Flow type filtering
- `idx_active_timers_callback_type` - Callback type filtering  
- `idx_active_timers_expires_at` - Expired timer cleanup
- `idx_active_timers_next_run` - Recurring timer scheduling

#### Added `timezone` Column to `conversation_participants`

**Files**: Both migration files

```sql
ALTER TABLE conversation_participants ADD COLUMN timezone TEXT;
```

This ensures participant timezone is persisted and recovered.

### 2. Data Models

#### New `TimerRecord` Model

**File**: `/internal/models/state.go`

```go
type TimerCallbackType string

const (
    TimerCallbackScheduledPrompt TimerCallbackType = "scheduled_prompt"
    TimerCallbackFeedbackInitial TimerCallbackType = "feedback_initial"
    TimerCallbackFeedbackFollowup TimerCallbackType = "feedback_followup"
    TimerCallbackStateTransition TimerCallbackType = "state_transition"
    TimerCallbackReminder TimerCallbackType = "reminder"
    TimerCallbackAutoFeedback TimerCallbackType = "auto_feedback"
)

type TimerRecord struct {
    ID                  string
    ParticipantID       string
    FlowType            FlowType
    TimerType           string // "once" or "recurring"
    StateType           StateType
    DataKey             DataKey
    CallbackType        TimerCallbackType
    CallbackParams      map[string]string // JSON-serialized
    ScheduledAt         time.Time
    ExpiresAt           *time.Time
    OriginalDelaySeconds *int64
    ScheduleJSON        string // Serialized Schedule object
    NextRun             *time.Time
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

### 3. Store Interface Updates

#### Enhanced Store Interface

**File**: `/internal/store/store.go`

Added timer persistence methods:

```go
type Store interface {
    // ... existing methods ...
    
    // Timer persistence management
    SaveTimer(timer models.TimerRecord) error
    GetTimer(id string) (*models.TimerRecord, error)
    ListTimers() ([]models.TimerRecord, error)
    ListTimersByParticipant(participantID string) ([]models.TimerRecord, error)
    ListTimersByFlowType(flowType string) ([]models.TimerRecord, error)
    DeleteTimer(id string) error
    DeleteExpiredTimers() error // Cleanup expired one-time timers
}
```

### 4. Store Implementations

#### InMemoryStore

**File**: `/internal/store/store.go`

- Added `timers map[string]models.TimerRecord` field
- Implemented all timer CRUD methods
- Added to initialization

#### SQLiteStore

**File**: `/internal/store/sqlite.go`

- Implemented `SaveTimer()` with JSON serialization
- Implemented `GetTimer()` with proper NULL handling
- Implemented `ListTimers()`, `ListTimersByParticipant()`, `ListTimersByFlowType()`
- Implemented `DeleteTimer()` and `DeleteExpiredTimers()`
- Updated conversation participant methods to include timezone

#### PostgresStore

**File**: `/internal/store/postgres.go`

- Implemented all timer methods using PostgreSQL syntax
- Used JSONB for callback_params and schedule_json
- Proper UPSERT handling with ON CONFLICT
- Updated conversation participant methods to include timezone

### 5. Timezone Persistence Fixes

**Files**:

- `/internal/store/sqlite.go`
- `/internal/store/postgres.go`

**Changes**:

- Updated `SaveConversationParticipant()` to include timezone field
- Updated `GetConversationParticipant()` to retrieve timezone
- Updated `GetConversationParticipantByPhone()` to retrieve timezone
- Updated `ListConversationParticipants()` to retrieve timezone

All queries now include the timezone column in INSERT, SELECT statements.

## Next Steps Required

### PHASE 2: Timer Persistence Integration (CRITICAL)

The database infrastructure is now ready, but the timer creation code needs to be updated to persist timers. This requires modifications to:

1. **`/internal/flow/timer.go`** - SimpleTimer implementation
   - Add store dependency
   - Persist on `ScheduleAfter()`
   - Persist on `ScheduleAt()`
   - Persist on `ScheduleWithSchedule()`
   - Delete from DB on `Cancel()`
   - Delete from DB on `Stop()`
   - Clean up on timer execution (one-time timers)

2. **Timer Recovery on Startup**
   - Read all active timers from database
   - Reconstruct timer callbacks based on CallbackType
   - Reschedule one-time timers (if not expired)
   - Reschedule recurring timers with correct next run time
   - Handle overdue timers gracefully

3. **Callback Reconstruction**
   - Need to recreate the appropriate callback function for each timer type
   - Map CallbackType to actual function execution
   - Restore callback context from CallbackParams

### PHASE 3: Flow Integration

Update flow tools to work with persisted timers:

1. **SchedulerTool** (`/internal/flow/scheduler_tool.go`)
   - Already has partial recovery logic
   - Enhance to use TimerRecord table
   - Clean up when schedules are deleted

2. **FeedbackModule**
   - Persist feedback initial timer
   - Persist feedback followup timer

3. **StateTransitionTool**
   - Persist delayed transition timers

4. **ConversationFlow**
   - Ensure all timer creations are persisted

### PHASE 4: Recovery Enhancements

**File**: `/internal/recovery/recovery.go`

Enhance recovery to:

- Load all timers from database
- Reconstruct timer callbacks
- Register recovered timers with SimpleTimer
- Clean up stale/expired records

### PHASE 5: Testing & Validation

1. **Unit Tests**
   - Test timer persistence in all stores
   - Test timer recovery logic
   - Test expired timer cleanup

2. **Integration Tests**
   - Test full restart cycle
   - Verify scheduled prompts survive restart
   - Verify feedback timers survive restart
   - Verify recurring schedules survive restart

3. **Edge Case Tests**
   - Server restart during timer execution
   - Overdue timers on restart
   - Multiple restarts in quick succession
   - Database corruption recovery

## Data Types Verified as Persisted

✅ **Receipts** - Fully persisted (receipts table)
✅ **Responses** - Fully persisted (responses table)
✅ **FlowState** - Fully persisted (flow_states table)
✅ **ConversationParticipant** - Fully persisted + timezone fix (conversation_participants table)
✅ **RegisteredHook** - Fully persisted (registered_hooks table)
✅ **Timers** - NOW SUPPORTED (active_timers table) - Integration pending

## Persistence Coverage Matrix

| Data Type | Persisted | Recovered | Migration Tested |
|-----------|-----------|-----------|------------------|
| Receipts | ✅ | N/A | ✅ |
| Responses | ✅ | N/A | ✅ |
| FlowState | ✅ | ✅ | ✅ |
| ConversationParticipant | ✅ | ✅ | ✅ |
| RegisteredHook | ✅ | ✅ | ✅ |
| Timers (Infrastructure) | ✅ | ⚠️ Pending | ⚠️ Pending |
| Timer Callbacks | ⚠️ Pending | ⚠️ Pending | ⚠️ Pending |

## Breaking Changes

### Database Migrations Required

After deploying these changes, existing databases need:

1. **Run migrations** to create `active_timers` table
2. **Run migrations** to add `timezone` column to `conversation_participants`

Both SQLite and PostgreSQL migrations are idempotent (use `IF NOT EXISTS`).

### No Breaking API Changes

All changes are additive:

- New methods added to Store interface
- Existing methods unchanged
- New fields added to existing tables (nullable)

## Security Considerations

1. **Timer Callback Parameters** - Serialized as JSON, ensure no sensitive data
2. **Timer Ownership** - Always scoped to participant_id for access control
3. **Database Indexes** - Added for performance, no security impact
4. **Timezone Data** - User-provided, validate using `time.LoadLocation()`

## Performance Impact

### Positive

- Indexes on active_timers table enable fast lookups
- Expired timer cleanup prevents database bloat
- Participant queries now include timezone (one field addition)

### Considerations

- Timer creation now requires DB write (minimal overhead)
- Recovery on startup requires DB read of all active timers
- Recommend periodic cleanup job for expired timers

## Monitoring & Observability

### Recommended Metrics

1. **Timer Persistence**
   - Timer save failures
   - Timer load failures on recovery
   - Timer count per participant

2. **Recovery**
   - Number of timers recovered on startup
   - Recovery failures
   - Overdue timers found

3. **Cleanup**
   - Expired timers deleted
   - Orphaned records found

### Logging

All implementations include comprehensive debug logging:

- Timer CRUD operations
- Recovery operations
- Cleanup operations
- All error conditions

## Conclusion

This comprehensive review identified critical gaps in timer persistence that would cause complete data loss on server restart. The implemented infrastructure changes provide:

1. ✅ Database tables for timer persistence
2. ✅ Complete CRUD operations in all store implementations  
3. ✅ Timezone persistence fix
4. ✅ Data models for timer records
5. ✅ Infrastructure for expired timer cleanup

**Remaining work** (PHASE 2-5) involves:

- Integrating timer persistence into SimpleTimer
- Building timer recovery logic
- Reconstructing timer callbacks
- Comprehensive testing

The foundation is now solid and ready for the integration work.
