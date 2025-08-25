// Package genai provides GenAI-enhanced operations using OpenAI API.

package genai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/util"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// DefaultModel is the default OpenAI model used for chat completions
// It's a variable so callers (e.g., main) can override the default globally.
var DefaultModel = string(openai.ChatModelGPT4oMini)

// Default configuration constants
const (
	// DefaultTemperature is the default temperature setting for chat completions
	DefaultTemperature = 0.1
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
	// GenerateThinkingWithMessages returns a structured response containing an internal "thinking" field
	// (model reasoning / chain-of-thought style summary) and a user-facing content field. The model
	// is prompted to emit JSON with keys: thinking, content. If parsing fails, the raw content is
	// returned as Content and Thinking is empty. Implementations should never error purely due to
	// JSON parse failure; they only error on transport/service issues.
	GenerateThinkingWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (*ThinkingResponse, error)
	// GenerateThinkingWithTools combines tool calling with structured thinking/content response.
	// The assistant is instructed to always place a JSON object {"thinking":"...","content":"..."}
	// in its message content even when issuing tool calls. Tool calls are returned separately via
	// OpenAI's tool call mechanism. RawContent preserves the exact content string (JSON) for
	// conversation continuity, while Content is the parsed user-facing portion.
	GenerateThinkingWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*ThinkingToolCallResponse, error)
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
	debugMode   bool   // Enable debug mode for API call logging
	stateDir    string // State directory for debug log files
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
	DebugMode   bool    // Enable debug mode for API call logging
	StateDir    string  // State directory for debug log files
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

// WithDebugMode enables debug mode for API call logging.
func WithDebugMode(enabled bool) Option {
	return func(o *Opts) {
		o.DebugMode = enabled
	}
}

// WithStateDir sets the state directory for debug log files.
func WithStateDir(stateDir string) Option {
	return func(o *Opts) {
		o.StateDir = stateDir
	}
}

// NewClient initializes a new GenAI client using the provided options.
func NewClient(opts ...Option) (*Client, error) {
	// Apply options with defaults
	cfg := Opts{
		Model:       DefaultModel,
		Temperature: DefaultTemperature,
		MaxTokens:   DefaultMaxTokens,
		DebugMode:   false,
		StateDir:    "",
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
		debugMode:   cfg.DebugMode,
		stateDir:    cfg.StateDir,
	}

	slog.Debug("Client.NewClient: client created successfully", "model", cfg.Model, "temperature", cfg.Temperature, "maxTokens", cfg.MaxTokens, "debugMode", cfg.DebugMode)
	return client, nil
}

// logAPICall logs the API call parameters and response to a debug file if debug mode is enabled.
func (c *Client) logAPICall(method string, params interface{}, response interface{}, err error) {
	if !c.debugMode || c.stateDir == "" {
		return
	}

	// Generate timestamp and random hex string for filename
	timestamp := time.Now().Format("2006-01-02.15-04-05")
	randomHex := util.GenerateRandomHex(16)

	filename := fmt.Sprintf("%s.%s.json", timestamp, randomHex)
	debugDir := filepath.Join(c.stateDir, "debug")
	filePath := filepath.Join(debugDir, filename)

	// Ensure debug directory exists
	if mkdirErr := os.MkdirAll(debugDir, 0755); mkdirErr != nil {
		slog.Warn("Failed to create debug directory", "error", mkdirErr, "path", debugDir)
		return
	}

	// Create debug log entry
	logEntry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"method":    method,
		"model":     c.model,
		"params":    params,
		"response":  response,
		"error":     nil,
	}

	if err != nil {
		logEntry["error"] = err.Error()
	}

	// Write to file
	logData, jsonErr := json.MarshalIndent(logEntry, "", "  ")
	if jsonErr != nil {
		slog.Warn("Failed to marshal debug log entry", "error", jsonErr)
		return
	}

	if writeErr := os.WriteFile(filePath, logData, 0644); writeErr != nil {
		slog.Warn("Failed to write debug log file", "error", writeErr, "path", filePath)
		return
	}

	slog.Debug("GenAI API call logged", "file", filename, "method", method)
}

// GeneratePrompt generates content based on provided system and user prompts.
// It uses the provided context for cancellation and timeout handling.
func (c *Client) GeneratePrompt(system, user string) (string, error) {
	return c.GeneratePromptWithContext(context.Background(), system, user)
}

// GeneratePromptWithContext generates content based on provided system and user prompts with context.
func (c *Client) GeneratePromptWithContext(ctx context.Context, system, user string) (string, error) {
	slog.Debug("GenAI.GeneratePrompt: generating prompt", "system", system, "user", user, "model", c.model)

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

	// Log API call for debugging
	c.logAPICall("GeneratePromptWithContext", params, resp, err)

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

	// Log API call for debugging
	c.logAPICall("GenerateWithMessages", params, resp, err)

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

// ThinkingResponse represents a structured response separating model reasoning from user-facing content.
type ThinkingResponse struct {
	Thinking string `json:"thinking"`
	Content  string `json:"content"`
}

// ThinkingToolCallResponse is like ToolCallResponse but includes model thinking and raw content.
type ThinkingToolCallResponse struct {
	Thinking   string     `json:"thinking"`
	Content    string     `json:"content"`
	RawContent string     `json:"raw_content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
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

	// Log API call for debugging
	c.logAPICall("GenerateWithTools", params, resp, err)

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

// GenerateThinkingWithMessages requests a structured response (thinking + content) from the model.
// This is implemented using an in-band JSON pattern until native structured outputs are available
// in the SDK for arbitrary schemas. The last user/system message context is appended with lightweight
// instructions that the assistant MUST respond ONLY with a compact JSON object {"thinking": "...", "content": "..."}.
// The returned Thinking segment is intended strictly for debug mode display and should not be sent
// to end users unless explicitly enabled.
func (c *Client) GenerateThinkingWithMessages(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (*ThinkingResponse, error) {
	slog.Debug("GenerateThinkingWithMessages invoked", "messageCount", len(messages), "model", c.model)

	// Append a system instruction to coerce JSON shape (non-destructive, we don't mutate caller slice)
	augmented := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	augmented = append(augmented, messages...)
	augmented = append(augmented, openai.SystemMessage("You are to produce a structured JSON response ONLY. Format strictly as {\"thinking\": string, \"content\": string}. 'thinking' = brief internal reasoning (max 120 words, no sensitive data). 'content' = final user-facing reply. Do not wrap in markdown. Do not add extra keys."))

	params := openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    augmented,
		Temperature: openai.Float(c.temperature),
		MaxTokens:   openai.Int(int64(c.maxTokens)),
	}

	resp, err := c.chat.Create(ctx, params)
	c.logAPICall("GenerateThinkingWithMessages", params, resp, err)
	if err != nil {
		slog.Error("GenAI GenerateThinkingWithMessages failed", "error", err, "model", c.model)
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		slog.Warn("GenerateThinkingWithMessages no choices returned", "model", c.model)
		return nil, ErrNoChoicesReturned
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)

	tr := &ThinkingResponse{}
	if err := json.Unmarshal([]byte(raw), tr); err != nil || (tr.Thinking == "" && tr.Content == "") {
		// Fallback: treat entire output as content
		slog.Warn("GenerateThinkingWithMessages: JSON parse failed, using raw content fallback", "error", err)
		tr.Thinking = ""
		tr.Content = raw
	}
	return tr, nil
}

// GenerateThinkingWithTools performs a chat completion with tools while asking the model to
// wrap its assistant message content in JSON {"thinking":"...","content":"..."}. This allows
// us to surface internal reasoning in debug mode while still leveraging native tool call outputs.
func (c *Client) GenerateThinkingWithTools(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (*ThinkingToolCallResponse, error) {
	slog.Debug("GenerateThinkingWithTools invoked", "messageCount", len(messages), "toolCount", len(tools), "model", c.model)

	augmented := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	augmented = append(augmented, messages...)
	augmented = append(augmented, openai.SystemMessage("Always respond with a JSON object {\"thinking\": string, \"content\": string}. If you decide to call tools, still provide this JSON: 'thinking' = concise reasoning (<=120 words), 'content' = user-facing reply (may be empty until tools executed). No extra text, no markdown."))

	params := openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    augmented,
		Tools:       tools,
		Temperature: openai.Float(c.temperature),
		MaxTokens:   openai.Int(int64(c.maxTokens)),
	}

	resp, err := c.chat.Create(ctx, params)
	c.logAPICall("GenerateThinkingWithTools", params, resp, err)
	if err != nil {
		slog.Error("GenAI GenerateThinkingWithTools failed", "error", err, "model", c.model)
		return nil, fmt.Errorf("failed to create chat completion with tools: %w", err)
	}
	if len(resp.Choices) == 0 {
		slog.Warn("GenerateThinkingWithTools no choices returned", "model", c.model)
		return nil, ErrNoChoicesReturned
	}
	choice := resp.Choices[0]
	raw := strings.TrimSpace(choice.Message.Content)
	result := &ThinkingToolCallResponse{RawContent: raw}

	// Parse thinking/content JSON if possible
	var temp ThinkingResponse
	if err := json.Unmarshal([]byte(raw), &temp); err == nil {
		result.Thinking = temp.Thinking
		result.Content = temp.Content
	} else {
		// Fallback: treat raw as user-facing content
		slog.Warn("GenerateThinkingWithTools: JSON parse failed, using raw content fallback", "error", err)
		result.Content = raw
	}

	// Extract tool calls
	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: string(tc.Type),
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return result, nil
}
