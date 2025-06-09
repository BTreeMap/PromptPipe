// Package flow defines state management interfaces for stateful flows.
package flow

import (
	"context"
	"time"
)

// StateManager defines the interface for managing flow state.
type StateManager interface {
	// GetCurrentState retrieves the current state for a participant in a flow
	GetCurrentState(ctx context.Context, participantID, flowType string) (string, error)

	// SetCurrentState updates the current state for a participant in a flow
	SetCurrentState(ctx context.Context, participantID, flowType, state string) error

	// GetStateData retrieves additional data associated with the participant's state
	GetStateData(ctx context.Context, participantID, flowType, key string) (string, error)

	// SetStateData stores additional data associated with the participant's state
	SetStateData(ctx context.Context, participantID, flowType, key, value string) error

	// TransitionState transitions from one state to another
	TransitionState(ctx context.Context, participantID, flowType, fromState, toState string) error

	// ResetState removes all state data for a participant in a flow
	ResetState(ctx context.Context, participantID, flowType string) error
}

// Timer defines the interface for scheduling delayed actions.
type Timer interface {
	// ScheduleAfter schedules a function to run after a delay
	ScheduleAfter(delay time.Duration, fn func()) error

	// ScheduleAt schedules a function to run at a specific time
	ScheduleAt(when time.Time, fn func()) error

	// Cancel cancels a scheduled function (implementation dependent)
	Cancel(id string) error
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
