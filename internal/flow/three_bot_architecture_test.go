// Package flow provides tests for the three-bot conversation architecture.
package flow

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestThreeBotArchitecture_IntakeFlow(t *testing.T) {
	// Create test dependencies
	stateManager := NewMockStateManager()
	mockGenAI := &MockGenAIClientWithTools{}
	mockMsgService := &MockMessagingService{}

	// Create the coordinator
	coordinator := NewConversationFlowCoordinator(stateManager, mockGenAI, mockMsgService)

	ctx := context.Background()
	participantID := "test-participant"

	// Test 1: Start conversation should initiate intake
	response, err := coordinator.StartConversation(ctx, participantID)
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}

	expectedWelcome := "Hi! I'm your micro-coach bot here to help you build a 1-minute healthy habit that fits into your day. I'll ask a few quick questions to personalize it. Is that okay?"
	if response != expectedWelcome {
		t.Errorf("Expected welcome message, got: %s", response)
	}

	// Test 2: Positive response should move to habit domain question
	response, err = coordinator.ProcessResponse(ctx, participantID, "yes")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response != "Great! Which of these four areas would you like to focus on?\n\n• Eating healthy\n• Mindful screen time\n• Physical activity\n• Mental well-being" {
		t.Errorf("Expected habit domain question, got: %s", response)
	}

	// Test 3: Select habit domain
	response, err = coordinator.ProcessResponse(ctx, participantID, "Physical activity")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response != "Great choice! Why did you pick physical activity? What makes this important to you right now?" {
		t.Errorf("Expected motivation question, got: %s", response)
	}

	// Test 4: Provide motivation
	response, err = coordinator.ProcessResponse(ctx, participantID, "I want to feel more energized")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response != "That makes total sense! What's one habit you've been meaning to build or restart?" {
		t.Errorf("Expected existing goal question, got: %s", response)
	}

	t.Log("Three-bot architecture intake flow test completed successfully")
}

func TestThreeBotArchitecture_UserProfileManagement(t *testing.T) {
	// Create test dependencies
	stateManager := NewMockStateManager()
	mockGenAI := &MockGenAIClientWithTools{}

	// Create intake bot
	intakeBot := NewIntakeBot(stateManager, mockGenAI)

	ctx := context.Background()
	participantID := "test-profile-participant"

	// Start intake
	_, err := intakeBot.StartIntake(ctx, participantID)
	if err != nil {
		t.Fatalf("StartIntake failed: %v", err)
	}

	// Verify profile was created
	profile, err := intakeBot.getUserProfile(ctx, participantID)
	if err != nil {
		t.Fatalf("Failed to get user profile: %v", err)
	}

	if profile.ParticipantID != participantID {
		t.Errorf("Expected participant ID %s, got %s", participantID, profile.ParticipantID)
	}

	if profile.IntakeComplete {
		t.Error("Expected intake to not be complete initially")
	}

	t.Log("User profile management test completed successfully")
}

func TestThreeBotArchitecture_StateTransitions(t *testing.T) {
	// Create test dependencies
	stateManager := NewMockStateManager()
	mockGenAI := &MockGenAIClientWithTools{}
	mockMsgService := &MockMessagingService{}

	// Create coordinator
	coordinator := NewConversationFlowCoordinator(stateManager, mockGenAI, mockMsgService)

	ctx := context.Background()
	participantID := "test-state-participant"

	// Start conversation
	_, err := coordinator.StartConversation(ctx, participantID)
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}

	// Check initial state
	state, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		t.Fatalf("GetCurrentState failed: %v", err)
	}

	if state != models.StateIntakeWelcome {
		t.Errorf("Expected state %s, got %s", models.StateIntakeWelcome, state)
	}

	// Process response and check state transition
	_, err = coordinator.ProcessResponse(ctx, participantID, "yes")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	state, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		t.Fatalf("GetCurrentState failed: %v", err)
	}

	if state != models.StateIntakeHabitDomain {
		t.Errorf("Expected state %s, got %s", models.StateIntakeHabitDomain, state)
	}

	t.Log("State transitions test completed successfully")
}
