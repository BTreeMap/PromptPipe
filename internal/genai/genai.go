// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
)

// Client wraps the OpenAI client for generating prompts.
type Client struct {
	client *openai.Client
}

// NewClient initializes a new GenAI client using the OPENAI_API_KEY environment variable.
func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	c := openai.NewClient(apiKey)
	return &Client{client: c}, nil
}

// GeneratePrompt generates a response based on the provided system and user prompts.
func (c *Client) GeneratePrompt(systemPrompt string, userPrompt string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	}
	resp, err := c.client.Chat.Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}
