# PromptPipe Logging Standard

## Format Convention

All log messages should follow this format:

```
[Component].[Function]: [Purpose/Action] [status]
```

### Examples

- `slog.Debug("ConversationFlow.ProcessResponse: checking phone number context", "participantID", participantID)`
- `slog.Info("ConversationFlow.ProcessResponse: response processed successfully", "participantID", participantID)`
- `slog.Error("ConversationFlow.ProcessResponse: failed to get current state", "error", err, "participantID", participantID)`

## Components

### Flow Package

- `ConversationFlow` - conversation flow operations
- `SchedulerTool` - scheduling operations  
- `OneMinuteInterventionTool` - intervention operations
- `SimpleTimer` - timer operations
- `StateManager` - state management operations
- `MicroHealthInterventionGenerator` - micro health intervention operations

### API Package

- `Server` - HTTP server operations
- `Handler` - HTTP request handling

### Messaging Package

- `ResponseHandler` - response handling operations
- `WhatsAppService` - messaging service operations

### GenAI Package

- `Client` - GenAI client operations

## Log Levels

### Debug

- Used for tracing execution flow and detailed state information
- Include function entry/exit points
- Include parameter validation and context checks
- Format: present tense for ongoing actions ("checking", "processing", "creating")

### Info  

- Used for successful completion of important operations
- Include significant state changes
- Include successful external service calls
- Format: past tense for completed actions ("processed successfully", "created", "loaded")

### Warn

- Used for recoverable errors or fallback scenarios
- Include configuration issues that can be worked around
- Format: past tense for completed fallback actions

### Error

- Used for failures that prevent operation completion
- Always include error details and context
- Format: past tense for failed operations ("failed to", "could not")

## Context Parameters

Always include relevant context parameters:

- `participantID` - for participant-specific operations
- `phoneNumber` - for messaging operations  
- `timerID` - for timer operations
- `error` - for error logging
- `toolCallID` - for tool execution tracking

## Purpose Descriptions

Common purpose patterns:

- "checking [what]" - validation/verification operations
- "loading [what]" - data loading operations
- "processing [what]" - data processing operations  
- "creating [what]" - resource creation operations
- "executing [what]" - action execution operations
- "failed to [action]" - error conditions
- "[action] successful" - success conditions
