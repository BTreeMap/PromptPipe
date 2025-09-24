package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestPromptGeneratorModule_NoToolCalls_ReturnsContent(t *testing.T) {
	// Setup
	stateManager := NewMockStateManager()
	gen := &MockGenAIClientWithTools{shouldCallTools: false}
	msg := &MockMessagingService{}

	// Shared tools
	stt := NewStateTransitionTool(stateManager, &MockTimer{})
	pgt := NewPromptGeneratorTool(stateManager, gen, msg, "test-prompt-file.txt")

	// Module under test
	mod := NewPromptGeneratorModule(stateManager, gen, msg, stt, nil, nil, pgt)

	// Prepare minimal args and history
	ctx := context.Background()
	participantID := "p1"

	out, err := mod.ExecutePromptGeneratorWithHistoryAndConversation(ctx, participantID, map[string]interface{}{"user_response": "hi"}, nil, &ConversationHistory{Messages: []ConversationMessage{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty content")
	}
}

func TestPromptGeneratorModule_GenerateHabitPrompt_ToolCall(t *testing.T) {
	// Setup
	stateManager := NewMockStateManager()
	gen := &MockGenAIClientWithTools{shouldCallTools: true, toolName: "generate_habit_prompt"}
	msg := &MockMessagingService{}

	// Provide tool call args for generate_habit_prompt
	args := map[string]interface{}{"delivery_mode": "immediate"}
	raw, _ := json.Marshal(args)
	gen.toolCallArgs = string(raw)

	// Create a minimal valid profile so the tool can generate
	ctx := context.Background()
	participantID := "p2"
	prof := &UserProfile{PreferredTime: "morning", PromptAnchor: "after coffee"}
	b, _ := json.Marshal(prof)
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(b))

	// Shared tools
	stt := NewStateTransitionTool(stateManager, &MockTimer{})
	pgt := NewPromptGeneratorTool(stateManager, gen, msg, "test-prompt-file.txt")

	mod := NewPromptGeneratorModule(stateManager, gen, msg, stt, nil, nil, pgt)

	out, err := mod.ExecutePromptGeneratorWithHistoryAndConversation(ctx, participantID, map[string]interface{}{"user_response": "please make a prompt"}, nil, &ConversationHistory{Messages: []ConversationMessage{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty content from generate_habit_prompt path")
	}
	// Verify last prompt stored by tool execution
	last, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt)
	if last == "" {
		t.Fatalf("expected last habit prompt stored")
	}
}
