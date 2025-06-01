// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// chatService defines minimal interface for chat completions.
type chatService interface {
	Create(ctx context.Context, body openai.ChatCompletionNewParams) (openai.ChatCompletion, error)
}

// Client wraps the OpenAI API client for prompt generation.
type Client struct {
	chat chatService
}

// wrapper implements chatService interface for OpenAI client
// newFunc is the underlying OpenAI call which may return a pointer
type chatServiceWrapper struct {
	newFunc func(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error)
}

// Create calls the underlying OpenAI chat completion and returns a value
func (w *chatServiceWrapper) Create(ctx context.Context, body openai.ChatCompletionNewParams) (openai.ChatCompletion, error) {
	respPtr, err := w.newFunc(ctx, body)
	if err != nil {
		return openai.ChatCompletion{}, err
	}
	return *respPtr, nil
}

// Opts holds configuration options for the GenAI client, including API key override.
// API key can be overridden via command-line options or environment variable.
type Opts struct {
	APIKey string // overrides OPENAI_API_KEY
}

// Option defines a configuration option for the GenAI client.
type Option func(*Opts)

// WithAPIKey overrides the API key used by the GenAI client.
func WithAPIKey(key string) Option {
	return func(o *Opts) {
		o.APIKey = key
	}
}

// NewClient initializes a new GenAI client using the provided options.
func NewClient(opts ...Option) (*Client, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}

	// Determine API key (required)
	apiKey := cfg.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("API key not set")
	}
	// Initialize OpenAI client with API key
	cli := openai.NewClient(option.WithAPIKey(apiKey))
	return &Client{chat: &chatServiceWrapper{newFunc: cli.Chat.Completions.New}}, nil
}

// GeneratePrompt generates content based on provided system and user prompts.
func (c *Client) GeneratePrompt(system, user string) (string, error) {
	// Prepare chat completion parameters
	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4oMini,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
	}
	resp, err := c.chat.Create(context.Background(), params)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}
