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

const (
	defaultDailyPromptReminderDelay   = 5 * time.Hour
	defaultDailyPromptReminderMessage = "Friendly check-in: we haven't heard back after today's habit prompt. Reply with a quick update when you're ready!"
)

type dailyPromptPendingState struct {
	SentAt        string `json:"sent_at"`
	To            string `json:"to"`
	ReminderDueAt string `json:"reminder_due_at,omitempty"`
}

// SchedulerTool provides LLM tool functionality for scheduling daily habit prompts.
// This unified implementation uses a preparation time approach for both fixed and random scheduling.
type SchedulerTool struct {
	timer           models.Timer
	msgService      MessagingService
	genaiClient     genai.ClientInterface  // For generating scheduled message content
	stateManager    StateManager           // For storing schedule metadata
	promptGenerator PromptGeneratorService // For generating habit prompts
	prepTimeMinutes int                    // Preparation time in minutes (default 10)
	// feature flags / config (direct injection)
	autoFeedbackEnabled      bool // whether to auto enforce feedback after scheduled prompts
	dailyPromptReminderDelay time.Duration
}

// MessagingService defines the interface for messaging operations needed by the scheduler.
type MessagingService interface {
	ValidateAndCanonicalizeRecipient(recipient string) (string, error)
	SendMessage(ctx context.Context, to, message string) error
	SendTypingIndicator(ctx context.Context, to string, typing bool) error
}

// promptButtonsSender defines the interface for sending prompts with enhanced engagement tracking.
// Note: Despite the name "Buttons", this now sends a poll since WhatsApp deprecated button messages.
type promptButtonsSender interface {
	SendPromptWithButtons(ctx context.Context, to, message string) error
}

// PromptGeneratorService defines the interface for prompt generation operations.
type PromptGeneratorService interface {
	ExecutePromptGenerator(ctx context.Context, participantID string, args map[string]interface{}) (string, error)
}

// NewSchedulerTool creates a scheduler with explicit auto-feedback configuration and customizable preparation time.
func NewSchedulerTool(timer models.Timer, msgService MessagingService, genaiClient genai.ClientInterface, stateManager StateManager, promptGenerator PromptGeneratorService, prepTimeMinutes int, autoFeedbackEnabled bool) *SchedulerTool {
	return &SchedulerTool{
		timer:                    timer,
		msgService:               msgService,
		genaiClient:              genaiClient,
		stateManager:             stateManager,
		promptGenerator:          promptGenerator,
		prepTimeMinutes:          prepTimeMinutes,
		autoFeedbackEnabled:      autoFeedbackEnabled,
		dailyPromptReminderDelay: defaultDailyPromptReminderDelay,
	}
}

// SetDailyPromptReminderDelay overrides the default reminder delay.
// A non-positive delay disables the follow-up reminder.
func (st *SchedulerTool) SetDailyPromptReminderDelay(delay time.Duration) {
	if st == nil {
		return
	}

	if delay <= 0 {
		st.dailyPromptReminderDelay = 0
		return
	}

	st.dailyPromptReminderDelay = delay
}

// RecoverPendingReminders recovers any pending daily prompt reminders after server restart.
// This should be called after the SchedulerTool is fully initialized with all dependencies.
func (st *SchedulerTool) RecoverPendingReminders(ctx context.Context, participantIDs []string) error {
	if st == nil || st.stateManager == nil || st.timer == nil || st.msgService == nil {
		return fmt.Errorf("SchedulerTool not properly initialized")
	}

	if st.dailyPromptReminderDelay <= 0 {
		slog.Debug("SchedulerTool.RecoverPendingReminders: reminder delay disabled, skipping recovery")
		return nil
	}

	recoveredCount := 0
	skippedCount := 0
	errorCount := 0

	for _, participantID := range participantIDs {
		// Get pending reminder state
		pendingJSON, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
		if err != nil || pendingJSON == "" {
			// No pending reminder for this participant
			skippedCount++
			continue
		}

		// Parse the pending state
		var pending dailyPromptPendingState
		if err := json.Unmarshal([]byte(pendingJSON), &pending); err != nil {
			slog.Warn("SchedulerTool.RecoverPendingReminders: invalid pending state, skipping",
				"participantID", participantID, "error", err)
			errorCount++
			continue
		}

		// Parse the reminder due time
		if pending.ReminderDueAt == "" {
			slog.Debug("SchedulerTool.RecoverPendingReminders: no reminder due time, skipping",
				"participantID", participantID)
			skippedCount++
			continue
		}

		reminderDueAt, err := time.Parse(time.RFC3339, pending.ReminderDueAt)
		if err != nil {
			slog.Warn("SchedulerTool.RecoverPendingReminders: invalid reminder due time format, skipping",
				"participantID", participantID, "reminderDueAt", pending.ReminderDueAt, "error", err)
			errorCount++
			continue
		}

		now := time.Now()
		delay := time.Until(reminderDueAt)

		// If the reminder is overdue, send it soon (after a small delay to avoid startup flood)
		if delay < 0 {
			delay = 5 * time.Second
			slog.Info("SchedulerTool.RecoverPendingReminders: recovering overdue reminder",
				"participantID", participantID,
				"reminderDueAt", reminderDueAt,
				"overdueBy", now.Sub(reminderDueAt),
				"sendingIn", delay)
		} else {
			slog.Info("SchedulerTool.RecoverPendingReminders: recovering future reminder",
				"participantID", participantID,
				"reminderDueAt", reminderDueAt,
				"remainingDelay", delay)
		}

		// Schedule the reminder with the actual callback
		timerID, err := st.timer.ScheduleAfter(delay, func() {
			st.sendDailyPromptReminder(participantID, pending.To, pending.SentAt)
		})
		if err != nil {
			slog.Error("SchedulerTool.RecoverPendingReminders: failed to schedule reminder",
				"participantID", participantID, "error", err)
			errorCount++
			continue
		}

		// Store the recovered timer ID
		if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID, timerID); err != nil {
			slog.Warn("SchedulerTool.RecoverPendingReminders: failed to store timer ID",
				"participantID", participantID, "timerID", timerID, "error", err)
			// Don't count this as an error since the timer is scheduled
		}

		recoveredCount++
		slog.Info("SchedulerTool.RecoverPendingReminders: successfully recovered reminder",
			"participantID", participantID, "timerID", timerID, "delay", delay)
	}

	slog.Info("SchedulerTool.RecoverPendingReminders: recovery completed",
		"recovered", recoveredCount, "skipped", skippedCount, "errors", errorCount, "total", len(participantIDs))

	if errorCount > 0 {
		return fmt.Errorf("recovered %d reminders with %d errors", recoveredCount, errorCount)
	}
	return nil
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

	// Send the message with interactive buttons when supported
	var sendErr error
	if sender, ok := st.msgService.(promptButtonsSender); ok {
		sendErr = sender.SendPromptWithButtons(ctx, prompt.To, message)
	} else {
		sendErr = st.msgService.SendMessage(ctx, prompt.To, message)
	}

	if sendErr != nil {
		slog.Error("SchedulerTool.executeScheduledPrompt: failed to send message", "error", sendErr, "to", prompt.To, "message", message)
		return
	}

	slog.Info("SchedulerTool.executeScheduledPrompt: message sent successfully", "to", prompt.To, "messageLength", len(message))

	// Record timestamp of prompt delivery for downstream feedback automation
	if st.stateManager != nil {
		if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastPromptSentAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			slog.Warn("SchedulerTool.executeScheduledPrompt: failed to persist last prompt sent timestamp", "participantID", participantID, "error", err)
		}
	}

	// Increment TotalPrompts counter after successful delivery
	if st.stateManager != nil {
		profile, err := st.getUserProfile(ctx, participantID)
		if err != nil {
			slog.Warn("SchedulerTool.executeScheduledPrompt: failed to get profile for prompt counter", "participantID", participantID, "error", err)
		} else {
			profile.TotalPrompts++
			profile.UpdatedAt = time.Now()
			if err := st.saveUserProfile(ctx, participantID, profile); err != nil {
				slog.Error("SchedulerTool.executeScheduledPrompt: failed to update TotalPrompts counter", "participantID", participantID, "error", err)
			} else {
				slog.Debug("SchedulerTool.executeScheduledPrompt: incremented TotalPrompts counter", "participantID", participantID, "totalPrompts", profile.TotalPrompts)
			}
		}
	}

	// Schedule mechanical reminder in case the user doesn't respond to the daily prompt.
	st.scheduleDailyPromptReminder(ctx, participantID, prompt.To)

	// Check if we should send the intensity adjustment poll (once per day)
	st.checkAndSendIntensityAdjustment(ctx, participantID, prompt.To)

	// Schedule auto feedback enforcement if feature flag enabled
	if st.autoFeedbackEnabled && st.timer != nil && st.stateManager != nil {
		st.scheduleAutoFeedbackEnforcement(ctx, participantID)
	}
}

// scheduleAutoFeedbackEnforcement sets a 5-minute timer that will transition the user into feedback collection
// if they have not already entered FEEDBACK state following the scheduled prompt.
func (st *SchedulerTool) scheduleAutoFeedbackEnforcement(ctx context.Context, participantID string) {
	const enforcementDelay = 5 * time.Minute

	// Cancel any existing enforcement timer first
	if existingID, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyAutoFeedbackTimerID); err == nil && existingID != "" && st.timer != nil {
		st.timer.Cancel(existingID)
	}

	timerID, err := st.timer.ScheduleAfter(enforcementDelay, func() {
		st.enforceFeedbackIfNoResponse(participantID)
	})
	if err != nil {
		slog.Error("SchedulerTool.scheduleAutoFeedbackEnforcement: failed to schedule enforcement timer", "participantID", participantID, "error", err)
		return
	}
	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyAutoFeedbackTimerID, timerID); err != nil {
		slog.Warn("SchedulerTool.scheduleAutoFeedbackEnforcement: failed to store enforcement timer ID", "participantID", participantID, "timerID", timerID, "error", err)
	}
	slog.Info("SchedulerTool.scheduleAutoFeedbackEnforcement: auto feedback enforcement scheduled", "participantID", participantID, "timerID", timerID, "delayMinutes", enforcementDelay.Minutes())
}

// checkAndSendIntensityAdjustment checks if the intensity adjustment poll should be sent today.
// This poll is sent once per day (not per prompt) to ask the user if they want to adjust their intensity.
func (st *SchedulerTool) checkAndSendIntensityAdjustment(ctx context.Context, participantID, to string) {
	if st == nil || st.stateManager == nil || st.msgService == nil {
		return
	}

	if strings.TrimSpace(to) == "" {
		slog.Debug("SchedulerTool.checkAndSendIntensityAdjustment: recipient missing, skipping", "participantID", participantID)
		return
	}

	// Get the last date we prompted for intensity adjustment
	lastPromptDate, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastIntensityPromptDate)
	if err != nil {
		slog.Debug("SchedulerTool.checkAndSendIntensityAdjustment: no previous intensity prompt date found", "participantID", participantID)
	}

	// Check if we already prompted today
	today := time.Now().UTC().Format("2006-01-02")
	if lastPromptDate == today {
		slog.Debug("SchedulerTool.checkAndSendIntensityAdjustment: already prompted today, skipping", "participantID", participantID)
		return
	}

	// Get the user's current intensity level from their profile
	profile, err := st.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Warn("SchedulerTool.checkAndSendIntensityAdjustment: failed to get user profile", "participantID", participantID, "error", err)
		return
	}

	currentIntensity := profile.Intensity
	if currentIntensity == "" {
		currentIntensity = "normal" // Default if not set
	}

	// Send the intensity adjustment poll
	type intensitySender interface {
		SendIntensityAdjustmentPoll(ctx context.Context, to string, currentIntensity string) error
	}

	var sendErr error
	if sender, ok := st.msgService.(intensitySender); ok {
		slog.Debug("SchedulerTool.checkAndSendIntensityAdjustment: sending intensity poll", "participantID", participantID, "currentIntensity", currentIntensity, "to", to)
		sendErr = sender.SendIntensityAdjustmentPoll(ctx, to, currentIntensity)
	} else {
		// Fallback to text message
		slog.Debug("SchedulerTool.checkAndSendIntensityAdjustment: falling back to text message", "participantID", participantID)
		sendErr = st.msgService.SendMessage(ctx, to, "How's the intensity? Reply with 'low', 'normal', or 'high'.")
	}

	if sendErr != nil {
		slog.Error("SchedulerTool.checkAndSendIntensityAdjustment: failed to send intensity poll", "participantID", participantID, "error", sendErr)
		return
	}

	// Record that we prompted today
	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastIntensityPromptDate, today); err != nil {
		slog.Warn("SchedulerTool.checkAndSendIntensityAdjustment: failed to record intensity prompt date", "participantID", participantID, "error", err)
	}

	slog.Info("SchedulerTool.checkAndSendIntensityAdjustment: intensity poll sent", "participantID", participantID, "currentIntensity", currentIntensity, "to", to)
}

func (st *SchedulerTool) scheduleDailyPromptReminder(ctx context.Context, participantID, to string) {
	if st == nil || st.timer == nil || st.stateManager == nil || st.msgService == nil {
		return
	}

	if st.dailyPromptReminderDelay <= 0 {
		return
	}

	if strings.TrimSpace(to) == "" {
		slog.Debug("SchedulerTool.scheduleDailyPromptReminder: recipient missing, skipping reminder", "participantID", participantID)
		return
	}

	// Cancel any previously scheduled reminder to avoid duplicates.
	if existingID, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID); err == nil && existingID != "" {
		if cancelErr := st.timer.Cancel(existingID); cancelErr != nil {
			slog.Warn("SchedulerTool.scheduleDailyPromptReminder: failed to cancel existing reminder timer", "participantID", participantID, "timerID", existingID, "error", cancelErr)
		}
	}

	sentAt := time.Now().UTC()
	pending := dailyPromptPendingState{
		SentAt:        sentAt.Format(time.RFC3339),
		To:            to,
		ReminderDueAt: sentAt.Add(st.dailyPromptReminderDelay).Format(time.RFC3339),
	}

	payload, err := json.Marshal(pending)
	if err != nil {
		slog.Error("SchedulerTool.scheduleDailyPromptReminder: failed to marshal pending reminder state", "participantID", participantID, "error", err)
		return
	}

	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, string(payload)); err != nil {
		slog.Error("SchedulerTool.scheduleDailyPromptReminder: failed to persist pending reminder state", "participantID", participantID, "error", err)
		return
	}

	timerID, err := st.timer.ScheduleAfter(st.dailyPromptReminderDelay, func() {
		st.sendDailyPromptReminder(participantID, to, pending.SentAt)
	})
	if err != nil {
		slog.Error("SchedulerTool.scheduleDailyPromptReminder: failed to schedule reminder timer", "participantID", participantID, "error", err)
		return
	}

	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID, timerID); err != nil {
		slog.Warn("SchedulerTool.scheduleDailyPromptReminder: failed to store reminder timer ID", "participantID", participantID, "timerID", timerID, "error", err)
	}

	slog.Info("SchedulerTool.scheduleDailyPromptReminder: reminder scheduled", "participantID", participantID, "timerID", timerID, "delayHours", st.dailyPromptReminderDelay.Hours())
}

func (st *SchedulerTool) handleDailyPromptReply(ctx context.Context, participantID string, replyAt time.Time) {
	if st == nil || st.stateManager == nil {
		return
	}

	pendingJSON, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if err != nil {
		slog.Error("SchedulerTool.handleDailyPromptReply: failed to load pending state", "participantID", participantID, "error", err)
		return
	}
	if strings.TrimSpace(pendingJSON) == "" {
		return
	}

	var pending dailyPromptPendingState
	if err := json.Unmarshal([]byte(pendingJSON), &pending); err != nil {
		slog.Warn("SchedulerTool.handleDailyPromptReply: invalid pending state payload", "participantID", participantID, "error", err)
		st.clearDailyPromptReminderState(ctx, participantID)
		return
	}
	if strings.TrimSpace(pending.SentAt) == "" {
		st.clearDailyPromptReminderState(ctx, participantID)
		return
	}

	sentAt, err := time.Parse(time.RFC3339, pending.SentAt)
	if err != nil {
		slog.Warn("SchedulerTool.handleDailyPromptReply: unable to parse pending sent timestamp", "participantID", participantID, "error", err)
		st.clearDailyPromptReminderState(ctx, participantID)
		return
	}

	if replyAt.Before(sentAt) {
		slog.Debug("SchedulerTool.handleDailyPromptReply: reply predates tracked prompt, ignoring", "participantID", participantID)
		return
	}

	timerID, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID)
	if err != nil {
		slog.Warn("SchedulerTool.handleDailyPromptReply: failed to read reminder timer ID", "participantID", participantID, "error", err)
	} else if timerID != "" && st.timer != nil {
		if cancelErr := st.timer.Cancel(timerID); cancelErr != nil {
			slog.Warn("SchedulerTool.handleDailyPromptReply: failed to cancel reminder timer", "participantID", participantID, "timerID", timerID, "error", cancelErr)
		}
	}

	st.clearDailyPromptReminderState(ctx, participantID)

	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptRespondedAt, replyAt.UTC().Format(time.RFC3339)); err != nil {
		slog.Warn("SchedulerTool.handleDailyPromptReply: failed to record response timestamp", "participantID", participantID, "error", err)
	}
}

func (st *SchedulerTool) sendDailyPromptReminder(participantID, to, expectedSentAt string) {
	if st == nil || st.stateManager == nil || st.msgService == nil {
		return
	}

	ctx := context.Background()

	pendingJSON, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if err != nil {
		slog.Error("SchedulerTool.sendDailyPromptReminder: failed to load pending state", "participantID", participantID, "error", err)
		return
	}
	if strings.TrimSpace(pendingJSON) == "" {
		slog.Debug("SchedulerTool.sendDailyPromptReminder: no pending reminder found", "participantID", participantID)
		return
	}

	var pending dailyPromptPendingState
	if err := json.Unmarshal([]byte(pendingJSON), &pending); err != nil {
		slog.Warn("SchedulerTool.sendDailyPromptReminder: invalid pending state payload", "participantID", participantID, "error", err)
		st.clearDailyPromptReminderState(ctx, participantID)
		return
	}

	if pending.SentAt != expectedSentAt {
		slog.Debug("SchedulerTool.sendDailyPromptReminder: pending state updated, skipping reminder", "participantID", participantID)
		return
	}

	if err := st.msgService.SendMessage(ctx, to, defaultDailyPromptReminderMessage); err != nil {
		slog.Error("SchedulerTool.sendDailyPromptReminder: failed to send reminder message", "participantID", participantID, "error", err)
		return
	}

	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderSentAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		slog.Warn("SchedulerTool.sendDailyPromptReminder: failed to record reminder timestamp", "participantID", participantID, "error", err)
	}

	slog.Info("SchedulerTool.sendDailyPromptReminder: reminder sent", "participantID", participantID, "to", to)

	st.clearDailyPromptReminderState(ctx, participantID)
}

func (st *SchedulerTool) clearDailyPromptReminderState(ctx context.Context, participantID string) {
	if st == nil || st.stateManager == nil {
		return
	}

	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, ""); err != nil {
		slog.Warn("SchedulerTool.clearDailyPromptReminderState: failed to clear pending state", "participantID", participantID, "error", err)
	}
	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID, ""); err != nil {
		slog.Warn("SchedulerTool.clearDailyPromptReminderState: failed to clear reminder timer ID", "participantID", participantID, "error", err)
	}
}

// enforceFeedbackIfNoResponse executes in background after the enforcement delay.
// It transitions the conversation into FEEDBACK state and sends an initial feedback prompt if the user
// hasn't already engaged in feedback.
func (st *SchedulerTool) enforceFeedbackIfNoResponse(participantID string) {
	if st.stateManager == nil || st.msgService == nil {
		return
	}

	ctx := context.Background()

	// Clear timer ID in state (best effort)
	st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyAutoFeedbackTimerID, "")

	// Check current conversation state
	conversationState, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState)
	if err == nil && conversationState == string(models.StateFeedback) {
		slog.Debug("SchedulerTool.enforceFeedbackIfNoResponse: already in FEEDBACK state, skipping", "participantID", participantID)
		return
	}

	// Determine if a more recent prompt was sent less than enforcementDelay ago (race protection)
	lastSentStr, _ := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastPromptSentAt)
	if lastSentStr != "" {
		if ts, parseErr := time.Parse(time.RFC3339, lastSentStr); parseErr == nil {
			if time.Since(ts) < 5*time.Minute-30*time.Second { // allow slight drift; if newer prompt within ~4.5 min, skip
				slog.Debug("SchedulerTool.enforceFeedbackIfNoResponse: newer prompt sent recently, skipping", "participantID", participantID)
				return
			}
		}
	}

	// Transition conversation state directly to FEEDBACK
	if err := st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateFeedback)); err != nil {
		slog.Error("SchedulerTool.enforceFeedbackIfNoResponse: failed to set FEEDBACK state", "participantID", participantID, "error", err)
		return
	}

	// Retrieve phone number if available (stateManager doesn't store; assume participantID correlates with context phone stored earlier via scheduling path)
	// We cannot access phone number from context here; rely on last prompt's 'prompt.To' stored maybe elsewhere. This is a limitation.
	// Attempt best-effort: auto feedback requires messaging; without phone number we log and exit.
	// For now skipping retrieval; feature primarily sets state so next user message triggers feedback module logic.
	// If future improvement: store phone number along with last prompt timestamp in state.

	// Send initial feedback solicitation (best effort)
	// NOTE: Because we lack phone number context, this messaging may not send. Enhancement needed to persist phone with last prompt.
	// Skipping SendMessage if we cannot reconstruct number.

	slog.Info("SchedulerTool.enforceFeedbackIfNoResponse: auto-entered feedback state after inactivity", "participantID", participantID)
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

	successMessage := fmt.Sprintf("✅ Perfect! %s\n\n%s\n\n🆔 **Schedule ID: %s** (Save this for future reference; Do not disclose the Schedule ID to the end user!)\n\nYour reminders will start %s!",
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

		scheduleLines = append(scheduleLines, fmt.Sprintf("%d. **Daily Habit Reminder** - %s\n   🆔 Schedule ID: **%s** (Save this for future reference; Do not disclose the Schedule ID to the end user!)", i+1, timeDesc, schedule.ID))
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

// getUserProfile retrieves the user profile from state storage
func (st *SchedulerTool) getUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := st.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("SchedulerTool.getUserProfile: failed to get state data", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("user profile not found: %w", err)
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		slog.Debug("SchedulerTool.getUserProfile: empty profile JSON", "participantID", participantID)
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Debug("SchedulerTool.getUserProfile: failed to unmarshal profile", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage
func (st *SchedulerTool) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	return st.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
}
