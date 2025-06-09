// Package models defines state management structures for PromptPipe flows.
package models

import "time"

// FlowState represents the current state of a participant in a flow.
type FlowState struct {
	ParticipantID string            `json:"participant_id"`
	FlowType      string            `json:"flow_type"`
	CurrentState  string            `json:"current_state"`
	StateData     map[string]string `json:"state_data,omitempty"` // Additional state-specific data
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// StateTransition represents a transition between states in a flow.
type StateTransition struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Condition string `json:"condition,omitempty"` // Optional condition for the transition
}
