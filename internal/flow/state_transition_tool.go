// Package flow provides state transition tool functionality for managing conversation state transitions.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// StateTransitionTool provides functionality for transitioning between conversation states.
type StateTransitionTool struct {
	stateManager StateManager
	timer        models.Timer
	jobRepo      store.JobRepo
}

// NewStateTransitionTool creates a new state transition tool instance.
func NewStateTransitionTool(stateManager StateManager, timer models.Timer) *StateTransitionTool {
	slog.Debug("StateTransitionTool.NewStateTransitionTool: creating state transition tool",
		"hasStateManager", stateManager != nil, "hasTimer", timer != nil)
	return &StateTransitionTool{
		stateManager: stateManager,
		timer:        timer,
	}
}

// SetJobRepo sets the durable job repository for restart-safe delayed transitions.
func (stt *StateTransitionTool) SetJobRepo(repo store.JobRepo) {
	stt.jobRepo = repo
}

// GetToolDefinition returns the OpenAI tool definition for state transitions.
func (stt *StateTransitionTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "transition_state",
			Description: openai.String("Transition the conversation to a specific state (INTAKE or FEEDBACK). Use this to route conversations to specialized handlers or to schedule delayed transitions."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"target_state": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"INTAKE", "FEEDBACK"},
						"description": "The target state to transition to",
					},
					"delay_minutes": map[string]interface{}{
						"type":        "number",
						"description": "Optional delay in minutes before the transition occurs (for delayed transitions)",
						"minimum":     0,
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Optional reason for the transition (for logging and debugging)",
					},
				},
				"required": []string{"target_state"},
			},
		},
	}
}

// ExecuteStateTransition executes a state transition, either immediately or after a delay.
func (stt *StateTransitionTool) ExecuteStateTransition(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("StateTransitionTool.ExecuteStateTransition: executing state transition",
		"participantID", participantID, "args", args)

	// Validate dependencies
	if stt.stateManager == nil {
		err := fmt.Errorf("state manager is required for state transitions")
		slog.Error("StateTransitionTool.ExecuteStateTransition: missing state manager", "error", err)
		return "", err
	}

	// Extract arguments
	targetStateStr, ok := args["target_state"].(string)
	if !ok {
		err := fmt.Errorf("target_state is required and must be a string")
		slog.Error("StateTransitionTool.ExecuteStateTransition: invalid target_state", "error", err)
		return "", err
	}

	// Convert string to StateType
	var targetState models.StateType
	switch targetStateStr {
	case "INTAKE":
		targetState = models.StateIntake
	case "FEEDBACK":
		targetState = models.StateFeedback
	default:
		err := fmt.Errorf("invalid target_state: %s", targetStateStr)
		slog.Error("StateTransitionTool.ExecuteStateTransition: invalid target_state value",
			"error", err, "targetState", targetStateStr)
		return "", err
	}

	delayMinutes, _ := args["delay_minutes"].(float64)
	reason, _ := args["reason"].(string)

	// Log the transition request
	slog.Info("StateTransitionTool.ExecuteStateTransition: processing transition request",
		"participantID", participantID,
		"targetState", targetState,
		"delayMinutes", delayMinutes,
		"reason", reason)

	// Handle immediate vs delayed transitions
	if delayMinutes > 0 {
		return stt.scheduleDelayedTransition(ctx, participantID, targetState, delayMinutes, reason)
	} else {
		return stt.executeImmediateTransition(ctx, participantID, targetState, reason)
	}
}

// executeImmediateTransition performs an immediate state transition.
func (stt *StateTransitionTool) executeImmediateTransition(ctx context.Context, participantID string, targetState models.StateType, reason string) (string, error) {
	slog.Debug("StateTransitionTool.executeImmediateTransition: performing immediate transition",
		"participantID", participantID, "targetState", targetState, "reason", reason)

	// Get current state
	currentState, err := stt.getCurrentConversationState(ctx, participantID)
	if err != nil {
		slog.Error("StateTransitionTool.executeImmediateTransition: failed to get current state",
			"error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	// Update the conversation state
	err = stt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyConversationState, string(targetState))
	if err != nil {
		slog.Error("StateTransitionTool.executeImmediateTransition: failed to set conversation state",
			"error", err, "participantID", participantID, "targetState", targetState)
		return "", fmt.Errorf("failed to set conversation state: %w", err)
	}

	// If we have transitioned into FEEDBACK state, cancel any pending auto-feedback enforcement timer
	if targetState == models.StateFeedback {
		if autoTimerID, err := stt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyAutoFeedbackTimerID); err == nil && autoTimerID != "" {
			if stt.jobRepo != nil {
				stt.jobRepo.CancelJob(autoTimerID)
			} else if stt.timer != nil {
				stt.timer.Cancel(autoTimerID)
			}
			stt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyAutoFeedbackTimerID, "")
			slog.Debug("StateTransitionTool.executeImmediateTransition: cancelled auto feedback enforcement timer", "participantID", participantID, "timerID", autoTimerID)
		}
	}

	// Log successful transition
	slog.Info("StateTransitionTool.executeImmediateTransition: transition completed",
		"participantID", participantID,
		"fromState", currentState,
		"toState", targetState,
		"reason", reason)

	// Return success message for the LLM
	return fmt.Sprintf("Conversation state transitioned to %s", targetState), nil
}

// scheduleDelayedTransition schedules a state transition to occur after a delay.
func (stt *StateTransitionTool) scheduleDelayedTransition(ctx context.Context, participantID string, targetState models.StateType, delayMinutes float64, reason string) (string, error) {
	slog.Debug("StateTransitionTool.scheduleDelayedTransition: scheduling delayed transition",
		"participantID", participantID, "targetState", targetState, "delayMinutes", delayMinutes, "reason", reason)

	duration := time.Duration(delayMinutes) * time.Minute

	// Cancel any existing state transition timer/job
	if existingTimerID, err := stt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyStateTransitionTimerID); err == nil && existingTimerID != "" {
		if stt.jobRepo != nil {
			stt.jobRepo.CancelJob(existingTimerID)
		} else if stt.timer != nil {
			stt.timer.Cancel(existingTimerID)
		}
		slog.Debug("StateTransitionTool.scheduleDelayedTransition: cancelled existing timer/job",
			"participantID", participantID, "existingTimerID", existingTimerID)
	}

	// Prefer durable job if jobRepo is available
	if stt.jobRepo != nil {
		payload, err := json.Marshal(StateTransitionPayload{
			ParticipantID: participantID,
			TargetState:   string(targetState),
			Reason:        reason,
		})
		if err != nil {
			return "", fmt.Errorf("failed to marshal state transition payload: %w", err)
		}

		dedupeKey := fmt.Sprintf("state_transition:%s", participantID)
		jobID, err := stt.jobRepo.EnqueueJob(JobKindStateTransition, time.Now().Add(duration), string(payload), dedupeKey)
		if err != nil {
			return "", fmt.Errorf("failed to enqueue state transition job: %w", err)
		}

		// Store the job ID for cancellation
		if err := stt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation,
			models.DataKeyStateTransitionTimerID, jobID); err != nil {
			stt.jobRepo.CancelJob(jobID)
			return "", fmt.Errorf("failed to store job ID: %w", err)
		}

		slog.Info("StateTransitionTool.scheduleDelayedTransition: durable job scheduled",
			"participantID", participantID, "targetState", targetState,
			"delayMinutes", delayMinutes, "jobID", jobID, "reason", reason)
		return fmt.Sprintf("Scheduled transition to %s in %.1f minutes", targetState, delayMinutes), nil
	}

	// Fallback to in-memory timer
	if stt.timer == nil {
		err := fmt.Errorf("timer is required for delayed state transitions")
		slog.Error("StateTransitionTool.scheduleDelayedTransition: missing timer", "error", err)
		return "", err
	}

	// Schedule the delayed transition
	timerID, err := stt.timer.ScheduleAfter(duration, func() {
		slog.Info("StateTransitionTool.scheduleDelayedTransition: executing delayed transition",
			"participantID", participantID, "targetState", targetState, "reason", reason)

		// Create new context for the delayed execution
		delayedCtx := context.Background()

		// Execute the transition
		_, err := stt.executeImmediateTransition(delayedCtx, participantID, targetState, reason)
		if err != nil {
			slog.Error("StateTransitionTool.scheduleDelayedTransition: delayed transition failed",
				"error", err, "participantID", participantID, "targetState", targetState)
		}

		// Clear the timer ID from state
		stt.stateManager.SetStateData(delayedCtx, participantID, models.FlowTypeConversation,
			models.DataKeyStateTransitionTimerID, "")
	})
	if err != nil {
		slog.Error("StateTransitionTool.scheduleDelayedTransition: failed to schedule timer",
			"error", err, "participantID", participantID, "targetState", targetState)
		return "", fmt.Errorf("failed to schedule delayed transition: %w", err)
	}

	// Store the timer ID
	err = stt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyStateTransitionTimerID, timerID)
	if err != nil {
		slog.Error("StateTransitionTool.scheduleDelayedTransition: failed to store timer ID",
			"error", err, "participantID", participantID, "timerID", timerID)
		stt.timer.Cancel(timerID)
		return "", fmt.Errorf("failed to store timer ID: %w", err)
	}

	slog.Info("StateTransitionTool.scheduleDelayedTransition: delayed transition scheduled",
		"participantID", participantID,
		"targetState", targetState,
		"delayMinutes", delayMinutes,
		"timerID", timerID,
		"reason", reason)

	// Return success message for the LLM
	return fmt.Sprintf("Scheduled transition to %s in %.1f minutes", targetState, delayMinutes), nil
}

// getCurrentConversationState retrieves the current conversation state for a participant.
func (stt *StateTransitionTool) getCurrentConversationState(ctx context.Context, participantID string) (models.StateType, error) {
	stateStr, err := stt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyConversationState)
	if err != nil {
		return "", err
	}

	// Default to INTAKE if no state is set
	if stateStr == "" {
		return models.StateIntake, nil
	}

	return models.StateType(stateStr), nil
}

// CancelPendingTransition cancels any pending delayed state transition for a participant.
func (stt *StateTransitionTool) CancelPendingTransition(ctx context.Context, participantID string) error {
	slog.Debug("StateTransitionTool.CancelPendingTransition: cancelling pending transition",
		"participantID", participantID)

	timerID, err := stt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyStateTransitionTimerID)
	if err != nil || timerID == "" {
		return nil // No pending timer/job
	}

	// Cancel the timer or job
	if stt.jobRepo != nil {
		stt.jobRepo.CancelJob(timerID)
	} else if stt.timer != nil {
		stt.timer.Cancel(timerID)
	}

	// Clear the timer ID
	err = stt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyStateTransitionTimerID, "")
	if err != nil {
		slog.Error("StateTransitionTool.CancelPendingTransition: failed to clear timer ID",
			"error", err, "participantID", participantID)
		return err
	}

	slog.Info("StateTransitionTool.CancelPendingTransition: cancelled pending transition",
		"participantID", participantID, "timerID", timerID)
	return nil
}
