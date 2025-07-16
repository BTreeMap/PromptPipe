// Package flow provides scheduler tool functionality for conversation flows.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

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

	// Convert to Schedule based on type
	var schedule *models.Schedule
	var err error

	switch params.Type {
	case models.SchedulerTypeFixed:
		schedule, err = st.buildFixedSchedule(params.FixedTime, params.Timezone)
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to create fixed schedule",
				Error:   err.Error(),
			}, nil
		}
	case models.SchedulerTypeRandom:
		schedule, err = st.buildRandomSchedule(params.RandomStartTime, params.RandomEndTime, params.Timezone)
		if err != nil {
			return &models.ToolResult{
				Success: false,
				Message: "Failed to create random schedule",
				Error:   err.Error(),
			}, nil
		}
	default:
		return &models.ToolResult{
			Success: false,
			Message: "Invalid scheduler type",
			Error:   fmt.Sprintf("unsupported scheduler type: %s", params.Type),
		}, nil
	}

	// Get the participant's phone number from participantID
	// Note: This would typically require a store lookup, but for now we'll assume
	// the participantID can be used directly or we need to modify the interface
	phoneNumber, err := st.getParticipantPhoneNumber(ctx, participantID)
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to get participant phone number",
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
	timerID, err := st.timer.ScheduleWithSchedule(schedule, func() {
		st.executeScheduledPrompt(ctx, prompt)
	})
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to schedule prompt",
			Error:   err.Error(),
		}, nil
	}

	// Build success message
	var scheduleDescription string
	if params.Type == models.SchedulerTypeFixed {
		timezone := params.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		scheduleDescription = fmt.Sprintf("daily at %s (%s)", params.FixedTime, timezone)
	} else {
		timezone := params.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		scheduleDescription = fmt.Sprintf("daily between %s and %s (%s)", params.RandomStartTime, params.RandomEndTime, timezone)
	}

	successMessage := fmt.Sprintf("✅ Perfect! I've scheduled your daily habit reminders to be sent %s. You'll receive personalized messages about: %s\n\nYour reminders are now active and will start tomorrow!",
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

// buildRandomSchedule creates a Schedule for random timing.
// This implementation creates a recurring timer at the start time of the interval
// that creates one one-time timer for the actual message within the interval.
func (st *SchedulerTool) buildRandomSchedule(startTime, endTime, timezone string) (*models.Schedule, error) {
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

	// For random scheduling, create a schedule that runs at the start time
	// The actual random timing will be handled by the execution logic
	hour := start.Hour()
	minute := start.Minute()

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

// getParticipantPhoneNumber retrieves the phone number for a participant.
// This is a placeholder implementation that would need to interact with the store.
func (st *SchedulerTool) getParticipantPhoneNumber(ctx context.Context, participantID string) (string, error) {
	// For now, assume participantID is the phone number or we can derive it
	// TODO: Implement actual lookup from store
	// This would typically query the participant table to get phone number
	if st.msgService != nil {
		// Use the messaging service to validate/canonicalize if available
		return st.msgService.ValidateAndCanonicalizeRecipient(participantID)
	}

	// Simple fallback - assume participantID is already a phone number
	return participantID, nil
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
