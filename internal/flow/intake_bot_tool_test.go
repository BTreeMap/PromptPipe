package flow

import (
	"context"
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

func TestIntakeBotTool_HandleGoalAreaStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	// Test physical activity recognition
	response, newState, err := tool.handleGoalAreaStage(ctx, participantID, "I want to exercise more", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.TargetBehavior != "physical activity" {
		t.Errorf("expected target behavior 'physical activity', got %q", profile.TargetBehavior)
	}

	if newState != IntakeStateMotivation {
		t.Errorf("expected state to transition to %s, got %s", IntakeStateMotivation, newState)
	}

	if !contains(response, "Why does this matter") {
		t.Errorf("expected motivation question, got: %s", response)
	}

	// Test custom response
	profile2 := &UserProfile{}
	response, newState, err = tool.handleGoalAreaStage(ctx, participantID, "Better sleep habits", profile2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile2.TargetBehavior != "Better sleep habits" {
		t.Errorf("expected target behavior 'Better sleep habits', got %q", profile2.TargetBehavior)
	}
}
