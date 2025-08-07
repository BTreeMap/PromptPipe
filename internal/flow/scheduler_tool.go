// Package flow provides scheduler tool functionality for conversation flows.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/util"
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
	timer        models.Timer
	msgService   MessagingService
	genaiClient  genai.ClientInterface // For generating scheduled message content
	stateManager StateManager          // For storing schedule metadata
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

// NewSchedulerToolWithStateManager creates a new scheduler tool instance with state management.
func NewSchedulerToolWithStateManager(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface, stateManager StateManager) *SchedulerTool {
	return &SchedulerTool{
		timer:        timer,
		msgService:   msgService,
		genaiClient:  genaiClient,
		stateManager: stateManager,
	}
}

// GetToolDefinition returns the OpenAI tool definition for the scheduler.
func (st *SchedulerTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "scheduler",
			Description: openai.String("Manage daily habit reminder schedules - create new schedules, list existing ones, or delete schedules"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"create", "list", "delete"},
						"description": "Action to perform: 'create' to schedule new reminders, 'list' to show existing schedules, 'delete' to remove a schedule",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fixed", "random"},
						"description": "Type of scheduling: 'fixed' for same time daily, 'random' for random time within a window (required for create action)",
					},
					"fixed_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "Fixed time in HH:MM format (24-hour) when type is 'fixed' (required for create with fixed type)",
					},
					"timezone": map[string]interface{}{
						"type":        "string",
						"description": "Timezone for scheduling (e.g., 'America/Toronto', 'UTC'). Defaults to 'UTC' if not specified (for create action)",
					},
					"random_start_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "Start time of random window in HH:MM format when type is 'random' (required for create with random type)",
					},
					"random_end_time": map[string]interface{}{
						"type":        "string",
						"pattern":     "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$",
						"description": "End time of random window in HH:MM format when type is 'random' (required for create with random type)",
					},
					"prompt_system_prompt": map[string]interface{}{
						"type":        "string",
						"description": "System prompt that defines the AI's role and behavior for daily messages (required for create action)",
					},
					"prompt_user_prompt": map[string]interface{}{
						"type":        "string",
						"description": "User prompt template that guides the content of daily messages (required for create action)",
					},
					"habit_description": map[string]interface{}{
						"type":        "string",
						"description": "Description of the user's chosen habit for personalization of messages (optional for create action)",
					},
					"schedule_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the schedule to delete (required for delete action)",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

// ExecuteScheduler executes the scheduler tool call.
func (st *SchedulerTool) ExecuteScheduler(ctx context.Context, participantID string, params models.SchedulerToolParams) (*models.ToolResult, error) {
	slog.Info("SchedulerTool.ExecuteScheduler: executing scheduler tool", "participantID", participantID, "action", params.Action)

	// Validate parameters
	if err := params.Validate(); err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Invalid scheduler parameters",
			Error:   err.Error(),
		}, nil
	}

	// Handle different actions
	switch params.Action {
	case models.SchedulerActionCreate:
		return st.executeCreateSchedule(ctx, participantID, params)
	case models.SchedulerActionList:
		return st.executeListSchedules(ctx, participantID)
	case models.SchedulerActionDelete:
		return st.executeDeleteSchedule(ctx, participantID, params.ScheduleID)
	default:
		return &models.ToolResult{
			Success: false,
			Message: "Invalid scheduler action",
			Error:   fmt.Sprintf("unsupported action: %s", params.Action),
		}, nil
	}
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

// executeCreateSchedule creates a new schedule.
func (st *SchedulerTool) executeCreateSchedule(ctx context.Context, participantID string, params models.SchedulerToolParams) (*models.ToolResult, error) {
	// Get the participant's phone number from participantID
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("SchedulerTool.executeCreateSchedule: checking phone number context",
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
			timezone = "America/Toronto" // Default timezone if not specified
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

	// Generate a unique schedule ID
	scheduleID := util.GenerateRandomID("sched_", 16)

	// Store schedule metadata if state manager is available
	if st.stateManager != nil {
		scheduleInfo := models.ScheduleInfo{
			ID:               scheduleID,
			Type:             params.Type,
			FixedTime:        params.FixedTime,
			RandomStartTime:  params.RandomStartTime,
			RandomEndTime:    params.RandomEndTime,
			Timezone:         params.Timezone,
			HabitDescription: params.HabitDescription,
			CreatedAt:        time.Now(),
			TimerID:          timerID,
		}

		if err := st.storeScheduleInfo(ctx, participantID, scheduleInfo); err != nil {
			slog.Warn("Failed to store schedule metadata", "participantID", participantID, "scheduleID", scheduleID, "error", err)
			// Don't fail the whole operation - the schedule is still active
		}
	}

	successMessage := fmt.Sprintf("✅ Perfect! I've scheduled your daily habit reminders to be sent %s. You'll receive personalized messages about: %s\n\nYour reminders are all set and will kick off tomorrow! Schedule ID: %s\n\nIf you'd like to try out a personalized message right now, just say the word.",
		scheduleDescription, params.HabitDescription, scheduleID)

	slog.Info("Scheduler tool executed successfully", "participantID", participantID, "timerID", timerID, "scheduleID", scheduleID, "schedule", scheduleDescription)

	return &models.ToolResult{
		Success: true,
		Message: successMessage,
	}, nil
}

// executeListSchedules lists all active schedules for a participant.
func (st *SchedulerTool) executeListSchedules(ctx context.Context, participantID string) (*models.ToolResult, error) {
	if st.stateManager == nil {
		return &models.ToolResult{
			Success: false,
			Message: "Schedule listing is not available",
			Error:   "State manager not configured",
		}, nil
	}

	schedules, err := st.getStoredSchedules(ctx, participantID)
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to retrieve schedules",
			Error:   err.Error(),
		}, nil
	}

	if len(schedules) == 0 {
		return &models.ToolResult{
			Success: true,
			Message: "📅 You don't have any active schedules yet. Use the scheduler to create your first habit reminder!",
			Data:    []models.ScheduleInfo{},
		}, nil
	}

	// Build a human-readable summary
	var scheduleLines []string
	for i, schedule := range schedules {
		var timeDesc string
		if schedule.Type == models.SchedulerTypeFixed {
			timezone := schedule.Timezone
			if timezone == "" {
				timezone = "UTC"
			}
			timeDesc = fmt.Sprintf("daily at %s (%s)", schedule.FixedTime, timezone)
		} else {
			timezone := schedule.Timezone
			if timezone == "" {
				timezone = "UTC"
			}
			timeDesc = fmt.Sprintf("daily between %s and %s (%s)", schedule.RandomStartTime, schedule.RandomEndTime, timezone)
		}

		habit := schedule.HabitDescription
		if habit == "" {
			habit = "habit reminder"
		}

		scheduleLines = append(scheduleLines, fmt.Sprintf("%d. **%s** - %s (ID: %s)", i+1, habit, timeDesc, schedule.ID))
	}

	message := fmt.Sprintf("📅 Your active schedules:\n\n%s\n\nTo remove a schedule, use the scheduler with the delete action and specify the schedule ID.", strings.Join(scheduleLines, "\n"))

	return &models.ToolResult{
		Success: true,
		Message: message,
		Data:    schedules,
	}, nil
}

// executeDeleteSchedule deletes a specific schedule.
func (st *SchedulerTool) executeDeleteSchedule(ctx context.Context, participantID, scheduleID string) (*models.ToolResult, error) {
	if st.stateManager == nil {
		return &models.ToolResult{
			Success: false,
			Message: "Schedule deletion is not available",
			Error:   "State manager not configured",
		}, nil
	}

	// Get existing schedules
	schedules, err := st.getStoredSchedules(ctx, participantID)
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to retrieve schedules",
			Error:   err.Error(),
		}, nil
	}

	// Find the schedule to delete
	var scheduleToDelete *models.ScheduleInfo
	var remainingSchedules []models.ScheduleInfo

	for _, schedule := range schedules {
		if schedule.ID == scheduleID {
			scheduleToDelete = &schedule
		} else {
			remainingSchedules = append(remainingSchedules, schedule)
		}
	}

	if scheduleToDelete == nil {
		return &models.ToolResult{
			Success: false,
			Message: fmt.Sprintf("Schedule with ID '%s' not found", scheduleID),
			Error:   "Schedule not found",
		}, nil
	}

	// Cancel the associated timer
	if scheduleToDelete.TimerID != "" {
		if err := st.timer.Cancel(scheduleToDelete.TimerID); err != nil {
			slog.Warn("Failed to cancel timer for schedule", "scheduleID", scheduleID, "timerID", scheduleToDelete.TimerID, "error", err)
			// Don't fail the whole operation - continue with deletion
		}
	}

	// Update stored schedules
	if err := st.storeSchedules(ctx, participantID, remainingSchedules); err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to delete schedule",
			Error:   err.Error(),
		}, nil
	}

	habit := scheduleToDelete.HabitDescription
	if habit == "" {
		habit = "habit reminder"
	}

	message := fmt.Sprintf("✅ Successfully deleted the schedule for '%s' (ID: %s). Your daily reminders for this habit have been stopped.", habit, scheduleID)

	slog.Info("Schedule deleted successfully", "participantID", participantID, "scheduleID", scheduleID, "timerID", scheduleToDelete.TimerID)

	return &models.ToolResult{
		Success: true,
		Message: message,
	}, nil
}

// storeScheduleInfo stores a single schedule in the participant's schedule registry.
func (st *SchedulerTool) storeScheduleInfo(ctx context.Context, participantID string, schedule models.ScheduleInfo) error {
	schedules, err := st.getStoredSchedules(ctx, participantID)
	if err != nil {
		// If no schedules exist yet, start with empty list
		schedules = []models.ScheduleInfo{}
	}

	// Add the new schedule
	schedules = append(schedules, schedule)

	return st.storeSchedules(ctx, participantID, schedules)
}

// getStoredSchedules retrieves all schedules for a participant.
func (st *SchedulerTool) getStoredSchedules(ctx context.Context, participantID string) ([]models.ScheduleInfo, error) {
	registryJSON, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyScheduleRegistry)
	if err != nil || registryJSON == "" {
		return []models.ScheduleInfo{}, nil // Empty list if no schedules exist
	}

	var schedules []models.ScheduleInfo
	if err := json.Unmarshal([]byte(registryJSON), &schedules); err != nil {
		return nil, fmt.Errorf("failed to parse schedule registry: %w", err)
	}

	return schedules, nil
}

// storeSchedules stores the complete list of schedules for a participant.
func (st *SchedulerTool) storeSchedules(ctx context.Context, participantID string, schedules []models.ScheduleInfo) error {
	schedulesJSON, err := json.Marshal(schedules)
	if err != nil {
		return fmt.Errorf("failed to marshal schedules: %w", err)
	}

	return st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyScheduleRegistry, string(schedulesJSON))
}
