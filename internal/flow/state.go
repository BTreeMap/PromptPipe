// Package flow defines state management interfaces for stateful flows.
package flow

import (
	"context"
	"time"

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

// Timer defines the interface for scheduling delayed actions.
type Timer interface {
	// ScheduleAfter schedules a function to run after a delay and returns a timer ID
	ScheduleAfter(delay time.Duration, fn func()) (string, error)

	// ScheduleAt schedules a function to run at a specific time and returns a timer ID
	ScheduleAt(when time.Time, fn func()) (string, error)

	// Cancel cancels a scheduled function by ID
	Cancel(id string) error

	// Stop cancels all scheduled timers
	Stop()
}

// Dependencies holds all dependencies that can be injected into flow generators.
type Dependencies struct {
	StateManager StateManager
	Timer        Timer
	// Note: Store interface should be accessed through StateManager to avoid circular imports
}

// StatefulGenerator extends the Generator interface for flows that need state management.
type StatefulGenerator interface {
	Generator
	// SetDependencies injects dependencies into the generator
	SetDependencies(deps Dependencies)
}
