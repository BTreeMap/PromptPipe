# Button to Poll Message Migration

> **Note**: This document describes a specific migration completed on 2025-10-17. It is maintained for historical reference and to document the current poll-based interaction system.

## Overview

This document describes the migration from WhatsApp's deprecated button messages to poll messages, completed on 2025-10-17.

## Background

WhatsApp deprecated `ButtonsMessage` in their API. To maintain functionality, we migrated to using poll messages while preserving the existing interface names for backward compatibility.

## Implementation Details

### Poll Configuration

- **Poll Title**: "Did you do it?"
- **Options**:
  1. "Done"
  2. "Next time"
- **Selection Mode**: Single selection only (selectableOptionsCount = 1)

### Message Flow

1. Send the main prompt message as a text message
2. Immediately follow with a poll message for engagement tracking
3. User responds by selecting one of the poll options

## Modified Files

### 1. `/internal/whatsapp/whatsapp.go`

- **Changes**: Updated `SendPromptButtons()` method to send text message + poll
- **Real Client**: Sends message then creates `PollCreationMessage` with protobuf structures
- **Mock Client**: Simulates both message sends for testing
- **Removed**: Unused constants `promptDoneButtonID` and `promptLaterButtonID`

### 2. `/internal/messaging/whatsapp_service.go`

- **Changes**: Updated documentation and logging
- **Note**: Method name remains `SendPromptWithButtons` for API compatibility
- **Logs**: Now mention "poll message" instead of "buttons"

### 3. `/internal/api/handlers.go`

- **Changes**: Added documentation explaining the button→poll migration
- **Functions Updated**: `shouldAttachPromptButtons()`, `sendPromptMessage()`
- **Note**: Interface names preserved for backward compatibility

### 4. `/internal/flow/scheduler_tool.go`

- **Changes**: Added documentation to `promptButtonsSender` interface
- **Note**: Interface name preserved despite internal poll implementation

## Testing

All existing tests pass without modification:

- API handler tests
- WhatsApp client tests (real and mock)
- Scheduler tool tests
- Full test suite: `go test ./internal/...` ✅

## Backward Compatibility

- All method names remain unchanged (`SendPromptWithButtons`, `promptButtonsSender`)
- External API contracts preserved
- Only internal implementation changed
- No breaking changes for existing code

## Migration Rationale

1. **WhatsApp Deprecation**: Button messages no longer supported
2. **Feature Parity**: Polls provide similar engagement tracking
3. **User Experience**: "Did you do it?" with two options maintains simplicity
4. **Interface Stability**: Existing code continues to work without changes

## Future Considerations

- Monitor WhatsApp API updates for poll message changes
- Consider renaming interfaces in a major version bump (breaking change)
- Potential to add more poll options if needed for enhanced tracking
