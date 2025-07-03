package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestConversationFlow_Generate(t *testing.T) {
	f := &ConversationFlow{}
	ctx := context.Background()

	// Active conversation state
	out, err := f.Generate(ctx, models.Prompt{State: models.StateConversationActive})
	if err != nil {
		t.Fatalf("conversation active: unexpected error: %v", err)
	}
	if !strings.Contains(out, "ready to chat") {
		t.Errorf("conversation active: expected chat message, got %q", out)
	}

	// Empty state (defaults to active)
	out, err = f.Generate(ctx, models.Prompt{State: ""})
	if err != nil {
		t.Fatalf("empty state: unexpected error: %v", err)
	}
	if !strings.Contains(out, "ready to chat") {
		t.Errorf("empty state: expected chat message, got %q", out)
	}

	// Unknown state
	_, err = f.Generate(ctx, models.Prompt{State: "UNKNOWN"})
	if err == nil {
		t.Error("expected error for unknown state, got nil")
	}
}

func TestConversationFlow_LoadSystemPrompt(t *testing.T) {
	// Test with non-existent file
	f := &ConversationFlow{systemPromptFile: "/non/existent/file.txt"}
	err := f.LoadSystemPrompt()
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	// Test with empty file path
	f = &ConversationFlow{systemPromptFile: ""}
	err = f.LoadSystemPrompt()
	if err == nil {
		t.Error("expected error for empty file path, got nil")
	}
}

func TestGetSystemPromptPath(t *testing.T) {
	path := GetSystemPromptPath()
	if !strings.Contains(path, "prompts") {
		t.Errorf("expected path to contain 'prompts', got %q", path)
	}
	if !strings.Contains(path, "conversation_system.txt") {
		t.Errorf("expected path to contain 'conversation_system.txt', got %q", path)
	}
}
