// Package flow defines state management interfaces for stateful flows.
package flow

import (
	"context"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// StateManager defines the interface for managing flow state.
type StateManager interface {
	// GetCurrentState retrieves the current state for a participant in a flow
	GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error)

	// SetCurrentState updates the current state for a participant in a flow
	SetCurrentState(ctx context.Context, participantID string, flowType models.FlowType, state models.StateType) error

	// GetStateData retrieves additional data associated with the participant's state
	GetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey) (string, error)

	// SetStateData stores additional data associated with the participant's state
	SetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey, value string) error

	// TransitionState transitions from one state to another
	TransitionState(ctx context.Context, participantID string, flowType models.FlowType, fromState, toState models.StateType) error

	// ResetState removes all state data for a participant in a flow
	ResetState(ctx context.Context, participantID string, flowType models.FlowType) error
}

// Dependencies holds all dependencies that can be injected into flow generators.
type Dependencies struct {
	StateManager StateManager
	Timer        models.Timer
	// Note: Store interface should be accessed through StateManager to avoid circular imports
}

// StatefulGenerator extends the Generator interface for flows that need state management.
type StatefulGenerator interface {
	Generator
	// SetDependencies injects dependencies into the generator
	SetDependencies(deps Dependencies)
}
