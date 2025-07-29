package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestIntakeBotTool_GetToolDefinition(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

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

func TestIntakeBotTool_HandleWelcomeStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	// Test initial welcome
	response, newState, err := tool.handleWelcomeStage(ctx, participantID, "", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newState != IntakeStateWelcome {
		t.Errorf("expected state to remain %s, got %s", IntakeStateWelcome, newState)
	}

	if !contains(response, "micro-coach bot") {
		t.Errorf("expected welcome message to contain 'micro-coach bot', got: %s", response)
	}

	// Test positive consent
	response, newState, err = tool.handleWelcomeStage(ctx, participantID, "yes", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newState != IntakeStateGoalArea {
		t.Errorf("expected state to transition to %s, got %s", IntakeStateGoalArea, newState)
	}

	if !contains(response, "habit you've been meaning") {
		t.Errorf("expected goal area question, got: %s", response)
	}

	// Test negative consent
	response, newState, err = tool.handleWelcomeStage(ctx, participantID, "no", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newState != IntakeStateComplete {
		t.Errorf("expected state to transition to %s, got %s", IntakeStateComplete, newState)
	}
}

func TestIntakeBotTool_HandleGoalAreaStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

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

func TestIntakeBotTool_HandleMotivationStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	response, newState, err := tool.handleMotivationStage(ctx, participantID, "I want to feel more energized", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.MotivationalFrame != "I want to feel more energized" {
		t.Errorf("expected motivational frame to be set, got %q", profile.MotivationalFrame)
	}

	if newState != IntakeStatePreferredTime {
		t.Errorf("expected state to transition to %s, got %s", IntakeStatePreferredTime, newState)
	}

	if !contains(response, "When during the day") {
		t.Errorf("expected timing question, got: %s", response)
	}
}

func TestIntakeBotTool_HandlePreferredTimeStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	response, newState, err := tool.handlePreferredTimeStage(ctx, participantID, "morning", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.PreferredTime != "morning" {
		t.Errorf("expected preferred time 'morning', got %q", profile.PreferredTime)
	}

	if newState != IntakeStatePromptAnchor {
		t.Errorf("expected state to transition to %s, got %s", IntakeStatePromptAnchor, newState)
	}

	if !contains(response, "naturally fit") {
		t.Errorf("expected anchor question, got: %s", response)
	}
}

func TestIntakeBotTool_HandlePromptAnchorStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	response, newState, err := tool.handlePromptAnchorStage(ctx, participantID, "after coffee", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.PromptAnchor != "after coffee" {
		t.Errorf("expected prompt anchor 'after coffee', got %q", profile.PromptAnchor)
	}

	if newState != IntakeStateAdditionalInfo {
		t.Errorf("expected state to transition to %s, got %s", IntakeStateAdditionalInfo, newState)
	}

	if !contains(response, "anything else") {
		t.Errorf("expected additional info question, got: %s", response)
	}
}

func TestIntakeBotTool_HandleAdditionalInfoStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	// Test with additional info
	response, newState, err := tool.handleAdditionalInfoStage(ctx, participantID, "I have limited mobility", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.AdditionalInfo != "I have limited mobility" {
		t.Errorf("expected additional info to be set, got %q", profile.AdditionalInfo)
	}

	if newState != IntakeStateComplete {
		t.Errorf("expected state to transition to %s, got %s", IntakeStateComplete, newState)
	}

	if !contains(response, "try a 1-minute version") {
		t.Errorf("expected completion question, got: %s", response)
	}

	// Test with "no" response
	profile2 := &UserProfile{}
	response, newState, err = tool.handleAdditionalInfoStage(ctx, participantID, "no", profile2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile2.AdditionalInfo != "" {
		t.Errorf("expected additional info to remain empty, got %q", profile2.AdditionalInfo)
	}
}

func TestIntakeBotTool_HandleCompleteStage(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"
	profile := &UserProfile{}

	// Test positive response
	response, newState, err := tool.handleCompleteStage(ctx, participantID, "yes", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newState != IntakeStateComplete {
		t.Errorf("expected state to remain %s, got %s", IntakeStateComplete, newState)
	}

	if !contains(response, "personalized 1-minute habit") {
		t.Errorf("expected immediate habit generation message, got: %s", response)
	}

	// Test negative response
	response, newState, err = tool.handleCompleteStage(ctx, participantID, "no", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(response, "preferred time") {
		t.Errorf("expected deferred message, got: %s", response)
	}
}

func TestIntakeBotTool_ExecuteIntakeBot(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// Test full intake flow
	args := map[string]interface{}{
		"conversation_stage": "welcome",
		"user_response":      "",
	}

	response, err := tool.ExecuteIntakeBot(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(response, "micro-coach bot") {
		t.Errorf("expected welcome message, got: %s", response)
	}

	// Check that profile was created and saved
	profileJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		t.Fatalf("expected profile to be saved: %v", err)
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		t.Fatalf("failed to unmarshal profile: %v", err)
	}

	if profile.CreatedAt.IsZero() {
		t.Error("expected profile to have creation time set")
	}
}

func TestIntakeBotTool_ExecuteIntakeBot_InvalidArgs(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewIntakeBotTool(stateManager, genaiClient, msgService, "prompts/intake_bot_system.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// Test with missing required arguments
	args := map[string]interface{}{
		// missing conversation_stage
		"user_response": "test response",
	}

	_, err := tool.ExecuteIntakeBot(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for missing required arguments")
	}
}
