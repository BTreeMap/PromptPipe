// Package flow provides recovery implementation for conversation flows  
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/recovery"
)

// ConversationFlowRecovery implements recovery for conversation flows
type ConversationFlowRecovery struct{}

// NewConversationFlowRecovery creates a new recovery handler for conversation flows
func NewConversationFlowRecovery() *ConversationFlowRecovery {
	return &ConversationFlowRecovery{}
}

// GetFlowType returns the flow type this recoverable handles
func (r *ConversationFlowRecovery) GetFlowType() models.FlowType {
	return models.FlowTypeConversation
}

// RecoverState performs recovery for all conversation participants
func (r *ConversationFlowRecovery) RecoverState(ctx context.Context, registry *recovery.RecoveryRegistry) error {
	slog.Info("Starting conversation flow recovery")

	store := registry.GetStore()
	participants, err := store.ListConversationParticipants()
	if err != nil {
		return fmt.Errorf("failed to list conversation participants: %w", err)
	}

	recoveredCount := 0
	errorCount := 0

	for _, participant := range participants {
		if participant.Status != models.ConversationStatusActive {
			continue
		}

		if err := r.RecoverParticipant(ctx, participant.ID, participant, registry); err != nil {
			slog.Error("Failed to recover conversation participant", 
				"error", err, "participantID", participant.ID, "phone", participant.PhoneNumber)
			errorCount++
			continue
		}

		recoveredCount++
	}

	slog.Info("Conversation flow recovery completed", 
		"recovered", recoveredCount, "errors", errorCount, "total", len(participants))
	return nil
}

// RecoverParticipant recovers state for a single conversation participant
func (r *ConversationFlowRecovery) RecoverParticipant(ctx context.Context, participantID string, participant interface{}, registry *recovery.RecoveryRegistry) error {
	p, ok := participant.(models.ConversationParticipant)
	if !ok {
		return fmt.Errorf("invalid participant type for conversation recovery")
	}

	// Conversation participants only need response handlers (no timers)
	handlerInfo := recovery.ResponseHandlerRecoveryInfo{
		PhoneNumber:   p.PhoneNumber,
		ParticipantID: participantID,
		FlowType:      models.FlowTypeConversation,
		HandlerType:   "conversation",
		TTL:           24 * time.Hour,
	}

	if err := registry.RecoverResponseHandler(handlerInfo); err != nil {
		return fmt.Errorf("failed to register response handler: %w", err)
	}

	slog.Debug("Successfully recovered conversation participant", 
		"participantID", participantID, "phone", p.PhoneNumber)
	return nil
}
