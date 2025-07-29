package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestFeedbackTrackerTool_GetToolDefinition(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("expected tool type 'function', got %q", definition.Type)
	}

	if definition.Function.Name != "track_feedback" {
		t.Errorf("expected function name 'track_feedback', got %q", definition.Function.Name)
	}

	if definition.Function.Description.Value == "" {
		t.Error("expected function description to be non-empty")
	}

	params := definition.Function.Parameters
	if params["type"] != "object" {
		t.Errorf("expected parameters type 'object', got %v", params["type"])
	}

	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// Check required properties
	requiredProps := []string{"user_response", "completion_status"}
	for _, prop := range requiredProps {
		if _, exists := properties[prop]; !exists {
			t.Errorf("expected property %q to exist", prop)
		}
	}

	// Check required array
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a string array")
	}
	if len(required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(required))
	}
}

func TestFeedbackTrackerTool_ExecuteFeedbackTracker_Completed(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	ctx := context.Background()
	participantID := "test-participant"

	// Set up existing profile
	existingProfile := &UserProfile{
		TargetBehavior:    "physical activity",
		MotivationalFrame: "feel more energized",
		PreferredTime:     "morning",
		PromptAnchor:      "after coffee",
		SuccessCount:      0,
		TotalPrompts:      0,
		CreatedAt:         time.Now().Add(-24 * time.Hour),
		UpdatedAt:         time.Now().Add(-24 * time.Hour),
	}
	profileJSON, _ := json.Marshal(existingProfile)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Set up last prompt
	lastPrompt := "After your coffee, try doing 10 jumping jacks — it helps you feel more energized. Would that feel doable?"
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt, lastPrompt)

	args := map[string]interface{}{
		"user_response":     "Yes, I did it! It was great!",
		"completion_status": "completed",
	}

	response, err := tool.ExecuteFeedbackTracker(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check that profile was updated
	updatedProfileJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		t.Fatalf("failed to get updated profile: %v", err)
	}

	var updatedProfile UserProfile
	if err := json.Unmarshal([]byte(updatedProfileJSON), &updatedProfile); err != nil {
		t.Fatalf("failed to unmarshal updated profile: %v", err)
	}

	if updatedProfile.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", updatedProfile.SuccessCount)
	}

	if updatedProfile.TotalPrompts != 1 {
		t.Errorf("expected total prompts 1, got %d", updatedProfile.TotalPrompts)
	}

	if updatedProfile.LastSuccessfulPrompt != lastPrompt {
		t.Errorf("expected last successful prompt to be set")
	}
}

func TestFeedbackTrackerTool_ExecuteFeedbackTracker_Skipped(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	ctx := context.Background()
	participantID := "test-participant"

	args := map[string]interface{}{
		"user_response":     "I didn't have time today",
		"completion_status": "skipped",
		"barrier_reason":    "lack of time",
	}

	response, err := tool.ExecuteFeedbackTracker(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check that profile was updated
	updatedProfileJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		t.Fatalf("failed to get updated profile: %v", err)
	}

	var updatedProfile UserProfile
	if err := json.Unmarshal([]byte(updatedProfileJSON), &updatedProfile); err != nil {
		t.Fatalf("failed to unmarshal updated profile: %v", err)
	}

	if updatedProfile.SuccessCount != 0 {
		t.Errorf("expected success count 0, got %d", updatedProfile.SuccessCount)
	}

	if updatedProfile.TotalPrompts != 1 {
		t.Errorf("expected total prompts 1, got %d", updatedProfile.TotalPrompts)
	}

	if updatedProfile.LastBarrier != "lack of time" {
		t.Errorf("expected last barrier to be 'lack of time', got %q", updatedProfile.LastBarrier)
	}
}

func TestFeedbackTrackerTool_ExecuteFeedbackTracker_Modified(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	ctx := context.Background()
	participantID := "test-participant"

	// Set up existing profile
	existingProfile := &UserProfile{
		PreferredTime: "morning",
		PromptAnchor:  "after coffee",
		CreatedAt:     time.Now().Add(-24 * time.Hour),
		UpdatedAt:     time.Now().Add(-24 * time.Hour),
	}
	profileJSON, _ := json.Marshal(existingProfile)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	args := map[string]interface{}{
		"user_response":          "Can we do this in the evening instead?",
		"completion_status":      "modified",
		"suggested_modification": "change time to evening",
	}

	response, err := tool.ExecuteFeedbackTracker(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check that profile was updated
	updatedProfileJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		t.Fatalf("failed to get updated profile: %v", err)
	}

	var updatedProfile UserProfile
	if err := json.Unmarshal([]byte(updatedProfileJSON), &updatedProfile); err != nil {
		t.Fatalf("failed to unmarshal updated profile: %v", err)
	}

	if updatedProfile.LastTweak != "change time to evening" {
		t.Errorf("expected last tweak to be set, got %q", updatedProfile.LastTweak)
	}

	if updatedProfile.PreferredTime != "evening" {
		t.Errorf("expected preferred time to be updated to 'evening', got %q", updatedProfile.PreferredTime)
	}
}

func TestFeedbackTrackerTool_ExecuteFeedbackTracker_InvalidArgs(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	ctx := context.Background()
	participantID := "test-participant"

	// Test with missing required arguments
	args := map[string]interface{}{
		"user_response": "test response",
		// missing completion_status
	}

	_, err := tool.ExecuteFeedbackTracker(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for missing required arguments")
	}
}

func TestFeedbackTrackerTool_ApplyProfileModifications(t *testing.T) {
	stateManager := NewMockStateManager()
	genaiClient := &MockGenAIClientWithTools{}
	tool := NewFeedbackTrackerTool(stateManager, genaiClient)

	profile := &UserProfile{
		PreferredTime: "morning",
		PromptAnchor:  "after coffee",
	}

	// Test time modification
	tool.applyProfileModifications(profile, "Can we do this in the evening instead?")
	if profile.PreferredTime != "evening" {
		t.Errorf("expected preferred time to be updated to 'evening', got %q", profile.PreferredTime)
	}

	// Test anchor modification
	profile.PromptAnchor = "after coffee"
	tool.applyProfileModifications(profile, "I'd prefer to do this before work meetings")
	if profile.PromptAnchor != "work breaks" {
		t.Errorf("expected prompt anchor to be updated to 'work breaks', got %q", profile.PromptAnchor)
	}
}
