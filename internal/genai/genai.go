// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Default configuration constants
const (
	// DefaultModel is the default OpenAI model used for chat completions
	DefaultModel = openai.ChatModelGPT4oMini
	// DefaultTemperature is the default temperature setting for chat completions
	DefaultTemperature = 0.7
	// DefaultMaxTokens is the default maximum tokens for chat completions
	DefaultMaxTokens = 1000
)

// Error variables for better error handling
var (
	ErrAPIKeyNotSet      = fmt.Errorf("API key not set")
	ErrNoChoicesReturned = fmt.Errorf("no choices returned from OpenAI API")
)

// ClientInterface defines the interface for GenAI clients to improve testability.
type ClientInterface interface {
	GeneratePrompt(system, user string) (string, error)
	GeneratePromptWithContext(ctx context.Context, system, user string) (string, error)
	GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error)
	GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*ToolCallResponse, error)
}

// ChatService defines minimal interface for chat completions.
// Interface names should be descriptive and use proper Go naming conventions.
type ChatService interface {
	Create(ctx context.Context, body openai.ChatCompletionNewParams) (openai.ChatCompletion, error)
}

// Client wraps the OpenAI API client for prompt generation.
type Client struct {
	chat        ChatService
	model       string
	temperature float64
	maxTokens   int
}

// chatServiceWrapper implements ChatService interface for OpenAI client
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
	APIKey      string  // overrides OPENAI_API_KEY
	Model       string  // overrides default model
	Temperature float64 // overrides default temperature
	MaxTokens   int     // overrides default max tokens
}

// Option defines a configuration option for the GenAI client.
type Option func(*Opts)

// WithAPIKey overrides the API key used by the GenAI client.
func WithAPIKey(key string) Option {
	return func(o *Opts) {
		o.APIKey = key
	}
}

// WithModel overrides the model used by the GenAI client.
func WithModel(model string) Option {
	return func(o *Opts) {
		o.Model = model
	}
}

// WithTemperature overrides the temperature used by the GenAI client.
func WithTemperature(temp float64) Option {
	return func(o *Opts) {
		o.Temperature = temp
	}
}

// WithMaxTokens overrides the max tokens used by the GenAI client.
func WithMaxTokens(tokens int) Option {
	return func(o *Opts) {
		o.MaxTokens = tokens
	}
}

// NewClient initializes a new GenAI client using the provided options.
func NewClient(opts ...Option) (*Client, error) {
	// Apply options with defaults
	cfg := Opts{
		Model:       DefaultModel,
		Temperature: DefaultTemperature,
		MaxTokens:   DefaultMaxTokens,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Determine API key (required)
	apiKey := cfg.APIKey
	if apiKey == "" {
		return nil, ErrAPIKeyNotSet
	}
	// Initialize OpenAI client with API key
	cli := openai.NewClient(option.WithAPIKey(apiKey))
	client := &Client{
		chat:        &chatServiceWrapper{newFunc: cli.Chat.Completions.New},
		model:       cfg.Model,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
	}

	slog.Debug("GenAI client created", "model", cfg.Model, "temperature", cfg.Temperature, "maxTokens", cfg.MaxTokens)
	return client, nil
}

// GeneratePrompt generates content based on provided system and user prompts.
// It uses the provided context for cancellation and timeout handling.
func (c *Client) GeneratePrompt(system, user string) (string, error) {
	return c.GeneratePromptWithContext(context.Background(), system, user)
}

// GeneratePromptWithContext generates content based on provided system and user prompts with context.
func (c *Client) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	slog.Debug("GeneratePrompt invoked", "system", system, "user", user, "model", c.model)

	// Prepare chat completion parameters with configured options
	params := openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
		Temperature: openai.Float(c.temperature),
		MaxTokens:   openai.Int(int64(c.maxTokens)),
	}

	resp, err := c.chat.Create(ctx, params)
	if err != nil {
		slog.Error("GenAI chat.Create failed", "error", err, "model", c.model)
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		slog.Warn("GeneratePrompt no choices returned", "model", c.model)
		return "", ErrNoChoicesReturned
	}

	content := resp.Choices[0].Message.Content
	slog.Debug("GeneratePrompt succeeded", "length", len(content), "model", c.model)
	return content, nil
}

// GenerateWithMessages generates content using OpenAI's native multi-message format.
// This method supports full conversation history with proper role separation.
func (c *Client) GenerateWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("GenerateWithMessages invoked", "messageCount", len(messages), "model", c.model)

	// Prepare chat completion parameters with configured options
	params := openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    messages,
		Temperature: openai.Float(c.temperature),
		MaxTokens:   openai.Int(int64(c.maxTokens)),
	}

	resp, err := c.chat.Create(ctx, params)
	if err != nil {
		slog.Error("GenAI GenerateWithMessages failed", "error", err, "model", c.model)
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		slog.Warn("GenerateWithMessages no choices returned", "model", c.model)
		return "", ErrNoChoicesReturned
	}

	content := resp.Choices[0].Message.Content
	slog.Debug("GenerateWithMessages succeeded", "length", len(content), "model", c.model)
	return content, nil
}

// ToolCallResponse represents a response that may contain tool calls.
type ToolCallResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool/function call made by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call with name and arguments.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// GenerateWithTools generates content with the ability to call tools/functions.
// Returns either a text response or tool calls that need to be executed.
func (c *Client) GenerateWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*ToolCallResponse, error) {
	slog.Debug("GenerateWithTools invoked", "messageCount", len(messages), "toolCount", len(tools), "model", c.model)

	params := openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		Temperature: openai.Float(c.temperature),
		MaxTokens:   openai.Int(int64(c.maxTokens)),
	}

	resp, err := c.chat.Create(ctx, params)
	if err != nil {
		slog.Error("GenAI GenerateWithTools failed", "error", err, "model", c.model)
		return nil, fmt.Errorf("failed to create chat completion with tools: %w", err)
	}

	if len(resp.Choices) == 0 {
		slog.Warn("GenerateWithTools no choices returned", "model", c.model)
		return nil, ErrNoChoicesReturned
	}

	choice := resp.Choices[0]
	result := &ToolCallResponse{
		Content: choice.Message.Content,
	}

	// Extract tool calls if present
	for _, toolCall := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   toolCall.ID,
			Type: string(toolCall.Type),
			Function: FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: json.RawMessage(toolCall.Function.Arguments),
			},
		})
	}

	slog.Debug("GenerateWithTools succeeded", "contentLength", len(result.Content), "toolCallCount", len(result.ToolCalls))
	return result, nil
}
