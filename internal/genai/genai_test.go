package genai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

// mockChatService implements chatService for testing.
type mockChatService struct {
	resp openai.ChatCompletion
	err  error
}

func (m *mockChatService) Create(ctx context.Context, params openai.ChatCompletionNewParams) (openai.ChatCompletion, error) {
	return m.resp, m.err
}

func TestGeneratePrompt_Success(t *testing.T) {
	// Prepare a mock response with one choice
	mockResp := openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: "Hello World"}},
		},
	}
	client := &Client{chat: &mockChatService{resp: mockResp}}
	out, err := client.GeneratePrompt("system prompt", "user prompt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", out)
	}
}

func TestGeneratePrompt_ServiceError(t *testing.T) {
	client := &Client{chat: &mockChatService{err: errors.New("service failure")}}
	_, err := client.GeneratePrompt("sys", "usr")
	if err == nil || !strings.Contains(err.Error(), "service failure") {
		t.Errorf("expected service failure error, got %v", err)
	}
}

func TestGeneratePrompt_NoChoices(t *testing.T) {
	// Empty choices slice
	mockResp := openai.ChatCompletion{Choices: []openai.ChatCompletionChoice{}}
	client := &Client{chat: &mockChatService{resp: mockResp}}
	_, err := client.GeneratePrompt("sys", "usr")
	if err != ErrNoChoicesReturned {
		t.Errorf("expected no choices returned error, got %v", err)
	}
}

func TestNewClient_NoKey(t *testing.T) {
	_, err := NewClient()
	if err == nil {
		t.Error("expected error when API key not provided, got nil")
	}
}

func TestNewClient_WithKey(t *testing.T) {
	key := "test-key"
	cli, err := NewClient(WithAPIKey(key))
	if err != nil {
		t.Fatalf("expected no error with API key, got %v", err)
	}
	if cli == nil {
		t.Error("expected client instance, got nil")
	}
}
