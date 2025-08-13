# Unified Scheduler Tool Design

## Overview

The SchedulerTool has been redesigned to use a unified "preparation time" approach for both fixed and random scheduling types. This simplifies the internal logic while maintaining backward compatibility with the existing LLM API.

## Key Changes

### 1. Unified Scheduling Logic

**Before**:

- Fixed scheduling: Sent messages at exact specified time
- Random scheduling: Sent messages at random time within specified window

**After**:

- Both types: Send preparation messages 10 minutes before the target habit time
- Fixed scheduling: Uses the specified time as target
- Random scheduling: Uses the start time of the window as target

### 2. Same-Day Scheduling Support

The scheduler now intelligently determines whether to schedule for today or tomorrow:

- If current time is before (target time - prep time), schedule for today
- Otherwise, schedule for tomorrow and recurring daily

**Example**:

- Current time: 1:30 PM
- Target time: 2:00 PM  
- Prep time: 10 minutes
- Result: Schedule notification for 1:50 PM today, then daily at 1:50 PM

### 3. Enhanced User Experience

Users now receive preparation messages that help them mentally prepare for their habit:

```
"Your 8:00 AM meditation session is starting in 10 minutes. 
Take a moment to prepare your space and mind."
```

## Implementation Details

### Core Components

#### 1. SchedulerTool Struct

```go
type SchedulerTool struct {
    timer           models.Timer
    msgService      MessagingService
    genaiClient     genai.ClientInterface
    stateManager    StateManager
    promptGenerator PromptGeneratorService
    prepTimeMinutes int  // New: Default 10 minutes
}
```

#### 2. Key Helper Methods

**determineTargetTime()**: Extracts target habit time from parameters

- Fixed type: Uses `fixed_time`
- Random type: Uses `random_start_time` as target

**shouldScheduleToday()**: Determines same-day vs next-day scheduling

- Calculates notification time (target - prep time)
- Checks if notification time is still in the future

**buildSchedule()**: Unified schedule building for both types

- Handles same-day scheduling with delays
- Creates recurring schedules for daily execution

### 3. Updated Tool Definition

The LLM interface remains unchanged but now includes preparation time messaging:

```json
{
  "name": "scheduler",
  "description": "Manage daily habit reminder schedules. The scheduler sends preparation notifications 10 minutes before the scheduled time to help users mentally prepare for their habit.",
  "parameters": {
    // Same parameters as before
    "type": {"enum": ["fixed", "random"]},
    "fixed_time": {"pattern": "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$"},
    "random_start_time": {"pattern": "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$"},
    "random_end_time": {"pattern": "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$"}
  }
}
```

## API Compatibility

### Backward Compatibility

The external API remains 100% compatible:

```go
// Fixed scheduling (unchanged)
params := models.SchedulerToolParams{
    Action:    "create",
    Type:      "fixed", 
    FixedTime: "08:00",
    Timezone:  "America/Toronto"
}

// Random scheduling (unchanged) 
params := models.SchedulerToolParams{
    Action:          "create",
    Type:            "random",
    RandomStartTime: "08:00", 
    RandomEndTime:   "10:00",
    Timezone:        "America/Toronto"
}
```

### Internal Behavior Changes

**Fixed Scheduling Example**:

- Input: `fixed_time: "08:00"`
- Old behavior: Message sent at 8:00 AM
- New behavior: Prep message sent at 7:50 AM

**Random Scheduling Example**:

- Input: `random_start_time: "08:00", random_end_time: "10:00"`
- Old behavior: Random message between 8:00-10:00 AM
- New behavior: Prep message sent at 7:50 AM (10 min before 8:00 AM)

## Testing & Debugging

### Same-Day Scheduling for Testing

The unified approach makes testing much easier:

```bash
# Schedule for 15 minutes from now
current_time=$(date '+%H:%M')
target_time=$(date -d '+15 minutes' '+%H:%M')

curl -X POST /scheduler \
  -d "{\"action\":\"create\", \"type\":\"fixed\", \"fixed_time\":\"$target_time\"}"

# Will get prep message in ~5 minutes (15min - 10min prep time)
```

### Debug Scenarios

1. **Same-day scheduling**: Schedule 30 minutes from now
2. **Next-day scheduling**: Schedule 5 minutes from now (will go to tomorrow)
3. **Timezone testing**: Use different timezones with same local time

## Message Flow

### 1. Schedule Creation Flow

```
User Request → LLM → SchedulerTool.ExecuteScheduler() 
↓
determineTargetTime() → shouldScheduleToday() → buildSchedule()
↓  
Timer.ScheduleAfter() OR Timer.ScheduleWithSchedule()
↓
Success Response with prep time explanation
```

### 2. Message Delivery Flow

```
Timer Triggers → executeScheduledPrompt()
↓
PromptGenerator.ExecutePromptGenerator() (if available)
OR GenAI.GeneratePromptWithContext() (fallback)
↓
MessagingService.SendMessage()
```

## Configuration

### Default Settings

- **Prep Time**: 10 minutes before target time
- **Timezone**: "America/Toronto" for fixed, "UTC" for random
- **Fallback Message**: "Daily habit reminder - it's time for your healthy habit!"

### Customization

```go
// Custom prep time
scheduler := NewSchedulerToolWithPrepTime(timer, msgService, genai, state, prompt, 15)

// Environment-based configuration  
prepTime := getEnvInt("SCHEDULER_PREP_TIME_MINUTES", 10)
```

## Migration Notes

### For Existing Deployments

1. **Existing schedules**: Continue to work with old behavior until recreated
2. **New schedules**: Automatically use new unified approach
3. **User communication**: Inform users about the prep time feature

### Database Changes

No database schema changes required. The `ScheduleInfo` model remains the same:

```go
type ScheduleInfo struct {
    ID              string    `json:"id"`
    Type            string    `json:"type"`           // "fixed" or "random"
    FixedTime       string    `json:"fixed_time,omitempty"`
    RandomStartTime string    `json:"random_start_time,omitempty"`  
    RandomEndTime   string    `json:"random_end_time,omitempty"`
    Timezone        string    `json:"timezone"`
    CreatedAt       time.Time `json:"created_at"`
    TimerID         string    `json:"timer_id"`
}
```

## Benefits Summary

1. **Simplified Logic**: Single code path for both scheduling types
2. **Better UX**: Preparation time helps users mentally prepare
3. **Easy Testing**: Same-day scheduling enables rapid feedback
4. **Backward Compatible**: No API changes required
5. **Intuitive Behavior**: Matches user expectations for scheduling

## Future Enhancements

1. **Configurable Prep Time**: Allow users to set their own prep time
2. **Multiple Reminders**: Support for multiple prep notifications
3. **Smart Scheduling**: AI-powered optimal timing based on user behavior
4. **Prep Content**: Customized preparation messages based on habit type
