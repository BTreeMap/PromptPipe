// Package flow provides concrete implementations of state management.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// StoreBasedStateManager implements StateManager using a Store backend.
type StoreBasedStateManager struct {
	store store.Store
}

// NewStoreBasedStateManager creates a new StateManager backed by a Store.
func NewStoreBasedStateManager(st store.Store) *StoreBasedStateManager {
	slog.Debug("Creating StoreBasedStateManager")
	return &StoreBasedStateManager{store: st}
}

// GetCurrentState retrieves the current state for a participant in a flow.
func (sm *StoreBasedStateManager) GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error) {
	slog.Debug("StateManager GetCurrentState", "participantID", participantID, "flowType", flowType)

	flowState, err := sm.store.GetFlowState(participantID, string(flowType))
	if err != nil {
		slog.Error("StateManager GetCurrentState error", "error", err, "participantID", participantID, "flowType", flowType)
		return "", err
	}

	if flowState == nil {
		slog.Debug("StateManager GetCurrentState not found", "participantID", participantID, "flowType", flowType)
		return "", nil
	}

	slog.Debug("StateManager GetCurrentState found", "participantID", participantID, "flowType", flowType, "state", flowState.CurrentState)
	return flowState.CurrentState, nil
}

// SetCurrentState updates the current state for a participant in a flow.
func (sm *StoreBasedStateManager) SetCurrentState(ctx context.Context, participantID string, flowType FlowType, state StateType) error {
	slog.Debug("StateManager SetCurrentState", "participantID", participantID, "flowType", flowType, "state", state)

	// Get existing state or create new one
	flowState, err := sm.store.GetFlowState(participantID, string(flowType))
	if err != nil {
		slog.Error("StateManager SetCurrentState get error", "error", err, "participantID", participantID, "flowType", flowType)
		return err
	}

	now := time.Now()
	if flowState == nil {
		// Create new flow state
		flowState = &models.FlowState{
			ParticipantID: participantID,
			FlowType:      flowType,
			CurrentState:  state,
			StateData:     make(map[DataKey]string),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	} else {
		// Update existing flow state
		flowState.CurrentState = state
		flowState.UpdatedAt = now
	}

	err = sm.store.SaveFlowState(*flowState)
	if err != nil {
		slog.Error("StateManager SetCurrentState save error", "error", err, "participantID", participantID, "flowType", flowType, "state", state)
		return err
	}

	slog.Debug("StateManager SetCurrentState succeeded", "participantID", participantID, "flowType", flowType, "state", state)
	return nil
}

// GetStateData retrieves additional data associated with the participant's state.
func (sm *StoreBasedStateManager) GetStateData(ctx context.Context, participantID string, flowType FlowType, key DataKey) (string, error) {
	slog.Debug("StateManager GetStateData", "participantID", participantID, "flowType", flowType, "key", key)

	flowState, err := sm.store.GetFlowState(participantID, string(flowType))
	if err != nil {
		slog.Error("StateManager GetStateData error", "error", err, "participantID", participantID, "flowType", flowType, "key", key)
		return "", err
	}

	if flowState == nil || flowState.StateData == nil {
		slog.Debug("StateManager GetStateData not found", "participantID", participantID, "flowType", flowType, "key", key)
		return "", nil
	}

	value, exists := flowState.StateData[key]
	if !exists {
		slog.Debug("StateManager GetStateData key not found", "participantID", participantID, "flowType", flowType, "key", key)
		return "", nil
	}

	slog.Debug("StateManager GetStateData found", "participantID", participantID, "flowType", flowType, "key", key)
	return value, nil
}

// SetStateData stores additional data associated with the participant's state.
func (sm *StoreBasedStateManager) SetStateData(ctx context.Context, participantID string, flowType FlowType, key DataKey, value string) error {
	slog.Debug("StateManager SetStateData", "participantID", participantID, "flowType", flowType, "key", key)

	// Get existing state or create new one
	flowState, err := sm.store.GetFlowState(participantID, string(flowType))
	if err != nil {
		slog.Error("StateManager SetStateData get error", "error", err, "participantID", participantID, "flowType", flowType, "key", key)
		return err
	}

	now := time.Now()
	if flowState == nil {
		// Create new flow state with empty current state
		flowState = &models.FlowState{
			ParticipantID: participantID,
			FlowType:      flowType,
			CurrentState:  "",
			StateData:     map[DataKey]string{key: value},
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	} else {
		// Update existing flow state
		if flowState.StateData == nil {
			flowState.StateData = make(map[DataKey]string)
		}
		flowState.StateData[key] = value
		flowState.UpdatedAt = now
	}

	err = sm.store.SaveFlowState(*flowState)
	if err != nil {
		slog.Error("StateManager SetStateData save error", "error", err, "participantID", participantID, "flowType", flowType, "key", key)
		return err
	}

	slog.Debug("StateManager SetStateData succeeded", "participantID", participantID, "flowType", flowType, "key", key)
	return nil
}

// TransitionState transitions from one state to another.
func (sm *StoreBasedStateManager) TransitionState(ctx context.Context, participantID string, flowType FlowType, fromState, toState StateType) error {
	slog.Debug("StateManager TransitionState", "participantID", participantID, "flowType", flowType, "from", fromState, "to", toState)

	// Verify current state matches expected fromState
	currentState, err := sm.GetCurrentState(ctx, participantID, flowType)
	if err != nil {
		slog.Error("StateManager TransitionState get current state error", "error", err, "participantID", participantID, "flowType", flowType)
		return err
	}

	if currentState != fromState {
		err := fmt.Errorf("invalid state transition: expected %s, current is %s", fromState, currentState)
		slog.Error("StateManager TransitionState invalid transition", "error", err, "participantID", participantID, "flowType", flowType, "expected", fromState, "current", currentState)
		return err
	}

	// Perform transition
	err = sm.SetCurrentState(ctx, participantID, flowType, toState)
	if err != nil {
		slog.Error("StateManager TransitionState set state error", "error", err, "participantID", participantID, "flowType", flowType, "to", toState)
		return err
	}

	slog.Info("StateManager TransitionState succeeded", "participantID", participantID, "flowType", flowType, "from", fromState, "to", toState)
	return nil
}

// ResetState removes all state data for a participant in a flow.
func (sm *StoreBasedStateManager) ResetState(ctx context.Context, participantID string, flowType FlowType) error {
	slog.Debug("StateManager ResetState", "participantID", participantID, "flowType", flowType)

	err := sm.store.DeleteFlowState(participantID, string(flowType))
	if err != nil {
		slog.Error("StateManager ResetState error", "error", err, "participantID", participantID, "flowType", flowType)
		return err
	}

	slog.Info("StateManager ResetState succeeded", "participantID", participantID, "flowType", flowType)
	return nil
}
