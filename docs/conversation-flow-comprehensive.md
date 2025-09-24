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

```text
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

- Participant enrollment, list/get/update/delete handlers
- Phone number validation and canonicalization
- Initial state setup and background context storage

**Data Flow**:

```text
HTTP Request -> JSON Validation -> Phone Canonicalization -> 
Participant Creation -> State Initialization -> Background Storage
```

**Key Data Structures**:

```go
type ConversationEnrollmentRequest struct {
    PhoneNumber string `json:"phone_number"`
    Name        string `json:"name,omitempty"`
    Gender      string `json:"gender,omitempty"`
    Ethnicity   string `json:"ethnicity,omitempty"`
    Background  string `json:"background,omitempty"`
    Timezone    string `json:"timezone,omitempty"`
}

type ConversationParticipant struct {
    ID          string    `json:"id"`
    PhoneNumber string    `json:"phone_number"`
    Name        string    `json:"name,omitempty"`
    Gender      string    `json:"gender,omitempty"`
    Ethnicity   string    `json:"ethnicity,omitempty"`
    Background  string    `json:"background,omitempty"`
    Timezone    string    `json:"timezone,omitempty"`
    Status      string    `json:"status"`
    EnrolledAt  time.Time `json:"enrolled_at"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
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

```text
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

```text
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

```text
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

```text
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

```text
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

## Three-Bot Architecture Data Flow

The PromptPipe system implements a sophisticated three-bot architecture where data flows through three specialized AI agents, each with distinct responsibilities in the habit formation journey. This section details how user data passes through each bot and the interactions between them.

### Architecture Overview

```text
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   INTAKE BOT    │ -> │ PROMPT GENERATOR│ -> │FEEDBACK TRACKER │
│  Profile Builder│    │  Habit Creator  │    │ Profile Updater │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         |                         |                         |
         v                         v                         v
   ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
   │User Profile │       │Generated    │       │Updated      │
   │Creation     │       │Habit Prompts│       │Profile &    │
   │             │       │             │       │Feedback     │
   └─────────────┘       └─────────────┘       └─────────────┘
```

### 1. Intake Bot Flow - Profile Building Phase

**Purpose**: Build comprehensive user profiles through structured conversational intake

**Entry Point**: Tool call `conduct_intake` from main conversation flow

**Data Flow Diagram**:

```text
User Message -> AI Decision Engine -> conduct_intake() -> Intake Bot Tool
     |                                                           |
     v                                                           v
┌─────────────────────────────────────────────────────────────────────────┐
│                           INTAKE BOT FLOW                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │   WELCOME    │ -> │   GOAL_AREA  │ -> │  MOTIVATION  │              │
│  │              │    │              │    │              │              │
│  │ "Hi! I'm     │    │ "What habit  │    │ "Why does    │              │
│  │ your micro-  │    │ have you     │    │ this matter  │              │
│  │ coach bot"   │    │ been meaning │    │ to you now?" │              │
│  └──────────────┘    │ to build?"   │    └──────────────┘              │
│         |             └──────────────┘           |                      │
│         v                      |                 v                      │
│  ┌─────────────────┐          v       ┌──────────────┐                 │
│  │ User consents?  │   ┌──────────────┐│ Store       │                 │
│  │ yes/no          │   │ Parse & Store││ motivation  │                 │
│  └─────────────────┘   │ target       │└──────────────┘                 │
│         |               │ behavior     │       |                        │
│         v               └──────────────┘       v                        │
│  ┌─────────────────┐           |               ┌──────────────┐         │
│  │ If yes: move to │           v       ┌──────────────┐│PREFERRED_TIME│                 │
│  │ goal area       │    ┌──────────────┐│              │                 │
│  │ If no: polite   │    │ Categories:  ││ "When during │                 │
│  │ exit            │    │ • healthy    ││ would you    │                 │
│  └─────────────────┘    │   eating     ││ like a       │                 │
│                         │ • physical   ││ nudge?"      │                 │
│                         │ • mental     │└──────────────┘                 │
│                         │ • reduce     │       |                        │
│                         │   screen     │       v                        │
│                         │ • custom     │┌──────────────┐                 │
│                         └──────────────┘│ Parse & Store│                 │
│                                        ││ preferred    │                 │
│                                        ││ time         │                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││PROMPT_ANCHOR │                 │
│                                        ││              │                 │
│                                        ││ "When do you │                 │
│                                        ││ think this   │                 │
│                                        ││ habit would  │                 │
│                                        ││ naturally    │                 │
│                                        ││ fit?"        │                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││ Parse & Store│                 │
│                                        ││ prompt anchor│                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││ADDITIONAL_   │                 │
│                                        ││INFO          │                 │
│                                        ││              │                 │
│                                        ││ "Anything    │                 │
│                                        ││ else to help │                 │
│                                        ││ personalize?"│                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││ Store        │                 │
│                                        ││ additional   │                 │
│                                        ││ info         │                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││  COMPLETE    │                 │
│                                        ││              │                 │
│                                        ││ "Would you   │                 │
│                                        ││ like to try  │                 │
│                                        ││ a 1-minute   │                 │
│                                        ││ version now?"│                 │
│                                        │└──────────────┘                 │
│                                        │       |                        │
│                                        │       v                        │
│                                        │┌──────────────┐                 │
│                                        ││Final Profile │                 │
│                                        ││Ready for     │                 │
│                                        ││Habit         │                 │
│                                        ││Generation    │                 │
│                                        │└──────────────┘                 │
└─────────────────────────────────────────────────────────────────────────┘
         |
         v
┌─────────────────────────────────────────────────────────────────────────┐
│                        DATA PERSISTENCE                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ UserProfile JSON stored in:                                             │
│ FlowType: models.FlowTypeConversation                                   │
│ DataKey: models.DataKeyUserProfile                                      │
│                                                                         │
│ Profile Structure:                                                      │
│ {                                                                       │
│   "target_behavior": "physical activity",                               │
│   "motivational_frame": "feel more energized",                          │
│   "preferred_time": "morning",                                          │
│   "prompt_anchor": "after coffee",                                      │
│   "additional_info": "limited mobility",                                │
│   "created_at": "2025-01-15T10:00:00Z",                                 │
│   "updated_at": "2025-01-15T10:05:00Z",                                 │
│   "success_count": 0,                                                   │
│   "total_prompts": 0                                                    │
│ }                                                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

**State Management**:

- **Flow Type**: `models.FlowTypeConversation`
- **State Progression**: `IntakeStateWelcome` → `IntakeStateGoalArea` → `IntakeStateMotivation` → `IntakeStatePreferredTime` → `IntakeStatePromptAnchor` → `IntakeStateAdditionalInfo` → `IntakeStateComplete`
- **Data Storage**: User profile stored as JSON in `models.DataKeyUserProfile`

### 2. Prompt Generator Flow - Habit Creation Phase

**Purpose**: Generate personalized 1-minute habit prompts using MAP (Motivation × Ability × Prompt) framework

**Entry Point**: Tool call `generate_habit_prompt` from main conversation flow

**Data Flow Diagram**:

```text
User Request -> AI Decision Engine -> generate_habit_prompt() -> Prompt Generator Tool
     |                                                                    |
     v                                                                    v
┌─────────────────────────────────────────────────────────────────────────┐
│                      PROMPT GENERATOR FLOW                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐               │
│  │   TRIGGER    │ -> │  RETRIEVE    │ -> │   VALIDATE   │               │
│  │              │    │   PROFILE    │    │   PROFILE    │               │
│  │ Tool called  │    │              │    │              │               │
│  │ with params: │    │ Query state  │    │ Check for:   │               │
│  │ • delivery_  │    │ manager for  │    │ • target_    │               │
│  │   mode       │    │ UserProfile  │    │   behavior   │               │
│  │ • personal-  │    │ from DataKey │    │ • motivation │               │
│  │   ization_   │    │ UserProfile  │    │ • timing     │               │
│  │   notes      │    │              │    │ • anchor     │               │
│  └──────────────┘    └──────────────┘    └──────────────┘               │
│         |                     |                  |                      │
│         v                     v                  v                      │
│  ┌─────────────────┐   ┌─────────────────┐   ┌──────────────┐           │
│  │ delivery_mode:  │   │ Profile found?  │   │ Complete?    │           │
│  │ • "immediate"   │   │                 │   │              │           │
│  │ • "scheduled"   │   │ Yes: Parse JSON │   │ Yes: Continue│           │
│  └─────────────────┘   │ No: Error       │   │ No: Error    │           │
│                        └─────────────────┘   └──────────────┘           │
│                               |                     |                   │
│                               v                     v                   │
│                        ┌─────────────────┐   ┌──────────────┐           │
│                        │ UserProfile:    │   │ Error msg    │           │
│                        │ • target_       │   │ "Profile     │           │
│                        │   behavior      │   │ incomplete"  │           │
│                        │ • motivational_ │   └──────────────┘           │
│                        │   frame         │                              │
│                        │ • preferred_    │                              │
│                        │   time          │                              │
│                        │ • prompt_anchor │                              │
│                        │ • success_count │                              │
│                        │ • last_barrier  │                              │
│                        │ • last_tweak    │                              │
│                        └─────────────────┘                              │
│                               |                                         │
│                               v                                         │
│                        ┌─────────────────┐                              │
│                        │ BUILD SYSTEM    │                              │
│                        │ PROMPT          │                              │
│                        │                 │                              │
│                        │ Base prompt +   │                              │
│                        │ • Delivery mode │                              │
│                        │ • Success count │                              │
│                        │ • Barriers      │                              │
│                        │ • Modifications │                              │
│                        │ • Personalize   │                              │
│                        │   notes         │                              │
│                        └─────────────────┘                              │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ GENERATE PROMPT │                            │
│                        │                 │                            │
│                        │ Call GenAI with:│                            │
│                        │ • System prompt │                            │
│                        │ • User prompt:  │                            │
│                        │   "Generate     │                            │
│                        │   personalized  │                            │
│                        │   1-min habit   │                            │
│                        │   using MAP     │                            │
│                        │   framework"    │                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ PROCESS OUTPUT  │                            │
│                        │                 │                            │
│                        │ • Clean response│                            │
│                        │ • Remove quotes │                            │
│                        │ • Validate      │                            │
│                        │   format        │                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ STORE & RETURN  │                            │
│                        │                 │                            │
│                        │ • Store in      │                            │
│                        │   DataKey       │                            │
│                        │   LastHabit     │                            │
│                        │   Prompt        │                            │
│                        │ • Add tracking  │                            │
│                        │   question      │                            │
│                        │ • Return to user│                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ SAMPLE OUTPUT:  │                            │
│                        │                 │                            │
│                        │ "After your     │                            │
│                        │ coffee, try     │                            │
│                        │ doing 10        │                            │
│                        │ jumping jacks — │                            │
│                        │ it helps you    │                            │
│                        │ feel more       │                            │
│                        │ energized.      │                            │
│                        │ Would that feel │                            │
│                        │ doable?"        │                            │
│                        │                 │                            │
│                        │ "Let me know    │                            │
│                        │ when you've     │                            │
│                        │ tried it!"      │                            │
│                        └─────────────────┘                            │
└─────────────────────────────────────────────────────────────────────────┘
         |
         v
┌─────────────────────────────────────────────────────────────────────────┐
│                        DATA PERSISTENCE                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ Last Habit Prompt stored in:                                            │
│ FlowType: models.FlowTypeConversation                                   │
│ DataKey: models.DataKeyLastHabitPrompt                                  │
│                                                                         │
│ Value: "After your coffee, try doing 10 jumping jacks..."              │
│                                                                         │
│ This triggers the Feedback Tracker to await user response               │
└─────────────────────────────────────────────────────────────────────────┘
```

**MAP Framework Implementation**:

- **Motivation**: Uses user's `motivational_frame` from profile
- **Ability**: Ensures 1-minute duration for high feasibility
- **Prompt**: Uses `prompt_anchor` for natural habit timing

**State Management**:

- **Flow Type**: `models.FlowTypeConversation`
- **Data Storage**: Generated prompt stored in `models.DataKeyLastHabitPrompt`
- **Trigger**: Usually called via scheduler or immediate user request

### 3. Feedback Tracker Flow - Profile Update Phase

**Purpose**: Analyze user responses to habit prompts and update profiles for continuous improvement

**Entry Point**: Tool call `track_feedback` from main conversation flow

**Data Flow Diagram**:

```text
User Response -> AI Decision Engine -> track_feedback() -> Feedback Tracker Tool
     |                                                            |
     v                                                            v
┌─────────────────────────────────────────────────────────────────────────┐
│                      FEEDBACK TRACKER FLOW                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │   TRIGGER    │ -> │   EXTRACT    │ -> │   CLASSIFY   │              │
│  │              │    │  ARGUMENTS   │    │   FEEDBACK   │              │
│  │ Tool called  │    │              │    │              │              │
│  │ with user    │    │ Required:    │    │ Status types:│              │
│  │ response to  │    │ • user_      │    │ • completed  │              │
│  │ habit prompt │    │   response   │    │ • attempted  │              │
│  │              │    │ • completion_│    │ • skipped    │              │
│  │              │    │   status     │    │ • rejected   │              │
│  │              │    │              │    │ • modified   │              │
│  │              │    │ Optional:    │    │              │              │
│  │              │    │ • barrier_   │    │              │              │
│  │              │    │   reason     │    │              │              │
│  │              │    │ • suggested_ │    │              │              │
│  │              │    │   modification │  │              │              │
│  └──────────────┘    └──────────────┘    └──────────────┘              │
│         |                     |                  |                     │
│         v                     v                  v                     │
│  ┌─────────────────┐   ┌─────────────────┐   ┌──────────────┐         │
│  │ Example inputs: │   │ Validation:     │   │ Feedback     │         │
│  │ "I did it!"     │   │ • Check         │   │ Processing:  │         │
│  │ "Didn't have    │   │   required      │   │              │         │
│  │  time"          │   │   fields        │   │ ┌─────────── │         │
│  │ "Can we do      │   │ • Validate      │   │ │ completed: │         │
│  │  this evening?" │   │   status        │   │ │ ✓ Success  │         │
│  └─────────────────┘   └─────────────────┘   │ │ ++ count   │         │
│                               |               │ │ Store last │         │
│                               v               │ │ successful │         │
│                        ┌─────────────────┐   │ └─────────── │         │
│                        │ Get Current     │   │              │         │
│                        │ User Profile    │   │ ┌─────────── │         │
│                        │                 │   │ │ skipped:   │         │
│                        │ Query from:     │   │ │ Store      │         │
│                        │ FlowType:       │   │ │ barrier    │         │
│                        │ Conversation    │   │ │ reason     │         │
│                        │ DataKey:        │   │ └─────────── │         │
│                        │ UserProfile     │   │              │         │
│                        └─────────────────┘   │ ┌─────────── │         │
│                               |               │ │ modified:  │         │
│                               v               │ │ Apply      │         │
│                        ┌─────────────────┐   │ │ profile    │         │
│                        │ Get Last Prompt │   │ │ changes    │         │
│                        │                 │   │ │ Store tweak│         │
│                        │ Retrieve from:  │   │ └─────────── │         │
│                        │ DataKey:        │   └──────────────┘         │
│                        │ LastHabitPrompt │                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ UPDATE PROFILE  │                            │
│                        │                 │                            │
│                        │ Increment:      │                            │
│                        │ • total_prompts │                            │
│                        │                 │                            │
│                        │ Update counts:  │                            │
│                        │ • success_count │                            │
│                        │   (if completed)│                            │
│                        │                 │                            │
│                        │ Track patterns: │                            │
│                        │ • last_barrier  │                            │
│                        │ • last_tweak    │                            │
│                        │ • last_success  │                            │
│                        │                 │                            │
│                        │ Apply smart     │                            │
│                        │ modifications:  │                            │
│                        │ • Time changes  │                            │
│                        │ • Anchor updates│                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ GENERATE        │                            │
│                        │ RESPONSE        │                            │
│                        │                 │                            │
│                        │ Based on status:│                            │
│                        │                 │                            │
│                        │ completed:      │                            │
│                        │ "Great job! 🎉  │                            │
│                        │ That's N        │                            │
│                        │ successful      │                            │
│                        │ habits..."      │                            │
│                        │                 │                            │
│                        │ skipped:        │                            │
│                        │ "No worries -   │                            │
│                        │ life happens!"  │                            │
│                        │                 │                            │
│                        │ modified:       │                            │
│                        │ "Perfect! I've  │                            │
│                        │ updated your    │                            │
│                        │ preferences"    │                            │
│                        └─────────────────┘                            │
│                               |                                        │
│                               v                                        │
│                        ┌─────────────────┐                            │
│                        │ SAVE UPDATED    │                            │
│                        │ PROFILE         │                            │
│                        │                 │                            │
│                        │ Marshall to JSON│                            │
│                        │ Store in        │                            │
│                        │ DataKey:        │                            │
│                        │ UserProfile     │                            │
│                        │                 │                            │
│                        │ Updated fields: │                            │
│                        │ • success_count │                            │
│                        │ • total_prompts │                            │
│                        │ • last_barrier  │                            │
│                        │ • last_tweak    │                            │
│                        │ • updated_at    │                            │
│                        └─────────────────┘                            │
└─────────────────────────────────────────────────────────────────────────┘
         |
         v
┌─────────────────────────────────────────────────────────────────────────┐
│                        DATA PERSISTENCE                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ Updated UserProfile JSON stored in:                                     │
│ FlowType: models.FlowTypeConversation                                   │
│ DataKey: models.DataKeyUserProfile                                      │
│                                                                         │
│ Example Updated Profile:                                                │
│ {                                                                       │
│   "target_behavior": "physical activity",                              │
│   "motivational_frame": "feel more energized",                         │
│   "preferred_time": "evening",  // ← Modified based on feedback        │
│   "prompt_anchor": "after coffee",                                     │
│   "additional_info": "limited mobility",                               │
│   "success_count": 3,           // ← Incremented                       │
│   "total_prompts": 5,           // ← Incremented                       │
│   "last_successful_prompt": "After your coffee...",                    │
│   "last_barrier": "lack of time",                                      │
│   "last_tweak": "change time to evening",                              │
│   "updated_at": "2025-01-15T15:30:00Z"  // ← Updated                  │
│ }                                                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

**Feedback Classification**:

- **completed**: Habit successfully performed
- **attempted**: Tried but didn't fully complete
- **skipped**: Didn't attempt due to barriers
- **rejected**: Didn't like the prompt format
- **modified**: Wants changes to timing/format

### Cross-Bot Data Dependencies

**Data Flow Between Bots**:

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                          INTER-BOT DATA FLOW                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│    INTAKE BOT                PROMPT GENERATOR            FEEDBACK       │
│    ────────────              ──────────────────          TRACKER        │
│                                                          ──────────      │
│  UserProfile                      ↓                          ↓          │
│  Creation                    Reads Profile              Reads Profile    │
│      ↓                            ↓                          ↓          │
│                                                                         │
│ ┌─────────────┐              ┌─────────────┐          ┌─────────────┐   │
│ │ Create new  │              │ Use profile │          │ Update      │   │
│ │ profile     │ ────────────→│ to generate │──────────→│ profile     │   │
│ │ with:       │              │ personalized│          │ based on    │   │
│ │             │              │ habit       │          │ user        │   │
│ │ • Target    │              │ prompts     │          │ feedback    │   │
│ │ • Motivation│              │             │          │             │   │
│ │ • Timing    │              │ Uses:       │          │ Updates:    │   │
│ │ • Anchor    │              │ • motivation│          │ • Success   │   │
│ │ • Info      │              │ • anchor    │          │   counts    │   │
│ └─────────────┘              │ • timing    │          │ • Barriers  │   │
│       │                      │ • success   │          │ • Tweaks    │   │
│       │                      │ • barriers  │          │ • Timing    │   │
│       │                      │ • tweaks    │          │ • Anchors   │   │
│       │                      └─────────────┘          └─────────────┘   │
│       │                            │                          │         │
│       │                            │                          │         │
│       │                            v                          │         │
│       │                      ┌─────────────┐                  │         │
│       │                      │ Generated   │                  │         │
│       │                      │ Habit       │──────────────────┘         │
│       │                      │ Prompt      │  Used for context          │
│       │                      │             │  in feedback               │
│       │                      │ Stored in:  │  analysis                  │
│       │                      │ DataKey     │                            │
│       │                      │ LastHabit   │                            │
│       │                      │ Prompt      │                            │
│       │                      └─────────────┘                            │
│       │                                                                 │
│       v                                                                 │
│ ┌─────────────┐                                                         │
│ │ Trigger     │                                                         │
│ │ Prompt      │                                                         │
│ │ Generation  │                                                         │
│ │             │                                                         │
│ │ When intake │                                                         │
│ │ complete,   │                                                         │
│ │ user can    │                                                         │
│ │ request     │                                                         │
│ │ immediate   │                                                         │
│ │ habit       │                                                         │
│ │ prompt      │                                                         │
│ └─────────────┘                                                         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Shared Data Schema**:

```json
{
  "UserProfile": {
    "core_fields": {
      "target_behavior": "string",
      "motivational_frame": "string", 
      "preferred_time": "string",
      "prompt_anchor": "string",
      "additional_info": "string"
    },
    "tracking_fields": {
      "success_count": "number",
      "total_prompts": "number",
      "last_successful_prompt": "string",
      "last_barrier": "string",
      "last_tweak": "string"
    },
    "timestamps": {
      "created_at": "timestamp",
      "updated_at": "timestamp"
    }
  }
}
```

### State Management Across Bots

**Flow Types and Data Keys**:

```go
// Flow types for each bot
FlowTypeConversation     // Main conversation orchestrator
FlowTypeIntakeBot        // Intake bot (deprecated, uses Conversation)
FlowTypePromptGenerator  // Prompt generator state
FlowTypeFeedbackTracker  // Feedback tracker state

// Data keys for shared data
DataKeyUserProfile       // Shared user profile
DataKeyLastHabitPrompt   // Last generated prompt
DataKeyConversationHistory // Main conversation history
DataKeyFeedbackHistory   // Feedback response history
```

**State Transitions**:

```text
Intake: WELCOME → GOAL_AREA → MOTIVATION → PREFERRED_TIME → 
        PROMPT_ANCHOR → ADDITIONAL_INFO → COMPLETE

Conversation: CONVERSATION_ACTIVE (continuous)

Prompt Generator: Ready → Generating → Complete

Feedback Tracker: AWAITING → PROCESSING → COMPLETE
```

This three-bot architecture ensures a seamless user experience where each bot specializes in its domain while sharing data efficiently to create a cohesive habit formation system. The data flows from initial profile creation through personalized prompt generation to continuous improvement based on user feedback.

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
