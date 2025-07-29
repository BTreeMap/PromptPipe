// Package models defines user profile structures for the three-bot conversation architecture.
package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// UserProfile represents the complete user profile built by the intake bot
// and used by the prompt generator and feedback tracker bots.
type UserProfile struct {
	// Basic participant info
	ParticipantID string    `json:"participant_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Intake bot collected data
	TargetBehavior    string `json:"target_behavior"`    // The chosen habit domain/goal
	MotivationalFrame string `json:"motivational_frame"` // Why this matters to the user
	PreferredTime     string `json:"preferred_time"`     // When they want nudges
	PromptAnchor      string `json:"prompt_anchor"`      // When/where habit fits in their day
	AdditionalNotes   string `json:"additional_notes"`   // Any extra personalization info

	// Prompt generator data
	LastPrompt           string    `json:"last_prompt,omitempty"`            // Last habit prompt sent
	LastPromptSentAt     time.Time `json:"last_prompt_sent_at,omitempty"`    // When last prompt was sent
	LastSuccessfulPrompt string    `json:"last_successful_prompt,omitempty"` // Last prompt that worked

	// Feedback tracker data
	LastFeedback   string    `json:"last_feedback,omitempty"`    // Last user feedback received
	LastBarrier    string    `json:"last_barrier,omitempty"`     // Last reported barrier
	LastTweak      string    `json:"last_tweak,omitempty"`       // Last requested modification
	FeedbackCount  int       `json:"feedback_count"`             // Total feedback interactions
	SuccessCount   int       `json:"success_count"`              // Number of successful attempts
	LastFeedbackAt time.Time `json:"last_feedback_at,omitempty"` // When last feedback was received

	// Profile completion tracking
	IntakeComplete  bool `json:"intake_complete"`   // Whether intake bot finished successfully
	ReadyForPrompts bool `json:"ready_for_prompts"` // Whether user is ready to receive prompts
}

// Validate ensures the user profile has required fields.
func (up *UserProfile) Validate() error {
	if up.ParticipantID == "" {
		return fmt.Errorf("participant_id is required")
	}
	return nil
}

// ToJSON serializes the user profile to JSON string.
func (up *UserProfile) ToJSON() (string, error) {
	data, err := json.Marshal(up)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user profile: %w", err)
	}
	return string(data), nil
}

// FromJSON deserializes a user profile from JSON string.
func (up *UserProfile) FromJSON(jsonStr string) error {
	if err := json.Unmarshal([]byte(jsonStr), up); err != nil {
		return fmt.Errorf("failed to unmarshal user profile: %w", err)
	}
	return nil
}

// UpdateLastFeedback updates the feedback tracking fields.
func (up *UserProfile) UpdateLastFeedback(feedback string, wasSuccessful bool) {
	up.LastFeedback = feedback
	up.LastFeedbackAt = time.Now()
	up.FeedbackCount++
	if wasSuccessful {
		up.SuccessCount++
	}
	up.UpdatedAt = time.Now()
}

// UpdatePrompt updates the prompt tracking fields.
func (up *UserProfile) UpdatePrompt(prompt string, wasSuccessful bool) {
	up.LastPrompt = prompt
	up.LastPromptSentAt = time.Now()
	if wasSuccessful {
		up.LastSuccessfulPrompt = prompt
	}
	up.UpdatedAt = time.Now()
}

// ConversationBotType represents which bot should handle the next response.
type ConversationBotType string

const (
	// BotTypeIntake handles the initial profile building conversation
	BotTypeIntake ConversationBotType = "intake"
	// BotTypePromptGenerator handles prompt delivery and immediate follow-up
	BotTypePromptGenerator ConversationBotType = "prompt_generator"
	// BotTypeFeedbackTracker handles feedback collection and profile updates
	BotTypeFeedbackTracker ConversationBotType = "feedback_tracker"
)

// ConversationState represents the current state of the three-bot conversation flow.
type ConversationState struct {
	CurrentBot       ConversationBotType `json:"current_bot"`
	IntakeStep       int                 `json:"intake_step"`       // Current step in intake process (1-9)
	AwaitingTryNow   bool                `json:"awaiting_try_now"`  // Whether waiting for "try now" response
	PromptDelivered  bool                `json:"prompt_delivered"`  // Whether habit prompt was delivered
	AwaitingFeedback bool                `json:"awaiting_feedback"` // Whether waiting for completion feedback
}

// Additional state constants for the three-bot architecture
const (
	// Intake bot states
	StateIntakeWelcome        StateType = "INTAKE_WELCOME"
	StateIntakeHabitDomain    StateType = "INTAKE_HABIT_DOMAIN"
	StateIntakeMotivation     StateType = "INTAKE_MOTIVATION"
	StateIntakeExistingGoal   StateType = "INTAKE_EXISTING_GOAL"
	StateIntakeSuggestOptions StateType = "INTAKE_SUGGEST_OPTIONS"
	StateIntakePreference     StateType = "INTAKE_PREFERENCE"
	StateIntakeOutcome        StateType = "INTAKE_OUTCOME"
	StateIntakeLanguage       StateType = "INTAKE_LANGUAGE"
	StateIntakeTone           StateType = "INTAKE_TONE"
	StateIntakeComplete       StateType = "INTAKE_COMPLETE"

	// Prompt generator states
	StatePromptGenerate  StateType = "PROMPT_GENERATE"
	StatePromptDelivered StateType = "PROMPT_DELIVERED"
	StatePromptTryNow    StateType = "PROMPT_TRY_NOW"

	// Feedback tracker states
	StateFeedbackAwaitingCompletion StateType = "FEEDBACK_AWAITING_COMPLETION"
	StateFeedbackProcessing         StateType = "FEEDBACK_PROCESSING"
)

// Data keys for the three-bot architecture
const (
	DataKeyUserProfile       DataKey = "userProfile"
	DataKeyConversationState DataKey = "conversationState"
	DataKeyIntakeStep        DataKey = "intakeStep"
	DataKeyCurrentBot        DataKey = "currentBot"
)
