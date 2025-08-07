package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestStateTransitionTool_GetToolDefinition(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("expected tool type 'function', got %q", definition.Type)
	}

	if definition.Function.Name != "transition_state" {
		t.Errorf("expected function name 'transition_state', got %q", definition.Function.Name)
	}

	if definition.Function.Description.Value == "" {
		t.Error("expected function description to be non-empty")
	}
}

func TestStateTransitionTool_ExecuteStateTransition_Immediate(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	ctx := context.Background()
	participantID := "test-participant"

	args := map[string]interface{}{
		"target_state": "INTAKE",
		"reason":       "User needs profile setup",
	}

	response, err := tool.ExecuteStateTransition(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check that state was set
	stateStr, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState)
	if err != nil {
		t.Fatalf("failed to get conversation state: %v", err)
	}

	if stateStr != string(models.StateIntake) {
		t.Errorf("expected state 'INTAKE', got %q", stateStr)
	}
}

func TestStateTransitionTool_ExecuteStateTransition_Delayed(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	ctx := context.Background()
	participantID := "test-participant"

	args := map[string]interface{}{
		"target_state":  "FEEDBACK",
		"delay_minutes": 5.0,
		"reason":        "Schedule feedback collection",
	}

	response, err := tool.ExecuteStateTransition(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check that timer was scheduled
	if len(timer.scheduledCalls) != 1 {
		t.Errorf("expected 1 scheduled function, got %d", len(timer.scheduledCalls))
	}

	// Check that timer ID was stored
	timerID, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyStateTransitionTimerID)
	if err != nil {
		t.Fatalf("failed to get timer ID: %v", err)
	}

	if timerID == "" {
		t.Error("expected timer ID to be stored")
	}
}

func TestStateTransitionTool_ExecuteStateTransition_InvalidState(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	ctx := context.Background()
	participantID := "test-participant"

	args := map[string]interface{}{
		"target_state": "INVALID_STATE",
	}

	_, err := tool.ExecuteStateTransition(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for invalid state")
	}

	if !strings.Contains(err.Error(), "invalid target_state") {
		t.Errorf("expected invalid target_state error, got: %v", err)
	}
}

func TestStateTransitionTool_ExecuteStateTransition_MissingTargetState(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	ctx := context.Background()
	participantID := "test-participant"

	args := map[string]interface{}{
		"reason": "Test transition",
	}

	_, err := tool.ExecuteStateTransition(ctx, participantID, args)
	if err == nil {
		t.Error("expected error for missing target_state")
	}

	if !strings.Contains(err.Error(), "target_state is required") {
		t.Errorf("expected target_state required error, got: %v", err)
	}
}

func TestStateTransitionTool_CancelPendingTransition(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := &MockTimer{}
	tool := NewStateTransitionTool(stateManager, timer)

	ctx := context.Background()
	participantID := "test-participant"

	// Schedule a delayed transition first
	args := map[string]interface{}{
		"target_state":  "FEEDBACK",
		"delay_minutes": 5.0,
	}

	_, err := tool.ExecuteStateTransition(ctx, participantID, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel the pending transition
	err = tool.CancelPendingTransition(ctx, participantID)
	if err != nil {
		t.Fatalf("unexpected error cancelling transition: %v", err)
	}

	// Check that timer ID was cleared
	timerID, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyStateTransitionTimerID)
	if err != nil {
		t.Fatalf("failed to get timer ID: %v", err)
	}

	if timerID != "" {
		t.Errorf("expected timer ID to be cleared, got %q", timerID)
	}
}
