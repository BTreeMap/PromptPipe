// Package flow provides durable job kind constants and payload types
// used to replace in-memory timers with restart-safe database jobs.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// Job kind constants for durable jobs that replace in-memory timers.
const (
	JobKindStateTransition         = "state_transition"
	JobKindFeedbackTimeout         = "feedback_timeout"
	JobKindFeedbackFollowup        = "feedback_followup"
	JobKindDailyPromptReminder     = "daily_prompt_reminder"
	JobKindAutoFeedbackEnforcement = "auto_feedback_enforcement"
)

// StateTransitionPayload is the JSON payload for state_transition jobs.
type StateTransitionPayload struct {
	ParticipantID string `json:"participant_id"`
	TargetState   string `json:"target_state"`
	Reason        string `json:"reason,omitempty"`
}

// FeedbackTimeoutPayload is the JSON payload for feedback_timeout jobs.
type FeedbackTimeoutPayload struct {
	ParticipantID string `json:"participant_id"`
	PhoneNumber   string `json:"phone_number"`
}

// FeedbackFollowupPayload is the JSON payload for feedback_followup jobs.
type FeedbackFollowupPayload struct {
	ParticipantID string `json:"participant_id"`
	PhoneNumber   string `json:"phone_number"`
}

// DailyPromptReminderPayload is the JSON payload for daily_prompt_reminder jobs.
type DailyPromptReminderPayload struct {
	ParticipantID  string `json:"participant_id"`
	To             string `json:"to"`
	ExpectedSentAt string `json:"expected_sent_at"`
}

// AutoFeedbackEnforcementPayload is the JSON payload for auto_feedback_enforcement jobs.
type AutoFeedbackEnforcementPayload struct {
	ParticipantID string `json:"participant_id"`
}

// RegisterJobHandlers registers all flow-related job handlers with the given JobRunner.
func RegisterJobHandlers(runner *store.JobRunner, stateManager StateManager, msgService MessagingService, schedulerTool *SchedulerTool, feedbackModule *FeedbackModule, stateTransitionTool *StateTransitionTool) {
	runner.RegisterHandler(JobKindStateTransition, makeStateTransitionHandler(stateManager, stateTransitionTool))
	runner.RegisterHandler(JobKindFeedbackTimeout, makeFeedbackTimeoutHandler(stateManager, feedbackModule))
	runner.RegisterHandler(JobKindFeedbackFollowup, makeFeedbackFollowupHandler(stateManager, msgService))
	runner.RegisterHandler(JobKindDailyPromptReminder, makeDailyPromptReminderHandler(schedulerTool))
	runner.RegisterHandler(JobKindAutoFeedbackEnforcement, makeAutoFeedbackEnforcementHandler(schedulerTool))
}

func makeStateTransitionHandler(stateManager StateManager, stt *StateTransitionTool) store.JobHandler {
	return func(ctx context.Context, payload string) error {
		var p StateTransitionPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("invalid state_transition payload: %w", err)
		}
		slog.Info("JobHandler.state_transition: executing", "participantID", p.ParticipantID, "targetState", p.TargetState)

		// Idempotency: check current state - if already at target, skip
		currentState, err := stateManager.GetStateData(ctx, p.ParticipantID, models.FlowTypeConversation, models.DataKeyConversationState)
		if err != nil {
			return fmt.Errorf("failed to read current state: %w", err)
		}
		if currentState == p.TargetState {
			slog.Info("JobHandler.state_transition: already at target state, skipping", "participantID", p.ParticipantID, "targetState", p.TargetState)
			return nil
		}

		// Execute the transition
		_, err = stt.executeImmediateTransition(ctx, p.ParticipantID, models.StateType(p.TargetState), p.Reason)
		if err != nil {
			return fmt.Errorf("state transition failed: %w", err)
		}
		// Clear the stored job ID reference
		_ = stateManager.SetStateData(ctx, p.ParticipantID, models.FlowTypeConversation, models.DataKeyStateTransitionTimerID, "")
		return nil
	}
}

func makeFeedbackTimeoutHandler(stateManager StateManager, fm *FeedbackModule) store.JobHandler {
	return func(ctx context.Context, payload string) error {
		var p FeedbackTimeoutPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("invalid feedback_timeout payload: %w", err)
		}
		slog.Info("JobHandler.feedback_timeout: executing", "participantID", p.ParticipantID)

		// Idempotency: check if feedback was already received
		feedbackState, err := stateManager.GetStateData(ctx, p.ParticipantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
		if err == nil && feedbackState != "waiting_initial" {
			slog.Info("JobHandler.feedback_timeout: feedback already received, skipping", "participantID", p.ParticipantID, "currentState", feedbackState)
			return nil
		}

		// Inject phone number into context for the handler
		ctxWithPhone := context.WithValue(ctx, phoneNumberContextKey, p.PhoneNumber)
		fm.handleInitialFeedbackTimeout(ctxWithPhone, p.ParticipantID)
		return nil
	}
}

func makeFeedbackFollowupHandler(stateManager StateManager, msgService MessagingService) store.JobHandler {
	return func(ctx context.Context, payload string) error {
		var p FeedbackFollowupPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("invalid feedback_followup payload: %w", err)
		}
		slog.Info("JobHandler.feedback_followup: executing", "participantID", p.ParticipantID)

		// Idempotency: check if feedback was already completed
		feedbackState, err := stateManager.GetStateData(ctx, p.ParticipantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
		if err == nil && feedbackState == "completed" {
			slog.Info("JobHandler.feedback_followup: feedback already completed, skipping", "participantID", p.ParticipantID)
			return nil
		}

		// Send follow-up message
		followupMessage := "Hey! 👋 Just checking in - I sent you a habit suggestion earlier. Even if you didn't try it, I'd love to know what you think! Your feedback helps me learn what works best for you. 😊"
		if err := msgService.SendMessage(ctx, p.PhoneNumber, followupMessage); err != nil {
			return fmt.Errorf("failed to send follow-up: %w", err)
		}

		// Update state
		_ = stateManager.SetStateData(ctx, p.ParticipantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "followup_sent")
		return nil
	}
}

func makeDailyPromptReminderHandler(schedulerTool *SchedulerTool) store.JobHandler {
	return func(ctx context.Context, payload string) error {
		var p DailyPromptReminderPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("invalid daily_prompt_reminder payload: %w", err)
		}
		slog.Info("JobHandler.daily_prompt_reminder: executing", "participantID", p.ParticipantID, "to", p.To)

		// Delegate to existing scheduler tool logic (which already checks pending state)
		schedulerTool.sendDailyPromptReminder(p.ParticipantID, p.To, p.ExpectedSentAt)
		return nil
	}
}

func makeAutoFeedbackEnforcementHandler(schedulerTool *SchedulerTool) store.JobHandler {
	return func(ctx context.Context, payload string) error {
		var p AutoFeedbackEnforcementPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return fmt.Errorf("invalid auto_feedback_enforcement payload: %w", err)
		}
		slog.Info("JobHandler.auto_feedback_enforcement: executing", "participantID", p.ParticipantID)

		// Delegate to existing scheduler tool logic (which already checks state)
		schedulerTool.enforceFeedbackIfNoResponse(p.ParticipantID)
		return nil
	}
}
