// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// chatService defines minimal interface for chat completions.
type chatService interface {
	CreateChatCompletion(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error)
}

// Client wraps the OpenAI ChatCompletion service for generating prompts.
type Client struct {
	chat chatService
}

// wrapper implements chatService interface for OpenAI client
type chatServiceWrapper struct {
	newFunc func(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error)
}

func (w *chatServiceWrapper) CreateChatCompletion(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	return w.newFunc(ctx, body, opts...)
}

// NewClient initializes a new GenAI client using the OPENAI_API_KEY environment variable.
func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	// Initialize OpenAI client with API key
	cli := openai.NewClient(openai.DefaultClientOptions()...)
	return &Client{chat: &chatServiceWrapper{newFunc: cli.Chat.Completions.New}}, nil
}

// GeneratePrompt generates a response based on the provided system and user prompts.
func (c *Client) GeneratePrompt(systemPrompt, userPrompt string) (string, error) {
	// Prepare chat completion parameters
	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4oMini,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
	}
	resp, err := c.chat.CreateChatCompletion(context.Background(), params)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}
