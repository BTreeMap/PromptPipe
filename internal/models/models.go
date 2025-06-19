// Package models defines the core data structures for PromptPipe.
//
// It includes types for prompts and delivery/read receipts, which are shared across modules.
package models

import (
	"errors"
	"time"
)

// PromptType defines how the prompt content is determined.
type PromptType string

const (
	// PromptTypeStatic sends a static message body.
	PromptTypeStatic PromptType = "static"
	// PromptTypeGenAI generates message content using GenAI.
	PromptTypeGenAI PromptType = "genai"
	// PromptTypeBranch sends a prompt with selectable branches.
	PromptTypeBranch PromptType = "branch"
	// PromptTypeCustom allows pluggable custom flow generators
	PromptTypeCustom PromptType = "custom"
)

// Validation constants for input validation
const (
	// MaxPromptBodyLength defines the maximum allowed length for prompt body content
	MaxPromptBodyLength = 4096
	// MaxBranchLabelLength defines the maximum allowed length for branch option labels
	MaxBranchLabelLength = 100
	// MaxBranchBodyLength defines the maximum allowed length for branch option body content
	MaxBranchBodyLength = 1000
	// MaxBranchOptionsCount defines the maximum number of branch options allowed
	MaxBranchOptionsCount = 10
	// MinBranchOptionsCount defines the minimum number of branch options required
	MinBranchOptionsCount = 2
)

// Error variables for better error handling and testability
var (
	ErrEmptyRecipient            = errors.New("recipient cannot be empty")
	ErrInvalidPromptType         = errors.New("invalid prompt type")
	ErrEmptyBody                 = errors.New("body is required for static prompts")
	ErrPromptBodyTooLong         = errors.New("prompt body exceeds maximum length")
	ErrMissingSystemPrompt       = errors.New("system prompt is required for GenAI prompts")
	ErrMissingUserPrompt         = errors.New("user prompt is required for GenAI prompts")
	ErrMissingBranchOptions      = errors.New("branch options are required for branch prompts")
	ErrInsufficientBranchOptions = errors.New("insufficient branch options")
	ErrTooManyBranchOptions      = errors.New("too many branch options")
	ErrEmptyBranchLabel          = errors.New("branch label cannot be empty")
	ErrBranchLabelTooLong        = errors.New("branch label exceeds maximum length")
	ErrEmptyBranchBody           = errors.New("branch body cannot be empty")
	ErrBranchBodyTooLong         = errors.New("branch body exceeds maximum length")
)

// IsValidPromptType checks if the given prompt type is supported.
func IsValidPromptType(pt PromptType) bool {
	switch pt {
	case PromptTypeStatic, PromptTypeGenAI, PromptTypeBranch, PromptTypeCustom:
		return true
	default:
		return false
	}
}

// BranchOption represents a selectable option for branch-type prompts.
type BranchOption struct {
	Label string `json:"label"` // option identifier shown to user
	Body  string `json:"body"`  // message content if selected
}

// Prompt represents a message to be sent, supporting static, GenAI, or branch types.
type Prompt struct {
	To            string         `json:"to"`
	Cron          string         `json:"cron,omitempty"`
	Type          PromptType     `json:"type,omitempty"`
	State         string         `json:"state,omitempty"` // current state for custom flows
	Body          string         `json:"body,omitempty"`
	SystemPrompt  string         `json:"system_prompt,omitempty"`
	UserPrompt    string         `json:"user_prompt,omitempty"`
	BranchOptions []BranchOption `json:"branch_options,omitempty"`
}

// Validate performs comprehensive validation on a Prompt structure.
func (p *Prompt) Validate() error {
	// Check recipient
	if p.To == "" {
		return ErrEmptyRecipient
	}

	// Check prompt type
	if !IsValidPromptType(p.Type) {
		return ErrInvalidPromptType
	}

	// Type-specific validation
	switch p.Type {
	case PromptTypeStatic:
		return p.validateStatic()
	case PromptTypeGenAI:
		return p.validateGenAI()
	case PromptTypeBranch:
		return p.validateBranch()
	case PromptTypeCustom:
		// Custom types may have different validation requirements
		return nil
	}

	return nil
}

// validateStatic validates static prompt requirements.
func (p *Prompt) validateStatic() error {
	if p.Body == "" {
		return ErrEmptyBody
	}
	if len(p.Body) > MaxPromptBodyLength {
		return ErrPromptBodyTooLong
	}
	return nil
}

// validateGenAI validates GenAI prompt requirements.
func (p *Prompt) validateGenAI() error {
	if p.SystemPrompt == "" {
		return ErrMissingSystemPrompt
	}
	if p.UserPrompt == "" {
		return ErrMissingUserPrompt
	}
	return nil
}

// validateBranch validates branch prompt requirements.
func (p *Prompt) validateBranch() error {
	if len(p.BranchOptions) == 0 {
		return ErrMissingBranchOptions
	}
	if len(p.BranchOptions) < MinBranchOptionsCount {
		return ErrInsufficientBranchOptions
	}
	if len(p.BranchOptions) > MaxBranchOptionsCount {
		return ErrTooManyBranchOptions
	}

	for _, option := range p.BranchOptions {
		if option.Label == "" {
			return ErrEmptyBranchLabel
		}
		if len(option.Label) > MaxBranchLabelLength {
			return ErrBranchLabelTooLong
		}
		if option.Body == "" {
			return ErrEmptyBranchBody
		}
		if len(option.Body) > MaxBranchBodyLength {
			return ErrBranchBodyTooLong
		}
	}

	return nil
}

// MessageStatus represents the delivery status of a message.
type MessageStatus string

const (
	// MessageStatusSent indicates the message was sent.
	MessageStatusSent MessageStatus = "sent"
	// MessageStatusDelivered indicates the message was delivered.
	MessageStatusDelivered MessageStatus = "delivered"
	// MessageStatusRead indicates the message was read.
	MessageStatusRead MessageStatus = "read"
	// MessageStatusFailed indicates the message failed to send.
	MessageStatusFailed MessageStatus = "failed"
	// MessageStatusCancelled indicates the message was cancelled.
	MessageStatusCancelled MessageStatus = "cancelled"
)

// APIStatus represents the status of an API response.
type APIStatus string

const (
	// APIStatusOK indicates an API request completed successfully.
	APIStatusOK APIStatus = "ok"
	// APIStatusError indicates an API request failed with an error.
	APIStatusError APIStatus = "error"
	// APIStatusScheduled indicates an API request resulted in scheduled content.
	APIStatusScheduled APIStatus = "scheduled"
	// APIStatusRecorded indicates data was successfully recorded via API.
	APIStatusRecorded APIStatus = "recorded"
)

type Receipt struct {
	To     string        `json:"to"`
	Status MessageStatus `json:"status"`
	Time   int64         `json:"time"`
}

// Response represents an incoming message response from a participant.
type Response struct {
	From string `json:"from"`
	Body string `json:"body"`
	Time int64  `json:"time"`
}

// API Response types for consistent JSON responses

// APIResponse represents a standard API response with a status and optional data.
type APIResponse struct {
	Status  string      `json:"status"`            // status of the API response
	Message string      `json:"message,omitempty"` // optional message for error responses or additional info
	Result  interface{} `json:"result,omitempty"`  // optional result data for successful responses
}

// APIResponseBuilder provides a fluent interface for building API responses.
type APIResponseBuilder struct {
	response APIResponse
}

// NewAPIResponseBuilder creates a new APIResponseBuilder instance.
func NewAPIResponseBuilder() *APIResponseBuilder {
	return &APIResponseBuilder{
		response: APIResponse{},
	}
}

// WithStatus sets the status of the API response.
func (b *APIResponseBuilder) WithStatus(status APIStatus) *APIResponseBuilder {
	b.response.Status = string(status)
	return b
}

// WithMessage sets the message of the API response.
func (b *APIResponseBuilder) WithMessage(message string) *APIResponseBuilder {
	b.response.Message = message
	return b
}

// WithResult sets the result data of the API response.
func (b *APIResponseBuilder) WithResult(result interface{}) *APIResponseBuilder {
	b.response.Result = result
	return b
}

// Build constructs and returns the final APIResponse.
func (b *APIResponseBuilder) Build() APIResponse {
	return b.response
}

// Convenience functions for common response patterns

// Success creates a successful API response with optional result data.
func Success(result interface{}) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusOK).
		WithResult(result).
		Build()
}

// SuccessWithMessage creates a successful API response with a message and optional result data.
func SuccessWithMessage(message string, result interface{}) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusOK).
		WithMessage(message).
		WithResult(result).
		Build()
}

// Error creates an error API response with a message.
func Error(message string) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusError).
		WithMessage(message).
		Build()
}

// Scheduled creates a scheduled API response.
func Scheduled() APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusScheduled).
		Build()
}

// ScheduledWithMessage creates a scheduled API response with a message.
func ScheduledWithMessage(message string) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusScheduled).
		WithMessage(message).
		Build()
}

// Recorded creates a recorded API response.
func Recorded() APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusRecorded).
		Build()
}

// RecordedWithMessage creates a recorded API response with a message.
func RecordedWithMessage(message string) APIResponse {
	return NewAPIResponseBuilder().
		WithStatus(APIStatusRecorded).
		WithMessage(message).
		Build()
}

// InterventionParticipantStatus represents the enrollment status of a participant.
type InterventionParticipantStatus string

const (
	// ParticipantStatusActive indicates the participant is actively enrolled.
	ParticipantStatusActive InterventionParticipantStatus = "active"
	// ParticipantStatusPaused indicates the participant is temporarily paused.
	ParticipantStatusPaused InterventionParticipantStatus = "paused"
	// ParticipantStatusCompleted indicates the participant has completed the intervention.
	ParticipantStatusCompleted InterventionParticipantStatus = "completed"
	// ParticipantStatusWithdrawn indicates the participant has withdrawn.
	ParticipantStatusWithdrawn InterventionParticipantStatus = "withdrawn"
)

// InterventionParticipant represents a participant in the micro health intervention study.
type InterventionParticipant struct {
	ID              string                        `json:"id"`
	PhoneNumber     string                        `json:"phone_number"`
	Name            string                        `json:"name,omitempty"`
	Timezone        string                        `json:"timezone,omitempty"`     // e.g., "America/New_York"
	Status          InterventionParticipantStatus `json:"status"`
	EnrolledAt      time.Time                     `json:"enrolled_at"`
	DailyPromptTime string                        `json:"daily_prompt_time"`      // e.g., "10:00"
	WeeklyReset     time.Time                     `json:"weekly_reset,omitempty"` // When to send weekly summary
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
}

// InterventionResponse represents a participant's response in the intervention flow.
type InterventionResponse struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	State         string    `json:"state"`           // Which state they were in when responding
	ResponseText  string    `json:"response_text"`   // The actual response
	ResponseType  string    `json:"response_type"`   // e.g., "commitment", "feeling", "completion"
	Timestamp     time.Time `json:"timestamp"`
}

// InterventionEnrollmentRequest represents the payload for enrolling a participant.
type InterventionEnrollmentRequest struct {
	PhoneNumber     string `json:"phone_number" validate:"required"`
	Name            string `json:"name,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
	DailyPromptTime string `json:"daily_prompt_time,omitempty"` // defaults to "10:00"
}

// InterventionResponseRequest represents the payload for processing a participant response.
type InterventionResponseRequest struct {
	ResponseText string `json:"response_text" validate:"required"`
	Context      string `json:"context,omitempty"` // Optional context about how the response was received
}

// InterventionStateAdvanceRequest represents the payload for manually advancing participant state.
type InterventionStateAdvanceRequest struct {
	ToState string `json:"to_state" validate:"required"`
	Reason  string `json:"reason,omitempty"` // Optional reason for manual advancement
}

// InterventionParticipantUpdate represents the payload for updating a participant.
type InterventionParticipantUpdate struct {
	Name            *string                        `json:"name,omitempty"`
	Timezone        *string                        `json:"timezone,omitempty"`
	DailyPromptTime *string                        `json:"daily_prompt_time,omitempty"`
	Status          *InterventionParticipantStatus `json:"status,omitempty"`
}

// InterventionStats represents statistics about the intervention.
type InterventionStats struct {
	TotalParticipants      int                                   `json:"total_participants"`
	ParticipantsByStatus   map[InterventionParticipantStatus]int `json:"participants_by_status"`
	ParticipantsByState    map[string]int                        `json:"participants_by_state"`
	TotalResponses         int                                   `json:"total_responses"`
	ResponsesByType        map[string]int                        `json:"responses_by_type"`
	CompletionRate         float64                               `json:"completion_rate"`
	AverageResponseTime    float64                               `json:"average_response_time_minutes"`
}

// Validate validates an InterventionEnrollmentRequest.
func (r *InterventionEnrollmentRequest) Validate() error {
	if r.PhoneNumber == "" {
		return errors.New("phone_number is required")
	}

	// Validate timezone if provided
	if r.Timezone != "" {
		if _, err := time.LoadLocation(r.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}

	// Validate daily prompt time format if provided
	if r.DailyPromptTime != "" {
		if _, err := time.Parse("15:04", r.DailyPromptTime); err != nil {
			return errors.New("daily_prompt_time must be in HH:MM format")
		}
	}

	return nil
}

// IsValidParticipantStatus checks if the given participant status is valid.
func IsValidParticipantStatus(status InterventionParticipantStatus) bool {
	switch status {
	case ParticipantStatusActive, ParticipantStatusPaused, ParticipantStatusCompleted, ParticipantStatusWithdrawn:
		return true
	default:
		return false
	}
}
