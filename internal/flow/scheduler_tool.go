// Package flow provides scheduler tool functionality for conversation flows.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// RandomScheduleInfo holds information for random scheduling between start and end times
type RandomScheduleInfo struct {
	StartSchedule *models.Schedule // Schedule for the start of the random window
	EndSchedule   *models.Schedule // Schedule for the end of the random window
	StartTime     time.Time        // Parsed start time (used for calculations)
	EndTime       time.Time        // Parsed end time (used for calculations)
	Timezone      string           // Timezone for the schedule
}

// SchedulerTool provides LLM tool functionality for scheduling daily habit prompts.
type SchedulerTool struct {
	timer       models.Timer
	msgService  MessagingService
	genaiClient genai.ClientInterface // For generating scheduled message content
}

// MessagingService defines the interface for messaging operations needed by the scheduler.
type MessagingService interface {
	ValidateAndCanonicalizeRecipient(recipient string) (string, error)
	SendMessage(ctx context.Context, to, message string) error
}

// NewSchedulerTool creates a new scheduler tool instance.
func NewSchedulerTool(timer models.Timer, msgService MessagingService) *SchedulerTool {
	return &SchedulerTool{
		timer:      timer,
		msgService: msgService,
	}
}

// NewSchedulerToolWithGenAI creates a new scheduler tool instance with GenAI support.
func NewSchedulerToolWithGenAI(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface) *SchedulerTool {
	return &SchedulerTool{
		timer:       timer,
		msgService:  msgService,
		genaiClient: genaiClient,
	}
}

// GetToolDefinition returns the OpenAI tool definition for the scheduler.
func (st *SchedulerTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "scheduler",
			Description: openai.String("Schedule daily habit reminder messages for users based on their preferences"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fixed", "random"},
						"description": "Type of scheduling: 'fixed' for same time daily, 'random' for random time within a window",
					},
					"fixed_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "Fixed time in HH:MM format (24-hour) when type is 'fixed'",
					},
					"timezone": map[string]interface{}{
						"type":        "string",
						"description": "Timezone for scheduling (e.g., 'America/Toronto', 'UTC'). Defaults to 'UTC' if not specified",
					},
					"random_start_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "Start time of random window in HH:MM format when type is 'random'",
					},
					"random_end_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "End time of random window in HH:MM format when type is 'random'",
					},
					"prompt_system_prompt": map[string]interface{}{
						"type":        "string",
						"description": "System prompt that defines the AI's role and behavior for daily messages",
					},
					"prompt_user_prompt": map[string]interface{}{
						"type":        "string",
						"description": "User prompt template that guides the content of daily messages",
					},
					"habit_description": map[string]interface{}{
						"type":        "string",
						"description": "Description of the user's chosen habit for personalization of messages",
					},
				},
				"required": []string{"type", "prompt_system_prompt", "prompt_user_prompt"},
			},
		},
	}
}

// ExecuteScheduler executes the scheduler tool call.
func (st *SchedulerTool) ExecuteScheduler(ctx context.Context, participantID string, params models.SchedulerToolParams) (*models.ToolResult, error) {
	slog.Info("Executing scheduler tool", "participantID", participantID, "type", params.Type)

	// Validate parameters
	if err := params.Validate(); err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Invalid scheduler parameters",
			Error:   err.Error(),
		}, nil
	}

	// Get the participant's phone number from participantID
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("SchedulerTool ExecuteScheduler phone number check",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber)
	if !hasPhone {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to get participant phone number",
			Error:   "Participant phone number is required for scheduling",
		}, nil
	}

	// Handle scheduling based on type
	var timerID string
	var scheduleDescription string

	switch params.Type {
	case models.SchedulerTypeFixed:
		schedule, err := st.buildFixedSchedule(params.FixedTime, params.Timezone)
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to create fixed schedule",
				Error:   err.Error(),
			}, nil
		}

		// Create the scheduled prompt
		prompt := models.Prompt{
			To:           phoneNumber,
			Type:         models.PromptTypeGenAI,
			SystemPrompt: params.PromptSystemPrompt,
			UserPrompt:   params.PromptUserPrompt,
			Schedule:     schedule,
		}

		// Schedule the prompt using the timer
		timerID, err = st.timer.ScheduleWithSchedule(schedule, func() {
			st.executeScheduledPrompt(ctx, prompt)
		})
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to schedule prompt",
				Error:   err.Error(),
			}, nil
		}

		timezone := params.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		scheduleDescription = fmt.Sprintf("daily at %s (%s)", params.FixedTime, timezone)

	case models.SchedulerTypeRandom:
		randomInfo, err := st.buildRandomScheduleInfo(params.RandomStartTime, params.RandomEndTime, params.Timezone)
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to create random schedule",
				Error:   err.Error(),
			}, nil
		}

		// Create the scheduled prompt
		prompt := models.Prompt{
			To:           phoneNumber,
			Type:         models.PromptTypeGenAI,
			SystemPrompt: params.PromptSystemPrompt,
			UserPrompt:   params.PromptUserPrompt,
		}

		// Schedule the recurring timer at the start time that will create random one-time timers
		timerID, err = st.timer.ScheduleWithSchedule(randomInfo.StartSchedule, func() {
			st.executeRandomScheduledPrompt(ctx, prompt, randomInfo)
		})
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to schedule random prompt",
				Error:   err.Error(),
			}, nil
		}

		timezone := params.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		scheduleDescription = fmt.Sprintf("daily between %s and %s (%s)", params.RandomStartTime, params.RandomEndTime, timezone)

	default:
		return &models.ToolResult{
			Success: false,
			Message: "Invalid scheduler type",
			Error:   fmt.Sprintf("unsupported scheduler type: %s", params.Type),
		}, nil
	}

	successMessage := fmt.Sprintf("✅ Perfect! I've scheduled your daily habit reminders to be sent %s. You'll receive personalized messages about: %s\n\nYour reminders are all set and will kick off tomorrow! If you'd like to try out a personalized message right now, just say the word.",
		scheduleDescription, params.HabitDescription)

	slog.Info("Scheduler tool executed successfully", "participantID", participantID, "timerID", timerID, "schedule", scheduleDescription)

	return &models.ToolResult{
		Success: true,
		Message: successMessage,
	}, nil
}

// buildFixedSchedule converts a fixed time to a Schedule.
func (st *SchedulerTool) buildFixedSchedule(fixedTime, timezone string) (*models.Schedule, error) {
	// Parse the time
	t, err := time.Parse("15:04", fixedTime)
	if err != nil {
		return nil, fmt.Errorf("invalid time format: %w", err)
	}

	// Create schedule for daily execution at the specified time
	hour := t.Hour()
	minute := t.Minute()

	schedule := &models.Schedule{
		Hour:   &hour,
		Minute: &minute,
	}

	// Set timezone if provided
	if timezone != "" {
		schedule.Timezone = timezone
	}

	return schedule, nil
}

// buildRandomScheduleInfo creates a RandomScheduleInfo for random timing.
// This implementation creates schedules for both start and end times of the interval
// to be used for proper random scheduling.
func (st *SchedulerTool) buildRandomScheduleInfo(startTime, endTime, timezone string) (*RandomScheduleInfo, error) {
	// Parse start and end times
	start, err := time.Parse("15:04", startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time format: %w", err)
	}

	end, err := time.Parse("15:04", endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time format: %w", err)
	}

	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	// Create schedules for both start and end times
	startHour := start.Hour()
	startMinute := start.Minute()
	startSchedule := &models.Schedule{
		Hour:   &startHour,
		Minute: &startMinute,
	}

	endHour := end.Hour()
	endMinute := end.Minute()
	endSchedule := &models.Schedule{
		Hour:   &endHour,
		Minute: &endMinute,
	}

	// Set timezone if provided
	if timezone != "" {
		startSchedule.Timezone = timezone
		endSchedule.Timezone = timezone
	}

	return &RandomScheduleInfo{
		StartSchedule: startSchedule,
		EndSchedule:   endSchedule,
		StartTime:     start,
		EndTime:       end,
		Timezone:      timezone,
	}, nil
}

// executeRandomScheduledPrompt handles the random scheduling logic.
// This method is called by the recurring timer at the start time and schedules
// a one-time timer at a random time within the specified interval.
func (st *SchedulerTool) executeRandomScheduledPrompt(ctx context.Context, prompt models.Prompt, randomInfo *RandomScheduleInfo) {
	slog.Debug("Executing random scheduled prompt logic", "to", prompt.To, "startTime", randomInfo.StartTime.Format("15:04"), "endTime", randomInfo.EndTime.Format("15:04"))

	// Calculate random delay from start time to end time
	startMinutes := randomInfo.StartTime.Hour()*60 + randomInfo.StartTime.Minute()
	endMinutes := randomInfo.EndTime.Hour()*60 + randomInfo.EndTime.Minute()

	if endMinutes <= startMinutes {
		// Handle case where end time is the next day (e.g., 23:00 to 01:00)
		endMinutes += 24 * 60
	}

	// Generate random minutes within the interval
	intervalMinutes := endMinutes - startMinutes
	if intervalMinutes <= 0 {
		slog.Error("Invalid time interval for random scheduling", "startMinutes", startMinutes, "endMinutes", endMinutes)
		// Fallback to immediate execution
		st.executeScheduledPrompt(ctx, prompt)
		return
	}

	// Generate random offset within the interval using math/rand
	randomDelayMinutes := rand.Intn(intervalMinutes)
	randomDelay := time.Duration(randomDelayMinutes) * time.Minute

	slog.Info("Scheduling random prompt",
		"to", prompt.To,
		"intervalMinutes", intervalMinutes,
		"randomDelayMinutes", randomDelayMinutes,
		"delay", randomDelay)

	// Schedule a one-time timer for the random time
	_, err := st.timer.ScheduleAfter(randomDelay, func() {
		st.executeScheduledPrompt(ctx, prompt)
	})

	if err != nil {
		slog.Error("Failed to schedule random one-time timer", "error", err, "to", prompt.To)
		// Fallback to immediate execution
		st.executeScheduledPrompt(ctx, prompt)
	}
}

// executeScheduledPrompt executes a scheduled prompt by generating content and sending it.
func (st *SchedulerTool) executeScheduledPrompt(ctx context.Context, prompt models.Prompt) {
	slog.Debug("Executing scheduled prompt", "to", prompt.To, "type", prompt.Type)

	var message string
	var err error

	// Generate message content based on the prompt type
	if st.genaiClient != nil && prompt.SystemPrompt != "" && prompt.UserPrompt != "" {
		// Use GenAI to generate personalized message content
		message, err = st.genaiClient.GeneratePromptWithContext(ctx, prompt.SystemPrompt, prompt.UserPrompt)
		if err != nil {
			slog.Error("Failed to generate scheduled prompt content", "error", err, "to", prompt.To)
			// Fallback to user prompt if generation fails
			message = prompt.UserPrompt
		}
	} else if prompt.Body != "" {
		// Use static message body
		message = prompt.Body
	} else {
		// Fallback message
		message = "Daily habit reminder - it's time for your healthy habit!"
		slog.Warn("No message content available, using fallback", "to", prompt.To)
	}

	// Send the message
	if err := st.msgService.SendMessage(ctx, prompt.To, message); err != nil {
		slog.Error("Failed to send scheduled prompt", "error", err, "to", prompt.To, "message", message)
		return
	}

	slog.Info("Scheduled prompt sent successfully", "to", prompt.To, "messageLength", len(message))
}
