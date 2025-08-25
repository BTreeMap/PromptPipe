package genai

import (
	"context"
	"testing"

	"github.com/openai/openai-go"
)

// mockChatServiceThinking returns a JSON structured response
type mockChatServiceThinking struct{}

func (m *mockChatServiceThinking) Create(ctx context.Context, params openai.ChatCompletionNewParams) (openai.ChatCompletion, error) {
	return openai.ChatCompletion{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: `{"thinking": "I consider factors", "content": "Hello user"}`}}}}, nil
}

func TestGenerateThinkingWithMessages_Success(t *testing.T) {
	client := &Client{chat: &mockChatServiceThinking{}, model: "test-model", temperature: 0.1, maxCompletionTokens: 100}
	resp, err := client.GenerateThinkingWithMessages(context.Background(), []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking == "" || resp.Content == "" {
		t.Fatalf("expected both thinking and content, got %+v", resp)
	}
}

// Test fallback when JSON invalid
type mockChatServiceThinkingInvalid struct{}

func (m *mockChatServiceThinkingInvalid) Create(ctx context.Context, params openai.ChatCompletionNewParams) (openai.ChatCompletion, error) {
	return openai.ChatCompletion{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: `Not JSON`}}}}, nil
}

func TestGenerateThinkingWithMessages_Fallback(t *testing.T) {
	client := &Client{chat: &mockChatServiceThinkingInvalid{}, model: "test-model", temperature: 0.1, maxCompletionTokens: 100}
	resp, err := client.GenerateThinkingWithMessages(context.Background(), []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "" { // fallback should leave thinking empty
		t.Fatalf("expected empty thinking on fallback, got %s", resp.Thinking)
	}
	if resp.Content != "Not JSON" {
		t.Fatalf("expected raw content fallback, got %s", resp.Content)
	}
}
