package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestFeedbackTrackerTool_ScheduleFeedbackCollection(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	timer := &MockTimer{}
	msgService := &MockMessagingService{}

	// Create feedback tracker tool with timeouts
	tool := NewFeedbackTrackerToolWithTimeouts(
		stateManager,
		genaiClient,
		"test-prompt-file.txt",
		timer,
		msgService,
		"15m", // initial timeout
		"3h",  // followup delay
	)

	participantID := "test-participant"

	// Test scheduling feedback collection
	err := tool.ScheduleFeedbackCollection(ctx, participantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify state was set
	feedbackState, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err != nil {
		t.Fatalf("failed to get feedback state: %v", err)
	}
	if feedbackState != "waiting_initial" {
		t.Errorf("expected feedback state 'waiting_initial', got %q", feedbackState)
	}

	// Verify timer was scheduled
	if len(timer.scheduledCalls) != 1 {
		t.Errorf("expected 1 scheduled call, got %d", len(timer.scheduledCalls))
	}

	// Verify timer ID was stored
	timerID, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID)
	if err != nil {
		t.Fatalf("failed to get timer ID: %v", err)
	}
	if timerID == "" {
		t.Error("expected timer ID to be stored")
	}
}

func TestFeedbackTrackerTool_CancelPendingFeedback(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	timer := &MockTimer{}
	msgService := &MockMessagingService{}

	// Create feedback tracker tool with timeouts
	tool := NewFeedbackTrackerToolWithTimeouts(
		stateManager,
		genaiClient,
		"test-prompt-file.txt",
		timer,
		msgService,
		"15m", // initial timeout
		"3h",  // followup delay
	)

	participantID := "test-participant"

	// Set up some timers to cancel
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID, "timer-1")
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID, "timer-2")
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "waiting_initial")

	// Cancel pending feedback
	tool.CancelPendingFeedback(ctx, participantID)

	// Verify state was updated
	feedbackState, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err != nil {
		t.Fatalf("failed to get feedback state: %v", err)
	}
	if feedbackState != "completed" {
		t.Errorf("expected feedback state 'completed', got %q", feedbackState)
	}

	// Note: MockTimer doesn't track cancellation calls, but the method should complete without error
}

func TestConversationFlow_AutomaticFeedbackCollection(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)

	// Create mock messaging service
	msgService := &MockMessagingService{}

	// Create mock GenAI client that calls the prompt generator tool
	mockGenAI := &MockGenAIClientWithTools{
		shouldCallTools: true,
		toolCallID:      "call_123",
		toolName:        "generate_habit_prompt",
	}

	// Set up the tool call arguments for prompt generation
	promptParams := map[string]interface{}{
		"delivery_mode": "immediate",
	}
	paramsJSON, err := json.Marshal(promptParams)
	if err != nil {
		t.Fatalf("Failed to marshal prompt params: %v", err)
	}
	mockGenAI.toolCallArgs = string(paramsJSON)

	// Create conversation flow with all tools and timeouts
	flow := NewConversationFlowWithAllToolsAndTimeouts(
		stateManager,
		mockGenAI,
		"",
		msgService,
		"prompts/intake_bot_system.txt",
		"prompts/prompt_generator_system.txt",
		"prompts/feedback_tracker_system.txt",
		"15m", // feedback initial timeout
		"3h",  // feedback followup delay
	)

	participantID := "test-participant-feedback"

	// Add phone number to context
	phoneNumber := "+1234567890"
	ctx = context.WithValue(ctx, GetPhoneNumberContextKey(), phoneNumber)

	// Set up a user profile for prompt generation
	profile := &UserProfile{
		HabitDomain:       "physical activity",
		MotivationalFrame: "feel more energized",
		PreferredTime:     "morning",
		PromptAnchor:      "after coffee",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	profileJSON, _ := json.Marshal(profile)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Process a message that should trigger prompt generation
	response, err := flow.ProcessResponse(ctx, participantID, "I'd like a personalized habit suggestion")
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	// Verify that feedback collection was scheduled
	feedbackState, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err != nil {
		t.Fatalf("failed to get feedback state: %v", err)
	}
	if feedbackState != "waiting_initial" {
		t.Errorf("expected feedback state 'waiting_initial', got %q", feedbackState)
	}

	t.Logf("Automatic feedback collection test completed successfully. Response: %s", response)
}
