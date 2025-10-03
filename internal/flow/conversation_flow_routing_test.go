package flow

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Tests that unknown/missing conversation sub-state defaults to INTAKE and that routing calls intake module.
func TestConversationFlow_DefaultsToIntake(t *testing.T) {
	sm := NewMockStateManager()
	gen := &MockGenAIClientWithTools{shouldCallTools: false}
	msg := &MockMessagingService{}

	// Build all tools, matching constructor
	timer := NewSimpleTimer()
	pgt := NewPromptGeneratorTool(sm, gen, msg, "test-prompt-file.txt")
	sched := NewSchedulerTool(timer, msg, gen, sm, pgt, 10, true)
	stt := NewStateTransitionTool(sm, timer)
	prof := NewProfileSaveTool(sm)

	intake := NewIntakeModule(sm, gen, msg, "intake.txt", stt, prof, sched, pgt)
	feedback := NewFeedbackModule(sm, gen, "feedback.txt", timer, msg, "15m", "3h", stt, prof, sched)
	f := &ConversationFlow{
		stateManager:        sm,
		genaiClient:         gen,
		intakeModule:        intake,
		feedbackModule:      feedback,
		promptGeneratorTool: pgt,
		stateTransitionTool: stt,
		profileSaveTool:     prof,
	}

	ctx := context.Background()
	participantID := "user-1"

	// Ensure no sub-state set
	_ = sm.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)
	_ = sm.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, "")

	out, err := f.ProcessResponse(ctx, participantID, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty response from intake default")
	}
	// Verify state was defaulted to INTAKE
	s, _ := sm.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState)
	if s != string(models.StateIntake) {
		t.Fatalf("expected state INTAKE, got %s", s)
	}
}

// Tests that a legacy PROMPT_GENERATOR state falls back to the intake module.
func TestConversationFlow_LegacyPromptGeneratorStateFallsBackToIntake(t *testing.T) {
	sm := NewMockStateManager()
	gen := &MockGenAIClientWithTools{shouldCallTools: false}
	msg := &MockMessagingService{}

	timer := NewSimpleTimer()
	pgt := NewPromptGeneratorTool(sm, gen, msg, "test-prompt-file.txt")
	sched := NewSchedulerTool(timer, msg, gen, sm, pgt, 10, true)
	stt := NewStateTransitionTool(sm, timer)
	prof := NewProfileSaveTool(sm)

	intake := NewIntakeModule(sm, gen, msg, "intake.txt", stt, prof, sched, pgt)
	feedback := NewFeedbackModule(sm, gen, "feedback.txt", timer, msg, "15m", "3h", stt, prof, sched)
	f := &ConversationFlow{
		stateManager:        sm,
		genaiClient:         gen,
		intakeModule:        intake,
		feedbackModule:      feedback,
		promptGeneratorTool: pgt,
		stateTransitionTool: stt,
		profileSaveTool:     prof,
	}

	ctx := context.Background()
	participantID := "user-2"
	_ = sm.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)
	_ = sm.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, "PROMPT_GENERATOR")

	out, err := f.ProcessResponse(ctx, participantID, "please generate a prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty response when falling back to intake")
	}

	// Verify the state was corrected back to INTAKE
	stateVal, _ := sm.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState)
	if stateVal != string(models.StateIntake) {
		t.Fatalf("expected fallback state INTAKE, got %s", stateVal)
	}
}
