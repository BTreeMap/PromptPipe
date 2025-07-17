package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestOneMinuteInterventionTool_GetToolDefinition(t *testing.T) {
	stateManager := &MockStateManager{}
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewOneMinuteInterventionTool(stateManager, genaiClient, msgService)

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("Expected tool type 'function', got %s", definition.Type)
	}

	if definition.Function.Name != "initiate_intervention" {
		t.Errorf("Expected function name 'initiate_intervention', got %s", definition.Function.Name)
	}

	if definition.Function.Description.Value == "" {
		t.Error("Expected function description to be set")
	}
}

func TestOneMinuteInterventionTool_ExecuteOneMinuteIntervention(t *testing.T) {
	stateManager := &MockStateManager{}
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewOneMinuteInterventionTool(stateManager, genaiClient, msgService)

	ctx := context.Background()
	ctx = context.WithValue(ctx, phoneNumberContextKey, "+1234567890") // Add phone number to context
	participantID := "test-participant"

	params := models.OneMinuteInterventionToolParams{
		InterventionFocus:    "breathing exercise",
		PersonalizationNotes: "User mentioned feeling stressed about work",
	}

	result, err := tool.ExecuteOneMinuteIntervention(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteOneMinuteIntervention failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if result.Message == "" {
		t.Error("Expected non-empty success message")
	}

	// Check that intervention data was stored
	interventionData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "current_intervention")
	if interventionData == "" {
		t.Error("Expected intervention data to be stored")
	}

	// Check that conversation history was updated
	historyData, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory)
	if historyData == "" {
		t.Error("Expected conversation history to be updated")
	}
}

func TestOneMinuteInterventionTool_FlexibleParameters(t *testing.T) {
	stateManager := &MockStateManager{}
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewOneMinuteInterventionTool(stateManager, genaiClient, msgService)

	ctx := context.Background()
	ctx = context.WithValue(ctx, phoneNumberContextKey, "+1234567890") // Add phone number to context
	participantID := "test-participant"

	// Test with minimal parameters
	params := models.OneMinuteInterventionToolParams{
		InterventionFocus: "gratitude practice",
	}

	result, err := tool.ExecuteOneMinuteIntervention(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteOneMinuteIntervention with minimal params failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true with minimal params, got %v", result.Success)
	}

	// Test with no parameters (should still work)
	emptyParams := models.OneMinuteInterventionToolParams{}
	result, err = tool.ExecuteOneMinuteIntervention(ctx, participantID, emptyParams)
	if err != nil {
		t.Fatalf("ExecuteOneMinuteIntervention with empty params failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true with empty params, got %v", result.Success)
	}
}

func TestOneMinuteInterventionTool_BuildInterventionSystemPrompt(t *testing.T) {
	stateManager := &MockStateManager{}
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewOneMinuteInterventionTool(stateManager, genaiClient, msgService)

	// Test with intervention focus
	params := models.OneMinuteInterventionToolParams{
		InterventionFocus:    "stress relief",
		PersonalizationNotes: "User mentioned tight deadline pressure",
	}

	prompt := tool.buildInterventionSystemPrompt(params)

	if prompt == "" {
		t.Error("Expected non-empty system prompt")
	}

	// Check that intervention focus is included
	if !contains(prompt, "stress relief") {
		t.Error("Expected intervention focus to be included in system prompt")
	}

	// Check that personalization notes are included
	if !contains(prompt, "tight deadline pressure") {
		t.Error("Expected personalization notes to be included in system prompt")
	}

	// Check that intervention guidelines are present
	if !contains(prompt, "INTERVENTION GUIDELINES") {
		t.Error("Expected intervention guidelines to be present in system prompt")
	}
}

// Helper function to check if a string contains a substring
func contains(str, substr string) bool {
	return strings.Contains(str, substr)
}
