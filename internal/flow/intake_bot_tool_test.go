package flow

import (
	"testing"
)

func TestIntakeBotTool_GetToolDefinition(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("expected tool type 'function', got %q", definition.Type)
	}

	if definition.Function.Name != "conduct_intake" {
		t.Errorf("expected function name 'conduct_intake', got %q", definition.Function.Name)
	}

	if definition.Function.Description.Value == "" {
		t.Error("expected function description to be non-empty")
	}
}
