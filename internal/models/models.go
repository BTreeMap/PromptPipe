// Package models defines the core data structures for PromptPipe.
//
// It includes types for prompts and delivery/read receipts, which are shared across modules.
package models

// PromptType defines how the prompt content is determined.
type PromptType string

const (
	// PromptTypeStatic sends a static message body.
	PromptTypeStatic PromptType = "static"
	// PromptTypeGenAI generates message content using GenAI.
	PromptTypeGenAI PromptType = "genai"
	// PromptTypeBranch sends a prompt with selectable branches.
	PromptTypeBranch PromptType = "branch"
)

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
	Body          string         `json:"body,omitempty"`
	SystemPrompt  string         `json:"system_prompt,omitempty"`
	UserPrompt    string         `json:"user_prompt,omitempty"`
	BranchOptions []BranchOption `json:"branch_options,omitempty"`
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
