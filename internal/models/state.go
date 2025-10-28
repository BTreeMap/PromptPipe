// Package models defines state management structures for PromptPipe flows.
package models

import (
	"time"
)

// FlowState represents the current state of a participant in a flow.
type FlowState struct {
	ParticipantID string             `json:"participant_id"`
	FlowType      FlowType           `json:"flow_type"`
	CurrentState  StateType          `json:"current_state"`
	StateData     map[DataKey]string `json:"state_data,omitempty"` // Additional state-specific data (key-value pairs)
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// StateTransition represents a transition between states in a flow.
type StateTransition struct {
	FromState StateType `json:"from_state"`
	ToState   StateType `json:"to_state"`
	Condition string    `json:"condition,omitempty"` // Optional condition for the transition
}

// TimerCallbackType defines the type of timer callback for recovery
type TimerCallbackType string

const (
	// TimerCallbackScheduledPrompt for daily habit prompt scheduling
	TimerCallbackScheduledPrompt TimerCallbackType = "scheduled_prompt"
	// TimerCallbackFeedbackInitial for initial feedback timeout
	TimerCallbackFeedbackInitial TimerCallbackType = "feedback_initial"
	// TimerCallbackFeedbackFollowup for feedback follow-up session
	TimerCallbackFeedbackFollowup TimerCallbackType = "feedback_followup"
	// TimerCallbackStateTransition for delayed state transitions
	TimerCallbackStateTransition TimerCallbackType = "state_transition"
	// TimerCallbackReminder for daily prompt reminders
	TimerCallbackReminder TimerCallbackType = "reminder"
	// TimerCallbackAutoFeedback for auto feedback enforcement
	TimerCallbackAutoFeedback TimerCallbackType = "auto_feedback"
)

// TimerRecord represents a persisted timer for recovery after restart
type TimerRecord struct {
	ID                   string            `json:"id" db:"id"`
	ParticipantID        string            `json:"participant_id" db:"participant_id"`
	FlowType             FlowType          `json:"flow_type" db:"flow_type"`
	TimerType            string            `json:"timer_type" db:"timer_type"` // "once" or "recurring"
	StateType            StateType         `json:"state_type,omitempty" db:"state_type"`
	DataKey              DataKey           `json:"data_key,omitempty" db:"data_key"`
	CallbackType         TimerCallbackType `json:"callback_type" db:"callback_type"`
	CallbackParams       map[string]string `json:"callback_params,omitempty" db:"callback_params"` // JSON-serialized parameters
	ScheduledAt          time.Time         `json:"scheduled_at" db:"scheduled_at"`
	ExpiresAt            *time.Time        `json:"expires_at,omitempty" db:"expires_at"` // For one-time timers
	OriginalDelaySeconds *int64            `json:"original_delay_seconds,omitempty" db:"original_delay_seconds"`
	ScheduleJSON         string            `json:"schedule_json,omitempty" db:"schedule_json"` // JSON serialized Schedule
	NextRun              *time.Time        `json:"next_run,omitempty" db:"next_run"`
	CreatedAt            time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at" db:"updated_at"`
}
