// Package flow provides recovery implementation for micro health intervention flows
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/recovery"
)

// MicroHealthInterventionRecovery implements recovery for micro health intervention flows
type MicroHealthInterventionRecovery struct {
	stateManager StateManager
}

// NewMicroHealthInterventionRecovery creates a new recovery handler for micro health interventions
func NewMicroHealthInterventionRecovery(stateManager StateManager) *MicroHealthInterventionRecovery {
	return &MicroHealthInterventionRecovery{
		stateManager: stateManager,
	}
}

// GetFlowType returns the flow type this recoverable handles
func (r *MicroHealthInterventionRecovery) GetFlowType() models.FlowType {
	return models.FlowTypeMicroHealthIntervention
}

// RecoverState performs recovery for all micro health intervention participants
func (r *MicroHealthInterventionRecovery) RecoverState(ctx context.Context, registry *recovery.RecoveryRegistry) error {
	slog.Info("MicroHealthInterventionRecovery.RecoverState: starting micro health intervention recovery")

	store := registry.GetStore()
	participants, err := store.ListInterventionParticipants()
	if err != nil {
		return fmt.Errorf("failed to list intervention participants: %w", err)
	}

	recoveredCount := 0
	errorCount := 0

	for _, participant := range participants {
		if participant.Status != models.ParticipantStatusActive {
			continue
		}

		if err := r.RecoverParticipant(ctx, participant.ID, participant, registry); err != nil {
			slog.Error("Failed to recover intervention participant",
				"error", err, "participantID", participant.ID, "phone", participant.PhoneNumber)
			errorCount++
			continue
		}

		recoveredCount++
	}

	slog.Info("Micro health intervention recovery completed",
		"recovered", recoveredCount, "errors", errorCount, "total", len(participants))
	return nil
}

// RecoverParticipant recovers state for a single intervention participant
func (r *MicroHealthInterventionRecovery) RecoverParticipant(ctx context.Context, participantID string, participant interface{}, registry *recovery.RecoveryRegistry) error {
	p, ok := participant.(models.InterventionParticipant)
	if !ok {
		return fmt.Errorf("invalid participant type for intervention recovery")
	}

	// Get current state
	currentState, err := r.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	if currentState == "" || currentState == models.StateEndOfDay {
		slog.Debug("Participant has no active state to recover",
			"participantID", participantID, "state", currentState)
		return nil
	}

	// Recover timers for this participant
	if err := r.recoverTimersForParticipant(ctx, participantID, currentState, registry); err != nil {
		slog.Error("Failed to recover timers for participant",
			"error", err, "participantID", participantID)
		// Continue with response handler recovery even if timer recovery fails
	}

	// Register response handler
	handlerInfo := recovery.ResponseHandlerRecoveryInfo{
		PhoneNumber:   p.PhoneNumber,
		ParticipantID: participantID,
		FlowType:      models.FlowTypeMicroHealthIntervention,
		HandlerType:   "intervention",
		TTL:           48 * time.Hour,
	}

	if err := registry.RecoverResponseHandler(handlerInfo); err != nil {
		return fmt.Errorf("failed to register response handler: %w", err)
	}

	slog.Debug("Successfully recovered intervention participant",
		"participantID", participantID, "state", currentState, "phone", p.PhoneNumber)
	return nil
}

// recoverTimersForParticipant recreates timers for a participant based on their current state
func (r *MicroHealthInterventionRecovery) recoverTimersForParticipant(ctx context.Context, participantID string, currentState models.StateType, registry *recovery.RecoveryRegistry) error {
	// Check for stored timer IDs that need cleanup
	timerKeys := []models.DataKey{
		models.DataKeyCommitmentTimerID,
		models.DataKeyFeelingTimerID,
		models.DataKeyCompletionTimerID,
		models.DataKeyDidYouGetAChanceTimerID,
		models.DataKeyContextTimerID,
		models.DataKeyMoodTimerID,
		models.DataKeyBarrierCheckTimerID,
		models.DataKeyBarrierReasonTimerID,
	}

	hasActiveTimer := false
	for _, key := range timerKeys {
		timerID, err := r.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, key)
		if err == nil && timerID != "" {
			hasActiveTimer = true
			// Clear the old timer ID since the timer no longer exists
			r.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, key, "")
		}
	}

	if !hasActiveTimer {
		slog.Debug("No active timers found for participant", "participantID", participantID)
		return nil
	}

	// Recreate appropriate timer based on current state
	// Use shorter recovery timeouts since we don't know how long the timer has been running
	var timerInfo recovery.TimerRecoveryInfo

	switch currentState {
	case models.StateCommitmentPrompt:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyCommitmentTimerID,
			OriginalTTL:   30 * time.Minute, // Shorter recovery timeout
			CreatedAt:     time.Now(),
		}

	case models.StateFeelingPrompt:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyFeelingTimerID,
			OriginalTTL:   5 * time.Minute,
			CreatedAt:     time.Now(),
		}

	case models.StateHabitReminder:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyCompletionTimerID,
			OriginalTTL:   15 * time.Minute,
			CreatedAt:     time.Now(),
		}

	case models.StateDidYouGetAChance:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyDidYouGetAChanceTimerID,
			OriginalTTL:   5 * time.Minute,
			CreatedAt:     time.Now(),
		}

	case models.StateContextQuestion:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyContextTimerID,
			OriginalTTL:   5 * time.Minute,
			CreatedAt:     time.Now(),
		}

	case models.StateMoodQuestion:
		timerInfo = recovery.TimerRecoveryInfo{
			ParticipantID: participantID,
			FlowType:      models.FlowTypeMicroHealthIntervention,
			StateType:     currentState,
			DataKey:       models.DataKeyMoodTimerID,
			OriginalTTL:   5 * time.Minute,
			CreatedAt:     time.Now(),
		}

	default:
		slog.Debug("No timer recovery needed for current state",
			"participantID", participantID, "state", currentState)
		return nil
	}

	// Request timer recovery from the registry
	timerID, err := registry.RecoverTimer(timerInfo)
	if err != nil {
		return fmt.Errorf("failed to recover timer: %w", err)
	}

	// Store the new timer ID
	r.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, timerInfo.DataKey, timerID)

	slog.Info("Recreated timer",
		"participantID", participantID, "state", currentState, "timerID", timerID, "ttl", timerInfo.OriginalTTL)

	return nil
}
