// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
)

// chatService defines minimal interface for chat completions.
type chatService interface {
	Create(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Client wraps the OpenAI ChatCompletion service for generating prompts.
type Client struct {
	chat chatService
}

// NewClient initializes a new GenAI client using the OPENAI_API_KEY environment variable.
func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	cli := openai.NewClient(apiKey)
	return &Client{chat: cli.Chat}, nil
}

// GeneratePrompt generates a response based on the provided system and user prompts.
func (c *Client) GeneratePrompt(systemPrompt, userPrompt string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model:    openai.ChatModelGPT4oMini,
		Messages: []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: systemPrompt}, {Role: openai.ChatMessageRoleUser, Content: userPrompt}},
	}
	resp, err := c.chat.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}
