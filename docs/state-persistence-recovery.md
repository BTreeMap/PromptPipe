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

- **`MicroHealthInterventionRecovery`**: Handles intervention participant recovery
- **`ConversationFlowRecovery`**: Handles conversation participant recovery
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
    state_data TEXT,  -- JSON with timer IDs and other state
    ...
);

-- Participant tables for recovery enumeration
CREATE TABLE intervention_participants (...);
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
2. For each participant, check current state and stored timer IDs
3. Clear stale timer IDs and recreate timers with shortened timeouts
4. Register appropriate response handlers for phone numbers

### **Infrastructure Recovery**

- **Timer Recovery**: Uses shortened timeouts since restart time unknown
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

## Integration Example

```go
// In server initialization
recoveryManager := recovery.NewRecoveryManager(store, timer)

// Register flow recoveries
stateManager := flow.NewStoreBasedStateManager(store)
recoveryManager.RegisterRecoverable(
    flow.NewMicroHealthInterventionRecovery(stateManager))
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
