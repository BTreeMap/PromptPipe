# Intensity Adjustment Feature

> **Note**: This document describes the intensity adjustment feature implementation. It serves as both implementation documentation and feature reference.

## Overview

This document describes the intensity adjustment feature that allows users to control the frequency and intensity of their daily habit prompts through a poll-based interaction system.

## Feature Design

### User Profile Integration

- Added `Intensity` field to `UserProfile` struct in `internal/flow/profile_save_tool.go`
- Default intensity: `"normal"`
- Backward compatibility: Existing profiles without intensity field automatically get set to `"normal"`
- The intensity value is exposed to all LLMs through the profile, allowing them to adjust prompt messaging accordingly

### Tri-Value System

The intensity parameter supports three levels:

- **low**: Reduced frequency/intensity of prompts
- **normal**: Default frequency/intensity (default for all users)
- **high**: Increased frequency/intensity of prompts

### Smart Option Presentation

The poll options are intelligently presented based on the user's current intensity level:

| Current Intensity | Available Options                                    |
|------------------|------------------------------------------------------|
| low              | "Keep current", "Increase"                          |
| normal           | "Decrease", "Keep current", "Increase"              |
| high             | "Decrease", "Keep current"                          |

This prevents users from selecting options that would exceed the boundaries (e.g., can't decrease from "low" or increase from "high").

### Daily Prompting Schedule

- The intensity adjustment poll is sent **once per day**, not once per prompt
- Tracking is done via `DataKeyLastIntensityPromptDate` in the flow state
- The poll is sent immediately after the daily habit prompt is sent
- If the user has already been prompted today, the poll is skipped

## Implementation Details

### Components Modified

#### 1. Data Model (`internal/models/flow_types.go`)

```go
DataKeyLastIntensityPromptDate = "last_intensity_prompt_date"
```

Tracks the last date the intensity poll was sent to prevent duplicate prompts on the same day.

#### 2. User Profile (`internal/flow/profile_save_tool.go`)

- Added `Intensity string` field with JSON tag `"intensity"`
- `GetOrCreateUserProfile()` sets default `intensity="normal"` for new and existing profiles

#### 3. WhatsApp Client (`internal/whatsapp/whatsapp.go`)

- `IntensityPollQuestion`: Constant for the poll question
- `IntensityPollOptions`: Map of intensity levels to available options
- `SendIntensityAdjustmentPoll()`: Sends the intensity poll with smart options
- `ParseIntensityPollResponse()`: Parses poll responses and returns new intensity level
- `MockClient.SendIntensityAdjustmentPoll()`: Mock implementation for testing

#### 4. Messaging Service (`internal/messaging/whatsapp_service.go`)

- Updated `promptButtonsClient` interface to include `SendIntensityAdjustmentPoll()`
- `SendIntensityAdjustmentPoll()`: Wrapper method that validates recipient and delegates to client

#### 5. Scheduler Tool (`internal/flow/scheduler_tool.go`)

- `checkAndSendIntensityAdjustment()`: Checks if intensity poll should be sent today
  - Reads last prompt date from state
  - Compares with today's date
  - Gets user's current intensity from profile
  - Sends poll if not already sent today
  - Records today's date after sending

#### 6. Conversation Flow (`internal/flow/conversation_flow.go`)

- Added intensity poll response detection and processing
- Parses intensity poll responses using `ParseIntensityPollResponse()`
- Updates user profile with new intensity level
- Logs all intensity changes for tracking

### Flow Diagram

```
Daily Habit Prompt Sent
         ↓
Check Last Intensity Prompt Date
         ↓
    Today? ────Yes──→ Skip
         ↓
        No
         ↓
Get User's Current Intensity
         ↓
Send Intensity Poll (smart options)
         ↓
Record Today's Date
         ↓
   (User responds)
         ↓
Parse Poll Response
         ↓
Calculate New Intensity
         ↓
Update User Profile
         ↓
Save Profile to State
```

### Poll Response Format

Poll responses follow the standard format:

```
Q: How's the intensity? A: [selected_option]
```

Examples:

- `Q: How's the intensity? A: Decrease`
- `Q: How's the intensity? A: Keep current`
- `Q: How's the intensity? A: Increase`

### State Transitions

| Current | Response  | New      |
|---------|-----------|----------|
| low     | Decrease  | low      |
| low     | Keep      | low      |
| low     | Increase  | normal   |
| normal  | Decrease  | low      |
| normal  | Keep      | normal   |
| normal  | Increase  | high     |
| high    | Decrease  | normal   |
| high    | Keep      | high     |
| high    | Increase  | high     |

## Testing

### Test Coverage

1. **intensity_test.go**: Comprehensive tests for intensity parsing and options
   - 11 test cases for `ParseIntensityPollResponse()`
   - Tests for all three intensity levels
   - Tests for boundary conditions (can't decrease from low, can't increase from high)
   - Tests for "Keep current" option at all levels
   - Tests for non-intensity responses

2. **scheduler_tool_test.go**: Updated tests for daily prompt flow
   - Tests now account for 2 messages (prompt + intensity poll)
   - Reminder flow tests updated to expect 3 messages total

3. **whatsapp_service_test.go**: Messaging service tests pass
   - Mock client implements `SendIntensityAdjustmentPoll()`

### Running Tests

```bash
# All tests
go test ./...

# Intensity-specific tests
go test ./internal/whatsapp/... -run TestIntensity
go test ./internal/whatsapp/... -run TestParse

# Flow tests
go test ./internal/flow/...
```

## Future Enhancements

1. **Intensity-Based Behavior**: Currently, the intensity value is stored but not actively used to modify prompt frequency. Future enhancements could:
   - Adjust the time between prompts based on intensity
   - Modify the tone/urgency of messages
   - Control follow-up reminder frequency

2. **Analytics**: Track intensity changes over time to:
   - Identify user engagement patterns
   - Detect when users are struggling (frequent decreases)
   - Celebrate progress (increases in intensity)

3. **Personalization**: Use intensity as a signal for:
   - LLM prompt customization
   - Adaptive scheduling (more/fewer prompts)
   - Recommendation of habit difficulty levels

## Design Decisions

### Why Once Per Day?

- Prevents user fatigue from excessive polling
- Aligns with the daily habit tracking cycle
- Gives users time to experience their current intensity level before adjusting

### Why Smart Options?

- Reduces cognitive load by not showing impossible options
- Prevents confusion from selecting unavailable choices
- Creates a cleaner, more intuitive user experience

### Why No System Prompt Changes?

- The intensity value is passed to LLMs through the profile
- LLMs can naturally incorporate intensity into their messaging
- Keeps the system flexible and adaptable
- Avoids hardcoded intensity-based logic in prompts

## Summary

The intensity adjustment feature provides a user-friendly way to control prompt frequency through poll-based interaction. It's fully integrated with the existing profile system, properly tested, and designed for future extensibility.
