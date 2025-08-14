// Package flow provides scheduler tool functionality for conversation flows.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/util"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// SchedulerTool provides LLM tool functionality for scheduling daily habit prompts.
// This unified implementation uses a preparation time approach for both fixed and random scheduling.
type SchedulerTool struct {
	timer           models.Timer
	msgService      MessagingService
	genaiClient     genai.ClientInterface  // For generating scheduled message content
	stateManager    StateManager           // For storing schedule metadata
	promptGenerator PromptGeneratorService // For generating habit prompts
	prepTimeMinutes int                    // Preparation time in minutes (default 10)
}

// MessagingService defines the interface for messaging operations needed by the scheduler.
type MessagingService interface {
	ValidateAndCanonicalizeRecipient(recipient string) (string, error)
	SendMessage(ctx context.Context, to, message string) error
}

// PromptGeneratorService defines the interface for prompt generation operations.
type PromptGeneratorService interface {
	ExecutePromptGenerator(ctx context.Context, participantID string, args map[string]interface{}) (string, error)
}

// NewSchedulerTool creates a new scheduler tool instance with default 10-minute preparation time.
func NewSchedulerTool(timer models.Timer, msgService MessagingService) *SchedulerTool {
	return &SchedulerTool{
		timer:           timer,
		msgService:      msgService,
		prepTimeMinutes: 10, // Default 10 minutes preparation time
	}
}

// NewSchedulerToolWithGenAI creates a new scheduler tool instance with GenAI support.
func NewSchedulerToolWithGenAI(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface) *SchedulerTool {
	return &SchedulerTool{
		timer:           timer,
		msgService:      msgService,
		genaiClient:     genaiClient,
		prepTimeMinutes: 10, // Default 10 minutes preparation time
	}
}

// NewSchedulerToolWithStateManager creates a new scheduler tool instance with state management.
func NewSchedulerToolWithStateManager(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface, stateManager StateManager) *SchedulerTool {
	return &SchedulerTool{
		timer:           timer,
		msgService:      msgService,
		genaiClient:     genaiClient,
		stateManager:    stateManager,
		prepTimeMinutes: 10, // Default 10 minutes preparation time
	}
}

// NewSchedulerToolComplete creates a new scheduler tool instance with all dependencies.
func NewSchedulerToolComplete(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface, stateManager StateManager, promptGenerator PromptGeneratorService) *SchedulerTool {
	return &SchedulerTool{
		timer:           timer,
		msgService:      msgService,
		genaiClient:     genaiClient,
		stateManager:    stateManager,
		promptGenerator: promptGenerator,
		prepTimeMinutes: 10, // Default 10 minutes preparation time
	}
}

// NewSchedulerToolWithPrepTime creates a new scheduler tool instance with custom preparation time.
func NewSchedulerToolWithPrepTime(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface, stateManager StateManager, promptGenerator PromptGeneratorService, prepTimeMinutes int) *SchedulerTool {
	return &SchedulerTool{
		timer:           timer,
		msgService:      msgService,
		genaiClient:     genaiClient,
		stateManager:    stateManager,
		promptGenerator: promptGenerator,
		prepTimeMinutes: prepTimeMinutes,
	}
}

// GetToolDefinition returns the OpenAI tool definition for the scheduler.
func (st *SchedulerTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "scheduler",
			Description: openai.String(fmt.Sprintf("Manage daily habit reminder schedules. The scheduler sends preparation notifications %d minutes before the scheduled time to help users mentally prepare for their habit.", st.prepTimeMinutes)),
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
						"description": "Timezone for scheduling (e.g., 'America/Toronto', 'UTC'). Defaults to 'America/Toronto' if not specified (for create action)",
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

// determineTargetTime extracts the target habit time from parameters.
// For fixed type: uses fixed_time
// For random type: uses random_start_time as target
func (st *SchedulerTool) determineTargetTime(params models.SchedulerToolParams) (time.Time, error) {
	var timeStr string

	switch params.Type {
	case models.SchedulerTypeFixed:
		timeStr = params.FixedTime
	case models.SchedulerTypeRandom:
		timeStr = params.RandomStartTime
	default:
		return time.Time{}, fmt.Errorf("invalid scheduler type: %s", params.Type)
	}

	targetTime, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format: %w", err)
	}

	return targetTime, nil
}

// shouldScheduleToday determines if we should schedule for today or tomorrow.
// Returns true if current time is before (target time - prep time)
func (st *SchedulerTool) shouldScheduleToday(targetTime time.Time) bool {
	now := time.Now()

	// Create today's target time in current timezone
	todayTarget := time.Date(now.Year(), now.Month(), now.Day(),
		targetTime.Hour(), targetTime.Minute(), 0, 0, now.Location())

	// Calculate notification time (target - prep time)
	notificationTime := todayTarget.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)

	// Schedule for today if notification time is still in the future
	return now.Before(notificationTime)
}

// buildSchedule creates a unified schedule for both fixed and random types.
// Handles same-day scheduling with delays and creates recurring schedules for daily execution.
func (st *SchedulerTool) buildSchedule(targetTime time.Time, timezone string) (*models.Schedule, time.Duration, error) {
	// Calculate notification time (target - prep time)
	notificationTime := targetTime.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)

	hour := notificationTime.Hour()
	minute := notificationTime.Minute()

	schedule := &models.Schedule{
		Hour:   &hour,
		Minute: &minute,
	}

	// Set timezone
	if timezone != "" {
		schedule.Timezone = timezone
	}

	// Calculate delay for same-day scheduling
	var delay time.Duration
	if st.shouldScheduleToday(targetTime) {
		now := time.Now()
		todayNotification := time.Date(now.Year(), now.Month(), now.Day(),
			hour, minute, 0, 0, now.Location())
		delay = todayNotification.Sub(now)

		// Ensure delay is positive
		if delay < 0 {
			delay = 0
		}
	}

	return schedule, delay, nil
}

// executeScheduledPrompt executes a scheduled prompt by calling the prompt generator tool.
func (st *SchedulerTool) executeScheduledPrompt(ctx context.Context, participantID string, prompt models.Prompt) {
	slog.Debug("SchedulerTool.executeScheduledPrompt: executing scheduled prompt", "participantID", participantID, "to", prompt.To, "type", prompt.Type)

	var message string
	var err error

	// Use prompt generator if available
	if st.promptGenerator != nil {
		// Call prompt generator with "scheduled" delivery mode
		args := map[string]interface{}{
			"delivery_mode": "scheduled",
		}

		message, err = st.promptGenerator.ExecutePromptGenerator(ctx, participantID, args)
		if err != nil {
			slog.Error("SchedulerTool.executeScheduledPrompt: failed to generate content with prompt generator", "error", err, "to", prompt.To)
			// Fallback to generic message
			message = "Daily habit reminder - it's time for your healthy habit!"
		}
	} else if st.genaiClient != nil && prompt.SystemPrompt != "" && prompt.UserPrompt != "" {
		// Legacy fallback: Use GenAI to generate personalized message content
		message, err = st.genaiClient.GeneratePromptWithContext(ctx, prompt.SystemPrompt, prompt.UserPrompt)
		if err != nil {
			slog.Error("SchedulerTool.executeScheduledPrompt: failed to generate content", "error", err, "to", prompt.To)
			// Fallback to user prompt if generation fails
			message = prompt.UserPrompt
		}
	} else if prompt.Body != "" {
		// Use static message body
		message = prompt.Body
	} else {
		// Fallback message
		message = "Daily habit reminder - it's time for your healthy habit!"
		slog.Warn("SchedulerTool.executeScheduledPrompt: no message content available, using fallback", "to", prompt.To)
	}

	// Send the message
	if err := st.msgService.SendMessage(ctx, prompt.To, message); err != nil {
		slog.Error("SchedulerTool.executeScheduledPrompt: failed to send message", "error", err, "to", prompt.To, "message", message)
		return
	}

	slog.Info("SchedulerTool.executeScheduledPrompt: message sent successfully", "to", prompt.To, "messageLength", len(message))
}

// executeCreateSchedule creates a new schedule using the unified approach.
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

	// Determine target time using unified approach
	targetTime, err := st.determineTargetTime(params)
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to parse target time",
			Error:   err.Error(),
		}, nil
	}

	// Set default timezone
	timezone := params.Timezone
	if timezone == "" {
		if params.Type == models.SchedulerTypeFixed {
			timezone = "America/Toronto"
		} else {
			timezone = "UTC"
		}
	}

	// Build unified schedule
	schedule, delay, err := st.buildSchedule(targetTime, timezone)
	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to create schedule",
			Error:   err.Error(),
		}, nil
	}

	// Create the scheduled prompt
	prompt := models.Prompt{
		To:   phoneNumber,
		Type: models.PromptTypeGenAI,
	}

	var timerID string
	var scheduleDescription string

	// Handle same-day scheduling vs recurring scheduling
	if st.shouldScheduleToday(targetTime) {
		// Same-day scheduling: schedule immediate delayed timer + recurring schedule
		if delay > 0 {
			// Schedule immediate delayed execution for today
			_, err = st.timer.ScheduleAfter(delay, func() {
				st.executeScheduledPrompt(ctx, participantID, prompt)
			})
			if err != nil {
				slog.Warn("SchedulerTool.executeCreateSchedule: failed to schedule same-day timer", "error", err)
			} else {
				slog.Info("SchedulerTool.executeCreateSchedule: same-day timer scheduled", "delay", delay)
			}
		}

		// Also set up recurring schedule starting tomorrow
		timerID, err = st.timer.ScheduleWithSchedule(schedule, func() {
			st.executeScheduledPrompt(ctx, participantID, prompt)
		})
	} else {
		// Next-day scheduling: just create recurring schedule
		timerID, err = st.timer.ScheduleWithSchedule(schedule, func() {
			st.executeScheduledPrompt(ctx, participantID, prompt)
		})
	}

	if err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Failed to schedule prompt",
			Error:   err.Error(),
		}, nil
	}

	// Build schedule description
	if params.Type == models.SchedulerTypeFixed {
		scheduleDescription = fmt.Sprintf("daily at %s (%s) with preparation message %d minutes before",
			params.FixedTime, timezone, st.prepTimeMinutes)
	} else {
		scheduleDescription = fmt.Sprintf("daily preparation message at %s (%s) for habit window %s-%s",
			params.RandomStartTime, timezone, params.RandomStartTime, params.RandomEndTime)
	}

	// Generate a unique schedule ID
	scheduleID := util.GenerateRandomID("sched_", 16)

	// Store schedule metadata if state manager is available
	if st.stateManager != nil {
		scheduleInfo := models.ScheduleInfo{
			ID:              scheduleID,
			Type:            params.Type,
			FixedTime:       params.FixedTime,
			RandomStartTime: params.RandomStartTime,
			RandomEndTime:   params.RandomEndTime,
			Timezone:        timezone,
			CreatedAt:       time.Now(),
			TimerID:         timerID,
		}

		if err := st.storeScheduleInfo(ctx, participantID, scheduleInfo); err != nil {
			slog.Warn("SchedulerTool.executeCreateSchedule: failed to store schedule metadata", "participantID", participantID, "scheduleID", scheduleID, "error", err)
			// Don't fail the whole operation - the schedule is still active
		}
	}

	// Create success message with preparation time explanation
	var timeExplanation string
	if params.Type == models.SchedulerTypeFixed {
		prepTime := targetTime.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)
		timeExplanation = fmt.Sprintf("Your %s habit reminder is now scheduled! You'll receive preparation messages at %s (%d minutes before your %s habit time) to help you mentally prepare.",
			params.FixedTime, prepTime.Format("15:04"), st.prepTimeMinutes, params.FixedTime)
	} else {
		prepTime := targetTime.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)
		timeExplanation = fmt.Sprintf("Your habit reminder is now scheduled! You'll receive preparation messages at %s (%d minutes before your %s-%s habit window) to help you mentally prepare.",
			prepTime.Format("15:04"), st.prepTimeMinutes, params.RandomStartTime, params.RandomEndTime)
	}

	successMessage := fmt.Sprintf("✅ Perfect! %s\n\n%s\n\n🆔 **Schedule ID: %s** (save this for future reference)\n\nYour reminders will start %s!",
		timeExplanation,
		scheduleDescription,
		scheduleID,
		func() string {
			if st.shouldScheduleToday(targetTime) {
				return "today"
			}
			return "tomorrow"
		}())

	slog.Info("SchedulerTool.executeCreateSchedule: schedule created successfully",
		"participantID", participantID,
		"timerID", timerID,
		"scheduleID", scheduleID,
		"schedule", scheduleDescription,
		"prepTimeMinutes", st.prepTimeMinutes)

	return &models.ToolResult{
		Success: true,
		Message: successMessage,
		Data: map[string]interface{}{
			"schedule_id":       scheduleID,
			"type":              params.Type,
			"description":       scheduleDescription,
			"prep_time_minutes": st.prepTimeMinutes,
			"starts_today":      st.shouldScheduleToday(targetTime),
		},
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
			timeDesc = fmt.Sprintf("daily preparation at %s (%s) for %s habit",
				func() string {
					t, _ := time.Parse("15:04", schedule.FixedTime)
					prep := t.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)
					return prep.Format("15:04")
				}(), timezone, schedule.FixedTime)
		} else {
			timezone := schedule.Timezone
			if timezone == "" {
				timezone = "UTC"
			}
			timeDesc = fmt.Sprintf("daily preparation at %s (%s) for %s-%s habit window",
				func() string {
					t, _ := time.Parse("15:04", schedule.RandomStartTime)
					prep := t.Add(-time.Duration(st.prepTimeMinutes) * time.Minute)
					return prep.Format("15:04")
				}(), timezone, schedule.RandomStartTime, schedule.RandomEndTime)
		}

		scheduleLines = append(scheduleLines, fmt.Sprintf("%d. **Daily Habit Reminder** - %s\n   🆔 Schedule ID: **%s**", i+1, timeDesc, schedule.ID))
	}

	message := fmt.Sprintf("📅 Your active schedules:\n\n%s\n\n💡 **Important**: To delete a schedule, use the scheduler tool with action='delete' and the exact Schedule ID shown above.\n\nAll reminders include %d-minute preparation messages to help you mentally prepare.",
		strings.Join(scheduleLines, "\n"), st.prepTimeMinutes)

	return &models.ToolResult{
		Success: true,
		Message: message,
		Data: map[string]interface{}{
			"schedules":         schedules,
			"schedule_ids":      extractScheduleIDs(schedules),
			"prep_time_minutes": st.prepTimeMinutes,
		},
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
		// Check if the provided ID looks like a time (common mistake)
		if strings.Contains(scheduleID, ":") && len(scheduleID) <= 5 {
			return &models.ToolResult{
				Success: false,
				Message: fmt.Sprintf("⚠️ It looks like you used a time ('%s') instead of a Schedule ID. Schedule IDs are unique identifiers like 'sched_abc123'. Please use the scheduler 'list' action first to see your active schedules and their proper Schedule IDs.", scheduleID),
				Error:   "Invalid schedule ID format - time used instead of schedule ID",
			}, nil
		}

		return &models.ToolResult{
			Success: false,
			Message: fmt.Sprintf("Schedule with ID '%s' not found. Use the scheduler 'list' action to see your active schedules and their Schedule IDs.", scheduleID),
			Error:   "Schedule not found",
		}, nil
	}

	// Cancel the associated timer
	if scheduleToDelete.TimerID != "" {
		if err := st.timer.Cancel(scheduleToDelete.TimerID); err != nil {
			slog.Warn("SchedulerTool.executeDeleteSchedule: failed to cancel timer", "participantID", participantID, "scheduleID", scheduleID, "timerID", scheduleToDelete.TimerID, "error", err)
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

	message := fmt.Sprintf("✅ Successfully deleted the daily habit reminder schedule (ID: %s). Your preparation messages and daily reminders have been stopped.", scheduleID)

	slog.Info("SchedulerTool.executeDeleteSchedule: schedule deleted successfully", "participantID", participantID, "scheduleID", scheduleID, "timerID", scheduleToDelete.TimerID)

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

// extractScheduleIDs extracts a list of schedule IDs from ScheduleInfo slice.
func extractScheduleIDs(schedules []models.ScheduleInfo) []string {
	ids := make([]string, len(schedules))
	for i, schedule := range schedules {
		ids[i] = schedule.ID
	}
	return ids
}
