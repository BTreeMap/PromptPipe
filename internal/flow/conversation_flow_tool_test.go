package flow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
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

func TestConversationFlow_WithSchedulerTool(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)

	// Create mock timer and messaging service
	timer := &MockTimer{}
	msgService := &MockMessagingService{}

	// Create scheduler tool
	schedulerTool := NewSchedulerTool(timer, msgService)

	// Create mock GenAI client
	mockGenAI := &MockGenAIClientWithTools{
		shouldCallTools: false, // Start without tool calls
	}

	// Create conversation flow with scheduler tool
	flow := NewConversationFlowWithScheduler(stateManager, mockGenAI, "", schedulerTool)

	participantID := "test-participant-tools"

	// Add phone number to context for scheduler tool to work
	phoneNumber := "+1234567890"
	ctx = context.WithValue(ctx, GetPhoneNumberContextKey(), phoneNumber)

	// Test basic conversation without tools
	response, err := flow.ProcessResponse(ctx, participantID, "Hi, I want to start a new habit")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	// Now test with tool calling
	mockGenAI.shouldCallTools = true
	mockGenAI.toolCallID = "call_123"

	// Create valid scheduler parameters
	schedulerParams := models.SchedulerToolParams{
		Type:               models.SchedulerTypeFixed,
		FixedTime:          "09:00",
		Timezone:           "America/Toronto",
		PromptSystemPrompt: "You are a daily habit coach. Provide encouraging reminders for the user's meditation practice.",
		PromptUserPrompt:   "Time for your 5-minute morning meditation! Find a quiet spot and focus on your breathing.",
		HabitDescription:   "5-minute morning meditation",
	}

	paramsJSON, err := json.Marshal(schedulerParams)
	if err != nil {
		t.Fatalf("Failed to marshal scheduler params: %v", err)
	}
	mockGenAI.toolCallArgs = string(paramsJSON)

	// Process a message that should trigger tool use
	response, err = flow.ProcessResponse(ctx, participantID, "Yes, please schedule daily reminders at 9 AM for my meditation habit")
	if err != nil {
		t.Fatalf("ProcessResponse with tools failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response after tool execution")
	}

	// Verify the response indicates success
	if !strings.Contains(response, "✅") {
		t.Error("Expected success message in response")
	}

	// Verify that a timer was scheduled
	if len(timer.scheduledCalls) != 1 {
		t.Errorf("Expected 1 scheduled call, got %d", len(timer.scheduledCalls))
	} else {
		expectedSchedule := &models.Schedule{
			Hour:     intPtr(9),
			Minute:   intPtr(0),
			Timezone: "America/Toronto", // Match the timezone from the test data
		}
		actualSchedule := timer.scheduledCalls[0].Schedule
		if !scheduleEquals(actualSchedule, expectedSchedule) {
			t.Errorf("Expected schedule %v, got %v", expectedSchedule, actualSchedule)
			if expectedSchedule.Hour != nil && actualSchedule.Hour != nil {
				t.Errorf("Expected Hour value: %d, Actual Hour value: %d", *expectedSchedule.Hour, *actualSchedule.Hour)
			}
			if expectedSchedule.Minute != nil && actualSchedule.Minute != nil {
				t.Errorf("Expected Minute value: %d, Actual Minute value: %d", *expectedSchedule.Minute, *actualSchedule.Minute)
			}
		}
	}
}

func TestConversationFlow_WithoutSchedulerTool(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)

	// Create mock GenAI client
	mockGenAI := &MockGenAIClientWithTools{
		shouldCallTools: false,
	}

	// Create conversation flow WITHOUT scheduler tool
	flow := NewConversationFlow(stateManager, mockGenAI, "")

	participantID := "test-participant-no-tools"

	// Test conversation fallback to basic mode
	response, err := flow.ProcessResponse(ctx, participantID, "Hi, I want to start a new habit")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	// Should use basic generation without tools
	if !strings.Contains(response, "Basic response without tools") {
		t.Error("Expected basic response without tools")
	}
}

func TestConversationFlow_ToolExecutionError(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)

	// Create mock timer and messaging service
	timer := &MockTimer{}
	msgService := &MockMessagingService{}

	// Create scheduler tool
	schedulerTool := NewSchedulerTool(timer, msgService)

	// Create mock GenAI client that will make invalid tool calls
	mockGenAI := &MockGenAIClientWithTools{
		shouldCallTools: true,
		toolCallID:      "call_invalid",
		toolCallArgs:    `{"invalid": "json"}`, // Invalid JSON that will fail parsing
		expectError:     true,                  // This test expects error responses
	}

	// Create conversation flow with scheduler tool
	flow := NewConversationFlowWithScheduler(stateManager, mockGenAI, "", schedulerTool)

	participantID := "test-participant-error"

	// Add phone number to context for consistency
	phoneNumber := "+1234567890"
	ctx = context.WithValue(ctx, GetPhoneNumberContextKey(), phoneNumber)

	// Process a message that should trigger tool use but fail
	response, err := flow.ProcessResponse(ctx, participantID, "Please schedule my reminders")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	t.Logf("Response: %s", response)

	// Should get an error response
	if !strings.Contains(response, "❌") {
		t.Errorf("Expected error message in response, got: %s", response)
	}

	// Should not have scheduled anything
	if len(timer.scheduledCalls) != 0 {
		t.Errorf("Expected 0 scheduled calls due to error, got %d", len(timer.scheduledCalls))
	}
}

func TestConversationFlow_UnknownTool(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)

	// Create mock timer and messaging service
	timer := &MockTimer{}
	msgService := &MockMessagingService{}

	// Create scheduler tool
	schedulerTool := NewSchedulerTool(timer, msgService)

	// Create a custom mock GenAI client that will call an unknown tool
	mockGenAI := &MockGenAIClientWithUnknownTool{}

	// Create conversation flow with scheduler tool
	flow := NewConversationFlowWithScheduler(stateManager, mockGenAI, "", schedulerTool)

	participantID := "test-participant-unknown"

	// Add phone number to context for consistency
	phoneNumber := "+1234567890"
	ctx = context.WithValue(ctx, GetPhoneNumberContextKey(), phoneNumber)

	// Process a message that should trigger unknown tool use
	response, err := flow.ProcessResponse(ctx, participantID, "Please use an unknown tool")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Should get an error response about unknown tool
	if !strings.Contains(response, "❌") || !strings.Contains(response, "unknown_tool") {
		t.Errorf("Expected error message about unknown tool, got: %s", response)
	}
}

// Separate mock for testing unknown tool calls
type MockGenAIClientWithUnknownTool struct{}

func (m *MockGenAIClientWithUnknownTool) GeneratePrompt(system, user string) (string, error) {
	return "Basic response", nil
}

func (m *MockGenAIClientWithUnknownTool) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	return "Basic response", nil
}

func (m *MockGenAIClientWithUnknownTool) GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	// Check if this is a call after tool execution (post-error response)
	if len(messages) > 3 { // More than system + user + assistant message suggests tool results
		return "❌ Sorry, I encountered an issue with the unknown_tool. Let me help you with something else.", nil
	}
	return "Basic response without tools", nil
}

func (m *MockGenAIClientWithUnknownTool) GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*genai.ToolCallResponse, error) {
	return &genai.ToolCallResponse{
		Content: "",
		ToolCalls: []genai.ToolCall{
			{
				ID:   "call_unknown",
				Type: "function",
				Function: genai.FunctionCall{
					Name:      "unknown_tool",
					Arguments: json.RawMessage(`{}`),
				},
			},
		},
	}, nil
}
