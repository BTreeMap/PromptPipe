package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestPromptGeneratorTool_GetToolDefinition(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("expected tool type 'function', got %q", definition.Type)
	}

	if definition.Function.Name != "generate_habit_prompt" {
		t.Errorf("expected function name 'generate_habit_prompt', got %q", definition.Function.Name)
	}

	if definition.Function.Description.Value == "" {
		t.Error("expected function description to be non-empty")
	}
}

func TestPromptGeneratorTool_ExecutePromptGenerator(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// Set up user profile
	profile := &UserProfile{
		TargetBehavior:    "physical activity",
		MotivationalFrame: "feel more energized",
		PreferredTime:     "morning",
		PromptAnchor:      "after coffee",
		AdditionalInfo:    "",
		CreatedAt:         time.Now().Add(-24 * time.Hour),
		UpdatedAt:         time.Now().Add(-24 * time.Hour),
	}
	profileJSON, _ := json.Marshal(profile)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Configure mock to return a habit prompt
	// The mock client will generate a response using GenerateWithMessages

	args := map[string]interface{}{
		"delivery_mode": "immediate",
	}

	response, err := tool.ExecutePromptGenerator(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// The response should contain the follow-up question
	if !contains(response, "Let me know when you've tried it") {
		t.Errorf("expected response to contain follow-up question, got: %s", response)
	}

	// Check that last prompt was stored
	storedPrompt, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt)
	if err != nil {
		t.Fatalf("expected last prompt to be stored: %v", err)
	}

	if storedPrompt == "" {
		t.Error("expected stored prompt to be non-empty")
	}
}

func TestPromptGeneratorTool_ExecutePromptGenerator_IncompleteProfile(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// Set up incomplete profile (missing required fields)
	profile := &UserProfile{
		TargetBehavior: "physical activity",
		// Missing other required fields
	}
	profileJSON, _ := json.Marshal(profile)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	args := map[string]interface{}{
		"delivery_mode": "immediate",
	}

	_, err := tool.ExecutePromptGenerator(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for incomplete profile")
	}

	if !contains(err.Error(), "profile incomplete") {
		t.Errorf("expected profile incomplete error, got: %v", err)
	}
}

func TestPromptGeneratorTool_ExecutePromptGenerator_NoProfile(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// No profile set up

	args := map[string]interface{}{
		"delivery_mode": "immediate",
	}

	_, err := tool.ExecutePromptGenerator(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for missing profile")
	}

	if !contains(err.Error(), "profile not found") {
		t.Errorf("expected profile not found error, got: %v", err)
	}
}

func TestPromptGeneratorTool_ValidateProfile(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	// Test complete profile
	completeProfile := &UserProfile{
		TargetBehavior:    "physical activity",
		MotivationalFrame: "feel more energized",
		PreferredTime:     "morning",
		PromptAnchor:      "after coffee",
	}

	err := tool.validateProfile(completeProfile)
	if err != nil {
		t.Errorf("expected complete profile to be valid, got error: %v", err)
	}

	// Test incomplete profiles
	testCases := []struct {
		name    string
		profile *UserProfile
		wantErr string
	}{
		{
			name:    "missing target behavior",
			profile: &UserProfile{MotivationalFrame: "test", PreferredTime: "test", PromptAnchor: "test"},
			wantErr: "target behavior is required",
		},
		{
			name:    "missing motivational frame",
			profile: &UserProfile{TargetBehavior: "test", PreferredTime: "test", PromptAnchor: "test"},
			wantErr: "motivational frame is required",
		},
		{
			name:    "missing preferred time",
			profile: &UserProfile{TargetBehavior: "test", MotivationalFrame: "test", PromptAnchor: "test"},
			wantErr: "preferred time is required",
		},
		{
			name:    "missing prompt anchor",
			profile: &UserProfile{TargetBehavior: "test", MotivationalFrame: "test", PreferredTime: "test"},
			wantErr: "prompt anchor is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tool.validateProfile(tc.profile)
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestPromptGeneratorTool_BuildPromptGeneratorSystemPrompt(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	// Mock the system prompt instead of loading from file
	tool.systemPrompt = `You are a micro-coach bot that creates personalized 1-minute habit prompts using the MAP framework (Motivation × Ability × Prompt).

## Core Requirements
- Keep habit suggestions under 30 words
- Focus on 1-minute actions that feel doable
- Connect to user's personal motivation
- Use MAP framework principles
- Always ask "Would that feel doable?" after suggestions

## Personalization Context
Use the user's profile and feedback to create targeted suggestions that feel relevant and achievable.`

	profile := &UserProfile{
		SuccessCount: 3,
		LastBarrier:  "lack of time",
		LastTweak:    "prefer evening time",
	}

	systemPrompt := tool.buildPromptGeneratorSystemPrompt(profile, "immediate", "user seems stressed today")

	// Check that core requirements are included
	if !contains(systemPrompt, "micro-coach bot") {
		t.Error("expected system prompt to identify as micro-coach bot")
	}

	if !contains(systemPrompt, "MAP framework") {
		t.Error("expected system prompt to mention MAP framework")
	}

	if !contains(systemPrompt, "30 words") {
		t.Error("expected system prompt to mention word limit")
	}

	// Check that delivery mode context is included
	if !contains(systemPrompt, "immediate") {
		t.Error("expected system prompt to include delivery mode context")
	}

	// Check that personalization notes are included
	if !contains(systemPrompt, "user seems stressed today") {
		t.Error("expected system prompt to include personalization notes")
	}

	// Check that success tracking is included
	if !contains(systemPrompt, "3 habit prompts") {
		t.Error("expected system prompt to include success count")
	}

	// Check that barrier context is included
	if !contains(systemPrompt, "lack of time") {
		t.Error("expected system prompt to include barrier context")
	}

	// Check that modification context is included
	if !contains(systemPrompt, "prefer evening time") {
		t.Error("expected system prompt to include modification context")
	}
}

func TestPromptGeneratorTool_ExecutePromptGenerator_InvalidArgs(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	msgService := &MockMessagingService{}
	tool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, "test-prompt-file.txt")

	ctx := context.Background()
	participantID := "test-participant"

	// Test with missing required arguments
	args := map[string]interface{}{
		// missing delivery_mode
		"personalization_notes": "test notes",
	}

	_, err := tool.ExecutePromptGenerator(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for missing required arguments")
	}
}
