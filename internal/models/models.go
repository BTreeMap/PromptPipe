// Package models defines the core data structures for PromptPipe.
//
// It includes types for prompts and delivery/read receipts, which are shared across modules.
package models

import "fmt"

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

// Validation error constants
const (
	ErrMsgEmptyRecipient       = "recipient cannot be empty"
	ErrMsgInvalidPromptType    = "invalid prompt type"
	ErrMsgPromptBodyTooLong    = "prompt body exceeds maximum length"
	ErrMsgBranchLabelTooLong   = "branch label exceeds maximum length"
	ErrMsgBranchBodyTooLong    = "branch body exceeds maximum length"
	ErrMsgTooManyBranchOptions = "too many branch options"
	ErrMsgTooFewBranchOptions  = "insufficient branch options"
	ErrMsgEmptyBranchLabel     = "branch label cannot be empty"
	ErrMsgEmptyBranchBody      = "branch body cannot be empty"
	ErrMsgMissingSystemPrompt  = "system prompt is required for GenAI prompts"
	ErrMsgMissingUserPrompt    = "user prompt is required for GenAI prompts"
	ErrMsgMissingStaticBody    = "body is required for static prompts"
	ErrMsgMissingBranchOptions = "branch options are required for branch prompts"
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
		return fmt.Errorf(ErrMsgEmptyRecipient)
	}

	// Check prompt type
	if !IsValidPromptType(p.Type) {
		return fmt.Errorf(ErrMsgInvalidPromptType)
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
		return fmt.Errorf(ErrMsgMissingStaticBody)
	}
	if len(p.Body) > MaxPromptBodyLength {
		return fmt.Errorf(ErrMsgPromptBodyTooLong)
	}
	return nil
}

// validateGenAI validates GenAI prompt requirements.
func (p *Prompt) validateGenAI() error {
	if p.SystemPrompt == "" {
		return fmt.Errorf(ErrMsgMissingSystemPrompt)
	}
	if p.UserPrompt == "" {
		return fmt.Errorf(ErrMsgMissingUserPrompt)
	}
	return nil
}

// validateBranch validates branch prompt requirements.
func (p *Prompt) validateBranch() error {
	if len(p.BranchOptions) == 0 {
		return fmt.Errorf(ErrMsgMissingBranchOptions)
	}
	if len(p.BranchOptions) < MinBranchOptionsCount {
		return fmt.Errorf(ErrMsgTooFewBranchOptions)
	}
	if len(p.BranchOptions) > MaxBranchOptionsCount {
		return fmt.Errorf(ErrMsgTooManyBranchOptions)
	}

	for _, option := range p.BranchOptions {
		if option.Label == "" {
			return fmt.Errorf(ErrMsgEmptyBranchLabel)
		}
		if len(option.Label) > MaxBranchLabelLength {
			return fmt.Errorf(ErrMsgBranchLabelTooLong)
		}
		if option.Body == "" {
			return fmt.Errorf(ErrMsgEmptyBranchBody)
		}
		if len(option.Body) > MaxBranchBodyLength {
			return fmt.Errorf(ErrMsgBranchBodyTooLong)
		}
	}

	return nil
}

type StatusType string

const (
	// StatusTypeSent indicates the message was sent.
	StatusTypeSent StatusType = "sent"
	// StatusTypeDelivered indicates the message was delivered.
	StatusTypeDelivered StatusType = "delivered"
	// StatusTypeRead indicates the message was read.
	StatusTypeRead StatusType = "read"
	// StatusTypeFailed indicates the message failed to send.
	StatusTypeFailed StatusType = "failed"
	// StatusTypeError indicates an error occurred while processing the message.
	StatusTypeError StatusType = "error"
	// StatusTypeScheduled indicates the message is scheduled for future delivery.
	StatusTypeScheduled StatusType = "scheduled"
	// StatusTypeCancelled indicates the message was cancelled.
	StatusTypeCancelled StatusType = "cancelled"
)

type Receipt struct {
	To     string     `json:"to"`
	Status StatusType `json:"status"`
	Time   int64      `json:"time"`
}

// Response represents an incoming message response from a participant.
type Response struct {
	From string `json:"from"`
	Body string `json:"body"`
	Time int64  `json:"time"`
}

// API Response types for consistent JSON responses

// APIResponse represents a standard API response with a status.
type APIResponse struct {
	Status string `json:"status"`
}

// APIResponseStatus defines standard API response status values.
type APIResponseStatus string

const (
	// APIStatusOK indicates a successful operation
	APIStatusOK APIResponseStatus = "ok"
	// APIStatusScheduled indicates a job was successfully scheduled
	APIStatusScheduled APIResponseStatus = "scheduled"
	// APIStatusRecorded indicates data was successfully recorded
	APIStatusRecorded APIResponseStatus = "recorded"
	// APIStatusError indicates an error occurred
	APIStatusError APIResponseStatus = "error"
)

// NewAPIResponse creates a standard API response with the given status.
func NewAPIResponse(status APIResponseStatus) APIResponse {
	return APIResponse{Status: string(status)}
}

// NewOKResponse creates a standard "ok" API response.
func NewOKResponse() APIResponse {
	return NewAPIResponse(APIStatusOK)
}

// NewScheduledResponse creates a standard "scheduled" API response.
func NewScheduledResponse() APIResponse {
	return NewAPIResponse(APIStatusScheduled)
}

// NewRecordedResponse creates a standard "recorded" API response.
func NewRecordedResponse() APIResponse {
	return NewAPIResponse(APIStatusRecorded)
}
