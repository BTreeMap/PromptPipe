package flow

import (
	"context"
	"encoding/json"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// Helper functions for test comparisons
func intPtr(i int) *int {
	return &i
}

// scheduleEquals compares two Schedule pointers for equality
func scheduleEquals(a, b *models.Schedule) bool {
	// Both nil
	if a == nil && b == nil {
		return true
	}
	// One nil, one not
	if a == nil || b == nil {
		return false
	}
	// Compare fields
	return intPtrEquals(a.Minute, b.Minute) &&
		intPtrEquals(a.Hour, b.Hour) &&
		intPtrEquals(a.Day, b.Day) &&
		intPtrEquals(a.Month, b.Month) &&
		intPtrEquals(a.Weekday, b.Weekday) &&
		a.Timezone == b.Timezone
}

// intPtrEquals compares two int pointers for equality
func intPtrEquals(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Mock GenAI client for testing tool use
type MockGenAIClientWithTools struct {
	shouldCallTools bool
	toolCallID      string
	toolCallArgs    string
	toolName        string // Make tool name configurable
	expectError     bool   // New field to indicate if we should return error responses
}

func (m *MockGenAIClientWithTools) GeneratePrompt(system, user string) (string, error) {
	return "Basic response", nil
}

func (m *MockGenAIClientWithTools) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	return "Basic response", nil
}

func (m *MockGenAIClientWithTools) GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	// Check if this is a call after tool execution by looking at the number of messages
	if len(messages) > 3 { // More than system + user + assistant message suggests tool results
		if m.expectError {
			return "❌ I encountered an issue while trying to help you. Please try again with different parameters.", nil
		}
		return "✅ Great! I've successfully completed your request.", nil
	}
	// This is a regular call without tool results
	return "Basic response without tools", nil
}

func (m *MockGenAIClientWithTools) GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*genai.ToolCallResponse, error) {
	if m.shouldCallTools {
		// Default to scheduler if no tool name is specified
		toolName := m.toolName
		if toolName == "" {
			toolName = "scheduler"
		}

		// Return a response that includes the specified tool call
		return &genai.ToolCallResponse{
			Content: "", // Empty content when making tool calls
			ToolCalls: []genai.ToolCall{
				{
					ID:   m.toolCallID,
					Type: "function",
					Function: genai.FunctionCall{
						Name:      toolName,
						Arguments: json.RawMessage(m.toolCallArgs),
					},
				},
			},
		}, nil
	} else {
		// Return a regular response without tool calls
		return &genai.ToolCallResponse{
			Content:   "I'd be happy to help you set up a habit! Let me ask you a few questions first...",
			ToolCalls: nil,
		}, nil
	}
}
