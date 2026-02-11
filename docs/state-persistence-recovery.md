# State Persistence Recovery System

## Problem Summary

The original PromptPipe application had critical state persistence issues that caused loss of functionality across application restarts:

1. **Timer State Loss**: Timer IDs were stored in flow state data, but actual timers existed only in memory
2. **Response Handler Loss**: Response handlers were registered only in memory
3. **Application-Aware Recovery**: Previous recovery logic was tightly coupled to specific flow types

## Solution: Decoupled Recovery Architecture

### 1. **Generic Recovery Infrastructure** (`internal/recovery/`)

- **`RecoveryManager`**: Orchestrates recovery of all registered components
- **`RecoveryRegistry`**: Provides services and callbacks for infrastructure recovery
- **`Recoverable` Interface**: Components implement this to handle their own recovery logic
- **`TimerRecoveryInfo` & `ResponseHandlerRecoveryInfo`**: Metadata for infrastructure recovery

### 2. **Flow-Specific Recovery** (`internal/flow/`)

- **`ConversationFlowRecovery`**: Handles conversation participant recovery (the current flow type)
- Each flow manages its own business logic while using generic infrastructure

### 3. **Application Integration** (`internal/api/api.go`)

- Recovery system integrated into server startup
- Infrastructure callbacks provided to avoid import cycles
- Recovery runs after store and timer initialization, before server start

## Key Architecture Principles

### **Separation of Concerns**

- Recovery infrastructure handles timers and response handlers generically
- Flow logic handles business-specific recovery concerns
- No business logic embedded in recovery infrastructure

### **Inversion of Control**

- Flows register with recovery manager rather than recovery knowing about flows
- Infrastructure provides callbacks rather than direct dependencies
- Plugin-like architecture for extensibility

### **No Import Cycles**

- Recovery package doesn't import messaging or flow packages
- Callbacks used to wire up dependencies at application level
- Clean dependency graph maintained

## Database Schema Support

The existing database schema already supports all required state persistence:

```sql
-- Flow states with current state and timer IDs in state_data
CREATE TABLE flow_states (
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    current_state TEXT NOT NULL,
    state_data TEXT,
    ...
);

-- Participant table for recovery enumeration
CREATE TABLE conversation_participants (...);
```

## Recovery Process

### **Application Startup**

1. Store and timer infrastructure initialized
2. Recovery manager created with infrastructure callbacks
3. Flow recoveries registered with manager
4. `RecoverAll()` called to restore state

### **Per-Participant Recovery**

1. Query database for active participants by flow type
2. Register response handlers for active conversation participants
3. Scheduler-specific reminders are recovered separately after initialization (see `recoverSchedulerReminders` in `internal/api/api.go`)

### **Infrastructure Recovery**

- **Timer Recovery**: Uses a generic callback that logs timeouts (business-specific timer recovery is handled by the owning component)
- **Response Handler Recovery**: Recreates hooks based on flow type
- **Error Handling**: Continues recovery even if individual components fail

## Testing

- **`recovery_test.go`**: Tests generic recovery infrastructure with mocks
- Demonstrates decoupled architecture without dependencies on real flows
- Validates timer and response handler recovery callbacks

## Benefits

1. **Resilient**: Application restarts don't lose participant state
2. **Extensible**: New flow types just implement `Recoverable` interface  
3. **Testable**: Clear boundaries enable focused unit testing
4. **Maintainable**: Business logic separated from infrastructure concerns
5. **No Breaking Changes**: Uses existing database schema

## Durable Jobs and Outbox (Persistence Hardening)

In addition to the recovery system above, the persistence layer includes:

### Durable Jobs (`internal/store/job_repo.go`, `job_runner.go`)

All time-based behavior that must survive restarts is represented as **persisted job records** in the `jobs` table, replacing in-memory `time.AfterFunc` calls:

- **`JobRepo`**: Interface for enqueueing, claiming, completing, failing, and canceling jobs. Implementations exist for both SQLite and Postgres.
- **`JobRunner`**: Background worker that polls for due jobs and dispatches them to registered handlers. Supports exponential backoff on failure and crash recovery via `RequeueStaleRunningJobs()`.
- **Dedupe keys**: Prevent duplicate jobs for the same intent (e.g., one scheduled prompt per participant per schedule).

### Outbox Messages (`internal/store/outbox_repo.go`, `outbox_sender.go`)

Outgoing WhatsApp messages are persisted to the `outbox_messages` table before being sent:

- **`OutboxRepo`**: Interface for enqueueing and managing outbound messages. Dedupe keys prevent duplicate sends.
- **`OutboxSender`**: Background worker that claims due messages and sends them via the messaging service. Supports retry with backoff.
- On startup, stale `sending` messages are requeued.

### Inbound Dedup (`internal/store/dedup_repo.go`)

The `inbound_dedup` table prevents reprocessing of inbound WhatsApp messages after restart:

- **`DedupRepo`**: Interface for recording and checking message IDs.
- On message arrival, the system checks if the message ID was already processed and skips if so.

### Startup Reconciliation

On startup:
1. Stale `running` jobs are requeued (`RequeueStaleRunningJobs`)
2. Stale `sending` outbox messages are requeued (`RequeueStaleSendingMessages`)
3. Existing recovery (response handlers, scheduler reminders) continues as before

### Docker Compose Persistence

The `docker-compose.yml` uses named volumes for both Postgres data and the PromptPipe state directory. `docker compose down && docker compose up -d` preserves all state. Only `docker compose down -v` destroys volumes.

See `docs/persistence-audit.md` for the complete state ledger.

## Integration Example

```go
// In server initialization
recoveryManager := recovery.NewRecoveryManager(store, timer)

// Register flow recoveries
stateManager := flow.NewStoreBasedStateManager(store)
recoveryManager.RegisterRecoverable(
    flow.NewConversationFlowRecovery())

// Register infrastructure callbacks
recoveryManager.RegisterTimerRecovery(
    recovery.TimerRecoveryHandler(timer))
recoveryManager.RegisterHandlerRecovery(
    createResponseHandlerCallback(respHandler, msgService))

// Perform recovery
recoveryManager.RecoverAll(context.Background())
```

This solution provides robust state recovery while maintaining clean architecture principles and enabling future extensibility.
