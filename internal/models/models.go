// Package models defines the core data structures for PromptPipe.
//
// It includes types for prompts and delivery/read receipts, which are shared across modules.
package models

import (
	"errors"
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

// APIResponseFactory provides a centralized factory for creating consistent API responses.
type APIResponseFactory struct{}

// NewFactory creates a new APIResponseFactory instance.
func NewFactory() *APIResponseFactory {
	return &APIResponseFactory{}
}

// Success creates a successful API response with optional result data.
func (f *APIResponseFactory) Success(result interface{}) APIResponse {
	return APIResponse{
		Status: string(APIStatusOK),
		Result: result,
	}
}

// SuccessWithMessage creates a successful API response with a message and optional result data.
func (f *APIResponseFactory) SuccessWithMessage(message string, result interface{}) APIResponse {
	return APIResponse{
		Status:  string(APIStatusOK),
		Message: message,
		Result:  result,
	}
}

// Error creates an error API response with a message.
func (f *APIResponseFactory) Error(message string) APIResponse {
	return APIResponse{
		Status:  string(APIStatusError),
		Message: message,
	}
}

// Scheduled creates a scheduled API response.
func (f *APIResponseFactory) Scheduled() APIResponse {
	return APIResponse{
		Status: string(APIStatusScheduled),
	}
}

// ScheduledWithMessage creates a scheduled API response with a message.
func (f *APIResponseFactory) ScheduledWithMessage(message string) APIResponse {
	return APIResponse{
		Status:  string(APIStatusScheduled),
		Message: message,
	}
}

// Recorded creates a recorded API response.
func (f *APIResponseFactory) Recorded() APIResponse {
	return APIResponse{
		Status: string(APIStatusRecorded),
	}
}

// RecordedWithMessage creates a recorded API response with a message.
func (f *APIResponseFactory) RecordedWithMessage(message string) APIResponse {
	return APIResponse{
		Status:  string(APIStatusRecorded),
		Message: message,
	}
}

// Global factory instance for convenience
var Factory = NewFactory()

// Backward compatibility functions (deprecated, use Factory instead)

// NewAPIResponse creates a standard API response with the given status.
// Deprecated: Use Factory.Success() or Factory.Error() instead.
func NewAPIResponse(status APIStatus) APIResponse {
	return APIResponse{Status: string(status)}
}

// NewAPIResponseWithMessage creates a standard API response with status and message.
// Deprecated: Use Factory methods instead.
func NewAPIResponseWithMessage(status APIStatus, message string) APIResponse {
	return APIResponse{Status: string(status), Message: message}
}

// NewOKResponse creates a standard "ok" API response.
// Deprecated: Use Factory.Success() instead.
func NewOKResponse() APIResponse {
	return Factory.Success(nil)
}

// NewScheduledResponse creates a standard "scheduled" API response.
// Deprecated: Use Factory.Scheduled() instead.
func NewScheduledResponse() APIResponse {
	return Factory.Scheduled()
}

// NewRecordedResponse creates a standard "recorded" API response.
// Deprecated: Use Factory.Recorded() instead.
func NewRecordedResponse() APIResponse {
	return Factory.Recorded()
}

// NewErrorResponse creates a standard "error" API response with a message.
// Deprecated: Use Factory.Error() instead.
func NewErrorResponse(message string) APIResponse {
	return Factory.Error(message)
}
