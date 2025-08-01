# Conversation Flow: Comprehensive Data Flow Architecture

This document provides a detailed technical overview of the conversation flow system within PromptPipe, focusing on how user data flows across multiple modules and the underlying architecture that enables persistent conversational interactions.

## Table of Contents

- [System Overview](#system-overview)
- [Data Flow Architecture](#data-flow-architecture)
- [Core Components](#core-components)
- [User Data Journey](#user-data-journey)
- [State Management System](#state-management-system)
- [Module Interactions](#module-interactions)
- [Message Processing Pipeline](#message-processing-pipeline)
- [Persistence and Recovery](#persistence-and-recovery)
- [Integration Patterns](#integration-patterns)
- [Error Handling and Resilience](#error-handling-and-resilience)

## System Overview

The conversation flow system enables persistent, AI-powered conversational interactions with users through a multi-layered architecture. The system maintains conversation state, history, and context across application restarts while providing extensible hooks for tool integration and custom behavior.

### Key Characteristics

- **Stateful Conversations**: Maintains conversation history and context per participant
- **Multi-Modal Integration**: Supports various messaging backends (WhatsApp, testing modes)
- **AI-Powered Responses**: Integrates with GenAI services for intelligent conversation generation
- **Tool Support**: Extensible architecture for AI tool calling (scheduler, interventions)
- **Persistent State**: Robust state persistence and recovery across application restarts
- **Scalable Architecture**: Clean separation of concerns with well-defined interfaces

## Data Flow Architecture

### High-Level Data Flow

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   User/Client   │ -> │  Bot Runtime    │ -> │   Flow Engine   │
│  (WhatsApp/SMS) │    │  (whatsmeow)    │    │ (Conversation)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         ▲                         |                         |
         |                         v                         v
         |              ┌─────────────────┐    ┌─────────────────┐
         |              │ Response Handler│    │ State Manager   │
         |              │   (Messaging)   │    │  (Persistence)  │
         |              └─────────────────┘    └─────────────────┘
         |                         |                         |
         |                         v                         v
         |              ┌─────────────────┐    ┌─────────────────┐
         |              │   GenAI Client  │    │   Store Layer   │
         └────────────── │   (OpenAI)      │    │ (SQLite/Postgres)│
                        └─────────────────┘    └─────────────────┘
```

### Data Flow Layers

1. **Presentation Layer**: API endpoints (`/conversation/participants`)
2. **Business Logic Layer**: Conversation flow engine and state management
3. **Integration Layer**: GenAI client, messaging services, tool integrations
4. **Persistence Layer**: Database storage with state management
5. **Infrastructure Layer**: Recovery system, timers, background processes

## Core Components

### 1. API Layer (`internal/api/conversation_handlers.go`)

**Purpose**: HTTP endpoints for participant management and conversation initiation

**Key Functions**:

- `enrollConversationParticipantHandler`: Participant registration
- Phone number validation and canonicalization
- Initial state setup and background context storage

**Data Flow**:

```
HTTP Request -> JSON Validation -> Phone Canonicalization -> 
Participant Creation -> State Initialization -> Background Storage
```

**Key Data Structures**:

```go
type ConversationEnrollmentRequest struct {
    PhoneNumber string `json:"phoneNumber"`
    Name        string `json:"name"`
    Gender      string `json:"gender,omitempty"`
    Ethnicity   string `json:"ethnicity,omitempty"`
    Background  string `json:"background,omitempty"`
}

type ConversationParticipant struct {
    ID          string    `json:"id"`
    PhoneNumber string    `json:"phoneNumber"`
    Name        string    `json:"name"`
    Gender      string    `json:"gender,omitempty"`
    Ethnicity   string    `json:"ethnicity,omitempty"`
    Background  string    `json:"background,omitempty"`
    Status      string    `json:"status"`
    EnrolledAt  time.Time `json:"enrolledAt"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}
```

### 2. Conversation Flow Engine (`internal/flow/conversation_flow.go`)

**Purpose**: Core business logic for managing conversational interactions

**Key Components**:

- **ConversationFlow**: Main flow implementation
- **ConversationMessage**: Individual message structure
- **ConversationHistory**: Complete conversation history container

**Core Methods**:

- `ProcessResponse()`: Handles incoming user messages
- `processConversationMessage()`: Processes messages and generates AI responses
- `processWithTools()`: Enhanced processing with AI tool calling

**Data Structures**:

```go
type ConversationMessage struct {
    Role      string    `json:"role"`      // "user" or "assistant"
    Content   string    `json:"content"`   // message content
    Timestamp time.Time `json:"timestamp"` // when the message was sent
}

type ConversationHistory struct {
    Messages []ConversationMessage `json:"messages"`
}
```

### 3. State Management System (`internal/flow/state_manager.go`)

**Purpose**: Centralized state persistence and retrieval across all flows

**Interface**:

```go
type StateManager interface {
    GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error)
    SetCurrentState(ctx context.Context, participantID string, flowType models.FlowType, state models.StateType) error
    GetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey) (string, error)
    SetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey, value string) error
    TransitionState(ctx context.Context, participantID string, flowType models.FlowType, fromState, toState models.StateType) error
    ResetState(ctx context.Context, participantID string, flowType models.FlowType) error
}
```

**Implementation**: `StoreBasedStateManager` provides concrete implementation backed by database storage

### 4. Messaging Layer (`internal/messaging/response_handler.go`)

**Purpose**: Routes incoming messages to appropriate flow handlers

**Key Features**:

- Hook-based architecture for response routing
- Phone number canonicalization
- Default response handling
- Persistent hook registration

**Core Methods**:

- `RegisterHook()`: Associates phone numbers with response handlers
- `ProcessResponse()`: Routes incoming messages to registered hooks
- `UnregisterHook()`: Removes response associations

### 5. Storage Layer (`internal/store/`)

**Purpose**: Database abstraction layer supporting multiple backends

**Supported Backends**:

- SQLite (`sqlite.go`)
- PostgreSQL (`postgres.go`)
- In-Memory (`store.go`)

**Key Tables**:

- `flow_states`: Current state and state data per participant/flow
- `conversation_participants`: Participant profiles and metadata
- `persistent_hooks`: Hook registrations for message routing

## User Data Journey

### 1. Participant Enrollment

**Entry Point**: `POST /conversation/participants`

**Data Flow**:

```
1. HTTP Request Received
   └── JSON payload with participant details

2. Input Validation
   └── Phone number format validation
   └── Required field validation

3. Phone Canonicalization
   └── msgService.ValidateAndCanonicalizeRecipient()
   └── Converts to standard format (e.g., "1234567890")

4. Duplicate Check
   └── store.GetConversationParticipantByPhone()
   └── Prevents duplicate enrollments

5. Participant Creation
   └── Generate unique participant ID
   └── Create ConversationParticipant struct
   └── Set timestamps and status

6. Database Persistence
   └── store.SaveConversationParticipant()
   └── Atomic write to participant table

7. State Initialization
   └── stateManager.SetCurrentState() -> CONVERSATION_ACTIVE
   └── Create flow_states record

8. Background Context Storage
   └── buildParticipantBackgroundInfo()
   └── stateManager.SetStateData() -> DataKeyParticipantBackground
   └── Stores user context for AI personalization

9. Response Hook Registration
   └── responseHandler.RegisterPersistentHook()
   └── Associates phone number with conversation processor
```

**Database Impact**:

- Insert into `conversation_participants` table
- Insert into `flow_states` table
- Insert into `persistent_hooks` table

### 2. Message Processing

**Entry Point**: Incoming message via messaging service

**Data Flow**:

```
1. Message Reception
   └── WhatsApp/SMS service receives message
   └── Creates models.Response object

2. Response Handler Processing
   └── responseHandler.ProcessResponse()
   └── Phone number canonicalization
   └── Hook lookup by phone number

3. Flow Engine Invocation
   └── ConversationFlow.ProcessResponse()
   └── Context injection (phone number)
   └── State validation and initialization

4. Conversation History Retrieval
   └── getConversationHistory()
   └── stateManager.GetStateData() -> DataKeyConversationHistory
   └── JSON deserialization to ConversationHistory

5. Message History Update
   └── Add user message to history
   └── Set timestamp and role

6. AI Processing Decision
   └── Check for available tools (scheduler, intervention)
   └── Route to processWithTools() or standard processing

7. AI Response Generation
   └── buildOpenAIMessages() -> Convert to OpenAI format
   └── Include system prompt + participant background
   └── genaiClient.GenerateWithMessages() or GenerateWithTools()

8. History Update and Persistence
   └── Add AI response to conversation history
   └── Trim history to maxHistoryLength (50 messages)
   └── saveConversationHistory()
   └── JSON serialization and database write

9. Response Delivery
   └── Return AI response to messaging service
   └── Message sent to participant
```

**Database Impact**:

- Update `flow_states.state_data` with new conversation history
- Potential state transitions in `flow_states.current_state`

### 3. State Persistence and Recovery

**Purpose**: Ensure conversation continuity across application restarts

**Persistence Points**:

```
1. Conversation History
   └── JSON-serialized in flow_states.state_data
   └── Key: DataKeyConversationHistory
   └── Includes all messages with timestamps

2. Participant Background
   └── User context for AI personalization
   └── Key: DataKeyParticipantBackground
   └── Includes name, gender, ethnicity, background

3. Flow State
   └── Current conversation state
   └── Usually: CONVERSATION_ACTIVE

4. Hook Registrations
   └── persistent_hooks table
   └── Phone -> flow type associations
   └── Recreated on application startup
```

**Recovery Process**:

```
1. Application Startup
   └── Recovery system initialization

2. Hook Recreation
   └── Query persistent_hooks table
   └── Recreate responseHandler hooks
   └── Associate phone numbers with flow processors

3. State Validation
   └── Existing flow_states preserved
   └── No active state modification during recovery

4. Dependency Injection
   └── Flow engines receive state managers
   └── GenAI clients configured
   └── Messaging services connected
```

## State Management System

### State Data Model

```go
type FlowState struct {
    ParticipantID string                    // Unique participant identifier
    FlowType      FlowType                  // "conversation" for conversation flow
    CurrentState  StateType                 // Current flow state
    StateData     map[DataKey]string        // Key-value state storage
    CreatedAt     time.Time                 // Initial creation timestamp
    UpdatedAt     time.Time                 // Last modification timestamp
}
```

### State Data Keys (Conversation Flow)

```go
const (
    DataKeyConversationHistory   DataKey = "conversationHistory"   // JSON conversation history
    DataKeySystemPrompt          DataKey = "systemPrompt"          // AI system prompt
    DataKeyParticipantBackground DataKey = "participantBackground" // User context
)
```

### State Transitions

**Conversation Flow States**:

- `CONVERSATION_ACTIVE`: Normal conversation state (primary state)

**State Management Operations**:

```go
// Retrieve current state
currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)

// Update state
err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)

// Store state data
err := stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, 
    models.DataKeyConversationHistory, historyJSON)

// Retrieve state data
data, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, 
    models.DataKeyConversationHistory)
```

## Module Interactions

### 1. API → Flow Engine

**Interaction**: Participant enrollment triggers flow initialization

```go
// API Handler
stateManager := flow.NewStoreBasedStateManager(s.st)
err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)

// Background info storage
backgroundInfo := buildParticipantBackgroundInfo(participant)
err := stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, 
    models.DataKeyParticipantBackground, backgroundInfo)
```

### 2. Messaging → Flow Engine

**Interaction**: Incoming messages routed to conversation processor

```go
// Response Handler
hook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
    participantID := getParticipantIDFromPhone(from)
    response, err := conversationFlow.ProcessResponse(ctx, participantID, responseText)
    if err != nil {
        return false, err
    }
    return sendResponse(from, response), nil
}
responseHandler.RegisterHook(phoneNumber, hook)
```

### 3. Flow Engine → GenAI

**Interaction**: AI response generation with conversation context

```go
// Conversation Flow
messages, err := f.buildOpenAIMessages(ctx, participantID, history)
if f.schedulerTool != nil || f.oneMinuteInterventionTool != nil {
    // Tool-enabled generation
    response, err := f.genaiClient.GenerateWithTools(ctx, messages, tools)
} else {
    // Standard generation
    response, err := f.genaiClient.GenerateWithMessages(ctx, messages)
}
```

### 4. Flow Engine → State Manager

**Interaction**: Conversation history persistence

```go
// Save updated conversation history
historyJSON, err := json.Marshal(history)
err = f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, 
    models.DataKeyConversationHistory, string(historyJSON))
```

### 5. State Manager → Store

**Interaction**: Database persistence abstraction

```go
// State Manager Implementation
flowState, err := sm.store.GetFlowState(participantID, string(flowType))
err = sm.store.SaveFlowState(flowState)
```

## Message Processing Pipeline

### 1. Message Reception

```
WhatsApp Service -> models.Response -> ResponseHandler.ProcessResponse()
```

### 2. Hook Resolution

```go
canonicalFrom, err := rh.msgService.ValidateAndCanonicalizeRecipient(response.From)
action, hasHook := rh.hooks[canonicalFrom]
```

### 3. Flow Processing

```go
handled, err := action(ctx, canonicalFrom, response.Body, response.Time)
// action internally calls ConversationFlow.ProcessResponse()
```

### 4. Context Building

```go
// Build OpenAI message format
messages := []openai.ChatCompletionMessageParamUnion{
    openai.SystemMessage(enhancedSystemPrompt),
    // ... conversation history messages
    openai.UserMessage(userMessage),
}
```

### 5. AI Generation

```go
if tools are available {
    toolResponse, err := f.genaiClient.GenerateWithTools(ctx, messages, tools)
    // Handle tool calls
} else {
    response, err := f.genaiClient.GenerateWithMessages(ctx, messages)
}
```

### 6. History Management

```go
// Add both user and assistant messages
history.Messages = append(history.Messages, userMsg, assistantMsg)

// Trim to prevent unbounded growth
const maxHistoryLength = 50
if len(history.Messages) > maxHistoryLength {
    history.Messages = history.Messages[len(history.Messages)-maxHistoryLength:]
}
```

### 7. Response Delivery

```go
// Return response for delivery
return aiResponse, nil
// ResponseHandler sends via messaging service
```

## Persistence and Recovery

### Database Schema

**flow_states table**:

```sql
CREATE TABLE flow_states (
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    current_state TEXT NOT NULL,
    state_data TEXT,              -- JSON blob with conversation history
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (participant_id, flow_type)
);
```

**conversation_participants table**:

```sql
CREATE TABLE conversation_participants (
    id TEXT PRIMARY KEY,
    phone_number TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    gender TEXT,
    ethnicity TEXT,
    background TEXT,
    status TEXT NOT NULL,
    enrolled_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

**persistent_hooks table**:

```sql
CREATE TABLE persistent_hooks (
    phone_number TEXT PRIMARY KEY,
    hook_type TEXT NOT NULL,
    params TEXT,                  -- JSON parameters
    created_at TIMESTAMP NOT NULL
);
```

### Recovery Architecture

**Recovery Manager** (`internal/recovery/`):

- Orchestrates restoration of system state after restarts
- Recreates response handler hooks
- Validates persistent state integrity

**Flow-Specific Recovery**:

- `ConversationFlowRecovery`: Handles conversation participant recovery
- Recreates conversation flow hooks from persistent_hooks table
- No active state modification during recovery

**Recovery Process**:

```go
// Application startup
recoveryManager := recovery.NewRecoveryManager(store, timer)
recoveryManager.RegisterRecoverable(flow.NewConversationFlowRecovery())
recoveryManager.RecoverAll(context.Background())
```

## Integration Patterns

### 1. Tool Integration

**Scheduler Tool**: Enables conversation participants to schedule daily prompts

```go
func (f *ConversationFlow) processWithTools(ctx context.Context, participantID string, 
    messages []openai.ChatCompletionMessageParamUnion, history *ConversationHistory) (string, error) {
    
    tools := []openai.ChatCompletionToolParam{}
    if f.schedulerTool != nil {
        tools = append(tools, f.schedulerTool.GetToolDefinition())
    }
    if f.oneMinuteInterventionTool != nil {
        tools = append(tools, f.oneMinuteInterventionTool.GetToolDefinition())
    }
    
    toolResponse, err := f.genaiClient.GenerateWithTools(ctx, messages, tools)
    // Handle tool calls and return response
}
```

### 2. Multi-Backend Support

**Store Interface**: Supports SQLite, PostgreSQL, and in-memory backends

```go
type Store interface {
    SaveFlowState(state models.FlowState) error
    GetFlowState(participantID, flowType string) (*models.FlowState, error)
    SaveConversationParticipant(participant models.ConversationParticipant) error
    // ... other methods
}
```

**Messaging Interface**: Supports WhatsApp and testing backends

```go
type Service interface {
    SendMessage(ctx context.Context, to, body string) error
    ValidateAndCanonicalizeRecipient(recipient string) (string, error)
    Responses() <-chan models.Response
    // ... other methods
}
```

### 3. Dependency Injection

**Flow Dependencies**:

```go
type Dependencies struct {
    StateManager StateManager
    Timer        models.Timer
}

// Injection
conversationFlow.SetDependencies(deps)
```

## Error Handling and Resilience

### 1. State Consistency

- Atomic database operations for state updates
- Rollback capabilities for failed state transitions
- Validation of state transitions before execution

### 2. Message Delivery Reliability

- Default message responses when hooks fail
- Error messages sent to users for processing failures
- Graceful degradation when AI services are unavailable

### 3. Recovery Resilience

- Continues recovery even if individual components fail
- Shortened timer timeouts after application restart
- Validates participant state before hook recreation

### 4. Data Validation

- Phone number canonicalization and validation
- JSON schema validation for API requests
- State data validation before persistence

### 5. Logging and Observability

```go
slog.Debug("flow.ProcessResponse: processing conversation message", 
    "participantID", participantID, "response", response, "currentState", currentState)
slog.Info("flow.generated response", 
    "participantID", participantID, "responseLength", len(response))
slog.Error("flow.failed to get conversation history", 
    "error", err, "participantID", participantID)
```

## Conclusion

The conversation flow system demonstrates a sophisticated, multi-layered architecture that effectively manages user data flow across diverse system components. The design prioritizes:

- **Modularity**: Clear separation of concerns with well-defined interfaces
- **Persistence**: Robust state management with recovery capabilities
- **Scalability**: Support for multiple backends and extensible tool integration
- **Reliability**: Comprehensive error handling and graceful degradation
- **Maintainability**: Clean code organization and comprehensive logging

This architecture enables the system to provide seamless conversational experiences while maintaining data integrity and system resilience across various operational scenarios.
