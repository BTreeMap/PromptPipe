// Package flow defines a common interface for coordinator implementations.
package flow

import (
	"context"

	"github.com/openai/openai-go"
)

// Coordinator defines the minimal behavior needed from a coordinator module.
// Both the LLM-driven CoordinatorModule and the static, rule-based coordinator
// should implement this interface to be swappable.
type Coordinator interface {
	// LoadSystemPrompt loads any system prompt or configuration needed.
	LoadSystemPrompt() error

	// ProcessMessageWithHistory handles a user message and may update the provided
	// conversation history in-place. Returns the assistant's reply to send.
	ProcessMessageWithHistory(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error)
}
