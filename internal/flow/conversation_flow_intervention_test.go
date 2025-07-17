package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// MockGenAIInterventionClient is a specialized mock for testing intervention tools
type MockGenAIInterventionClient struct{}

func (m *MockGenAIInterventionClient) GeneratePrompt(system, user string) (string, error) {
	return "Generated intervention message", nil
}

func (m *MockGenAIInterventionClient) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	return "Generated intervention message with context", nil
}

func (m *MockGenAIInterventionClient) GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	return "🌱 Let's take a moment for a quick breathing exercise. Take a deep breath in for 4 counts... hold for 4... and slowly release for 6. Feel your body relax as you breathe out any tension. How are you feeling right now?", nil
}

func (m *MockGenAIInterventionClient) GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*genai.ToolCallResponse, error) {
	// Return a response that includes an intervention tool call
	return &genai.ToolCallResponse{
		Content: "", // Empty content when making tool calls
		ToolCalls: []genai.ToolCall{
			{
				ID:   "call_intervention_456",
				Type: "function",
				Function: genai.FunctionCall{
					Name:      "initiate_intervention",
					Arguments: json.RawMessage(`{"intervention_focus": "stress relief", "personalization_notes": "User mentioned work stress"}`),
				},
			},
		},
	}, nil
}

func TestConversationFlow_WithInterventionTool(t *testing.T) {
	// Create mocks
	stateManager := NewMockStateManager()

	mockGenAI := &MockGenAIClientWithTools{
		shouldCallTools: true,
		toolCallID:      "call_intervention_123",
	}

	// Create intervention tool parameters for the mock
	interventionParams := models.OneMinuteInterventionToolParams{
		InterventionFocus:    "breathing exercise",
		PersonalizationNotes: "User mentioned feeling stressed",
	}
	paramsJSON, _ := json.Marshal(interventionParams)
	mockGenAI.toolCallArgs = string(paramsJSON)

	// Create intervention tool
	msgService := &MockMessagingService{}
	interventionTool := NewOneMinuteInterventionTool(stateManager, mockGenAI, msgService)

	// Create conversation flow with intervention tool
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", nil, interventionTool)

	// Test initial response processing
	ctx := context.Background()
	participantID := "test-participant-intervention"

	// First message - setup conversation without tools
	mockGenAI.shouldCallTools = false
	response, err := flow.ProcessResponse(ctx, participantID, "I'm feeling really stressed about work today")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response from conversation")
	}

	// Test the intervention tool being called
	// Create a mock that specifically returns intervention tool call
	interventionMockGenAI := &MockGenAIInterventionClient{}
	msgService2 := &MockMessagingService{}
	interventionTool2 := NewOneMinuteInterventionTool(stateManager, interventionMockGenAI, msgService2)
	flow2 := NewConversationFlowWithTools(stateManager, interventionMockGenAI, "", nil, interventionTool2)

	response, err = flow2.ProcessResponse(ctx, participantID, "I think I could use some help with stress relief right now")
	if err != nil {
		t.Fatalf("ProcessResponse with intervention tool failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response after intervention tool execution")
	}

	// Verify that intervention data was stored
	interventionData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "current_intervention")
	if interventionData == "" {
		t.Error("Expected intervention data to be stored after tool execution")
	}

	t.Logf("Intervention tool integration test completed successfully. Response: %s", response)
}

func TestConversationFlow_WithBothTools(t *testing.T) {
	// Create mocks
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	mockGenAI := &MockGenAIClientWithTools{}

	// Create both tools
	schedulerTool := NewSchedulerTool(timer, &MockMessagingService{})
	interventionTool := NewOneMinuteInterventionTool(stateManager, mockGenAI, &MockMessagingService{})

	// Create conversation flow with both tools
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", schedulerTool, interventionTool)

	ctx := context.Background()
	participantID := "test-participant-both-tools"

	// Test that both tools are available
	response, err := flow.ProcessResponse(ctx, participantID, "Hello! I want to set up habits and try an intervention")
	if err != nil {
		t.Fatalf("ProcessResponse with both tools failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	t.Logf("Both tools integration test completed successfully")
}

func TestInterventionTool_CanBeCalledDirectly(t *testing.T) {
	// Test that the intervention tool can be called directly (for scheduler integration)
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewOneMinuteInterventionTool(stateManager, genaiClient, &MockMessagingService{})

	ctx := context.Background()
	participantID := "test-direct-call"

	// Set up some conversation history first
	historyData := `{"messages": [
		{"role": "user", "content": "I've been feeling overwhelmed lately", "timestamp": "2025-07-17T10:00:00Z"},
		{"role": "assistant", "content": "I understand you're feeling overwhelmed. That's completely normal.", "timestamp": "2025-07-17T10:00:01Z"}
	]}`
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, historyData)

	// Call the intervention tool directly
	params := models.OneMinuteInterventionToolParams{
		InterventionFocus:    "mindfulness exercise",
		PersonalizationNotes: "User mentioned feeling overwhelmed",
	}

	result, err := tool.ExecuteOneMinuteIntervention(ctx, participantID, params)
	if err != nil {
		t.Fatalf("Direct intervention tool call failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	// Verify intervention was properly executed and stored
	interventionData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "current_intervention")
	if interventionData == "" {
		t.Error("Expected intervention data to be stored")
	}

	t.Logf("Direct intervention tool call test completed successfully")
}

func TestConversationFlow_InterventionToolIntegration(t *testing.T) {
	// Create mocks
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	mockGenAI := &MockGenAIClientWithTools{}

	// Create both tools
	schedulerTool := NewSchedulerTool(timer, &MockMessagingService{})
	interventionTool := NewOneMinuteInterventionTool(stateManager, mockGenAI, &MockMessagingService{})

	// Create conversation flow with both tools
	flow := NewConversationFlowWithTools(stateManager, mockGenAI, "", schedulerTool, interventionTool)

	ctx := context.Background()
	participantID := "test-participant-integration"

	// Test that both tools can be used in sequence
	response, err := flow.ProcessResponse(ctx, participantID, "Hi, I need help with my stress and also want to set a timer for breaks")
	if err != nil {
		t.Fatalf("ProcessResponse with both tools failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	// Verify that intervention was triggered
	interventionData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "current_intervention")
	if interventionData == "" {
		t.Error("Expected intervention data to be stored after tool execution")
	}

	t.Logf("Intervention tool integration test completed successfully. Response: %s", response)
}
