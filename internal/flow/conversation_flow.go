// Package flow provides conversation flow implementation for persistent conversational interactions.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// Context key for storing phone number in context
type contextKey string

const phoneNumberContextKey contextKey = "phone_number"

// GetPhoneNumberContextKey returns the context key used for storing phone numbers
func GetPhoneNumberContextKey() contextKey {
	return phoneNumberContextKey
}

// GetPhoneNumberFromContext retrieves the phone number from the context
func GetPhoneNumberFromContext(ctx context.Context) (string, bool) {
	phoneNumber, ok := ctx.Value(phoneNumberContextKey).(string)
	return phoneNumber, ok && phoneNumber != ""
}

// ConversationMessage represents a single message in the conversation history.
type ConversationMessage struct {
	Role      string    `json:"role"`      // "user" or "assistant"
	Content   string    `json:"content"`   // message content
	Timestamp time.Time `json:"timestamp"` // when the message was sent
}

// ConversationHistory represents the full conversation history for a participant.
type ConversationHistory struct {
	Messages []ConversationMessage `json:"messages"`
}

// ConversationFlow implements a stateful conversation flow that maintains history and uses GenAI.
type ConversationFlow struct {
	stateManager              StateManager
	genaiClient               genai.ClientInterface
	systemPrompt              string
	systemPromptFile          string
	schedulerTool             *SchedulerTool             // Tool for scheduling daily prompts
	oneMinuteInterventionTool *OneMinuteInterventionTool // Tool for initiating one-minute interventions
}

// NewConversationFlow creates a new conversation flow with dependencies.
func NewConversationFlow(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string) *ConversationFlow {
	slog.Debug("Creating ConversationFlow with dependencies", "systemPromptFile", systemPromptFile)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
	}
}

// NewConversationFlowWithScheduler creates a new conversation flow with scheduler tool support.
func NewConversationFlowWithScheduler(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool) *ConversationFlow {
	slog.Debug("Creating ConversationFlow with scheduler tool", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasSchedulerTool", schedulerTool != nil)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
		schedulerTool:    schedulerTool,
	}
}

// NewConversationFlowWithTools creates a new conversation flow with both scheduler and intervention tools.
func NewConversationFlowWithTools(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool, interventionTool *OneMinuteInterventionTool) *ConversationFlow {
	slog.Debug("Creating ConversationFlow with tools", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasSchedulerTool", schedulerTool != nil, "hasInterventionTool", interventionTool != nil)
	return &ConversationFlow{
		stateManager:              stateManager,
		genaiClient:               genaiClient,
		systemPromptFile:          systemPromptFile,
		schedulerTool:             schedulerTool,
		oneMinuteInterventionTool: interventionTool,
	}
}

// SetDependencies injects dependencies into the flow.
func (f *ConversationFlow) SetDependencies(deps Dependencies) {
	slog.Debug("ConversationFlow SetDependencies called")
	f.stateManager = deps.StateManager
	// Note: genaiClient needs to be set separately as it's not part of standard Dependencies
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (f *ConversationFlow) LoadSystemPrompt() error {
	if f.systemPromptFile == "" {
		return fmt.Errorf("system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(f.systemPromptFile); os.IsNotExist(err) {
		return fmt.Errorf("system prompt file does not exist: %s", f.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(f.systemPromptFile)
	if err != nil {
		slog.Error("Failed to read system prompt file", "file", f.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read system prompt file: %w", err)
	}

	f.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("System prompt loaded successfully", "file", f.systemPromptFile, "length", len(f.systemPrompt))
	return nil
}

// Generate generates conversation responses based on user input and history.
func (f *ConversationFlow) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("ConversationFlow Generate invoked", "to", p.To, "userPrompt", p.UserPrompt)

	// For simple message generation without state operations, dependencies are not required
	// Dependencies are only needed for stateful operations like maintaining conversation history
	switch p.State {
	case "", models.StateConversationActive:
		// Return a generic response - actual conversation logic happens in ProcessResponse
		return "I'm ready to chat! Send me a message to start our conversation.", nil
	default:
		slog.Error("ConversationFlow unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf("unsupported conversation flow state '%s'", p.State)
	}
}

// ProcessResponse handles participant responses and maintains conversation state.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	// Log context information for debugging
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("ConversationFlow ProcessResponse context check",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber,
		"responseLength", len(response))

	// Validate dependencies for stateful operations
	if f.stateManager == nil || f.genaiClient == nil {
		slog.Error("ConversationFlow dependencies not initialized for state operations")
		return "", fmt.Errorf("flow dependencies not properly initialized for state operations")
	}

	// Load system prompt if not already loaded
	if f.systemPrompt == "" {
		if err := f.LoadSystemPrompt(); err != nil {
			// If system prompt file doesn't exist or fails to load, use a default
			f.systemPrompt = "You are a helpful AI assistant. Engage in natural conversation with the user."
			slog.Warn("Using default system prompt due to load failure", "error", err)
		}
	}

	// Get current state
	currentState, err := f.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		slog.Error("ConversationFlow ProcessResponse: failed to get current state", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	// If no state exists, initialize the conversation
	if currentState == "" {
		err = f.transitionToState(ctx, participantID, models.StateConversationActive)
		if err != nil {
			return "", err
		}
		// Update currentState to the new state
		currentState = models.StateConversationActive
		// Initialize empty conversation history
		emptyHistory := ConversationHistory{Messages: []ConversationMessage{}}
		historyJSON, _ := json.Marshal(emptyHistory)
		f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, string(historyJSON))
		slog.Debug("ConversationFlow initialized new conversation", "participantID", participantID)
	}

	slog.Debug("ConversationFlow ProcessResponse", "participantID", participantID, "response", response, "currentState", currentState)

	// Process the conversation response
	switch currentState {
	case models.StateConversationActive:
		return f.processConversationMessage(ctx, participantID, response)
	default:
		slog.Warn("ConversationFlow ProcessResponse: unhandled state", "state", currentState, "participantID", participantID)
		return "", fmt.Errorf("unhandled conversation state: %s", currentState)
	}
}

// processConversationMessage handles a user message and generates an AI response.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) processConversationMessage(ctx context.Context, participantID, userMessage string) (string, error) {
	// Get conversation history
	history, err := f.getConversationHistory(ctx, participantID)
	if err != nil {
		slog.Error("ConversationFlow failed to get conversation history", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Add user message to history
	userMsg := ConversationMessage{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, userMsg)

	// Build OpenAI messages using native multi-message format
	messages, err := f.buildOpenAIMessages(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow failed to build OpenAI messages", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build OpenAI messages: %w", err)
	}

	// Check if any tools are available and we should enable tool calling
	if (f.schedulerTool != nil || f.oneMinuteInterventionTool != nil) && f.genaiClient != nil {
		// Use tool-enabled generation
		return f.processWithTools(ctx, participantID, messages, history)
	}

	// Fallback to standard generation without tools
	response, err := f.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("ConversationFlow GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Add assistant response to history
	assistantMsg := ConversationMessage{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	// Save updated history
	err = f.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	// Return the AI response for sending
	slog.Info("ConversationFlow generated response", "participantID", participantID, "responseLength", len(response))
	return response, nil
}

// processWithTools handles conversation with tool calling capability.
func (f *ConversationFlow) processWithTools(ctx context.Context, participantID string, messages []openai.ChatCompletionMessageParamUnion, history *ConversationHistory) (string, error) {
	slog.Debug("ConversationFlow processWithTools", "participantID", participantID, "messageCount", len(messages))

	// Create tool definitions
	tools := []openai.ChatCompletionToolParam{}

	if f.schedulerTool != nil {
		toolDef := f.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("ConversationFlow added scheduler tool",
			"participantID", participantID,
			"toolName", "scheduler")
	} else {
		slog.Debug("ConversationFlow scheduler tool not available", "participantID", participantID)
	}

	if f.oneMinuteInterventionTool != nil {
		toolDef := f.oneMinuteInterventionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("ConversationFlow added intervention tool",
			"participantID", participantID,
			"toolName", "initiate_intervention")
	} else {
		slog.Debug("ConversationFlow intervention tool not available", "participantID", participantID)
	}

	slog.Info("ConversationFlow calling GenAI with tools",
		"participantID", participantID,
		"toolCount", len(tools),
		"messageCount", len(messages))

	// Generate response with tools
	toolResponse, err := f.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("ConversationFlow tool generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate response with tools: %w", err)
	}

	slog.Debug("ConversationFlow received tool response",
		"participantID", participantID,
		"hasContent", toolResponse.Content != "",
		"contentLength", len(toolResponse.Content),
		"toolCallCount", len(toolResponse.ToolCalls))

	// Check if the AI wants to call tools
	if len(toolResponse.ToolCalls) > 0 {
		slog.Info("ConversationFlow processing tool calls", "participantID", participantID, "toolCallCount", len(toolResponse.ToolCalls))
		return f.handleToolCalls(ctx, participantID, toolResponse, history, messages, tools)
	}

	// No tool calls, process as regular response
	if toolResponse.Content == "" {
		slog.Warn("ConversationFlow received empty content and no tool calls", "participantID", participantID)
		return "I'm here to help you with your habits. What would you like to work on?", nil
	}

	// Add assistant response to history
	assistantMsg := ConversationMessage{
		Role:      "assistant",
		Content:   toolResponse.Content,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	// Save updated history
	err = f.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	slog.Info("ConversationFlow generated tool-enabled response", "participantID", participantID, "responseLength", len(toolResponse.Content))
	return toolResponse.Content, nil
}

// handleToolCalls processes tool calls from the AI and executes them.
func (f *ConversationFlow) handleToolCalls(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, history *ConversationHistory, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (string, error) {
	// Add assistant message with tool calls to history
	assistantMsg := ConversationMessage{
		Role:      "assistant", // May be empty if only tool calls
		Content:   toolResponse.Content,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	var toolResults []string

	// Execute each tool call
	for i, toolCall := range toolResponse.ToolCalls {
		slog.Info("ConversationFlow executing tool call",
			"participantID", participantID,
			"toolName", toolCall.Function.Name,
			"toolCallID", toolCall.ID,
			"toolIndex", i,
			"totalTools", len(toolResponse.ToolCalls))

		// Log the tool call details for debugging
		slog.Debug("ConversationFlow tool call details",
			"participantID", participantID,
			"toolCallID", toolCall.ID,
			"functionName", toolCall.Function.Name,
			"argumentsLength", len(toolCall.Function.Arguments),
			"arguments", string(toolCall.Function.Arguments))

		switch toolCall.Function.Name {
		case "scheduler":
			result, err := f.executeSchedulerTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("ConversationFlow scheduler tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't set up your scheduling: %s", err.Error())
				toolResults = append(toolResults, errorMsg)
			} else if !result.Success {
				slog.Warn("ConversationFlow scheduler tool returned error", "error", result.Error, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ %s", result.Message)
				toolResults = append(toolResults, errorMsg)
			} else {
				toolResults = append(toolResults, result.Message)
			}

		case "initiate_intervention":
			result, err := f.executeInterventionTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("ConversationFlow intervention tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't start your intervention: %s", err.Error())
				toolResults = append(toolResults, errorMsg)
			} else if !result.Success {
				slog.Warn("ConversationFlow intervention tool returned error", "error", result.Error, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ %s", result.Message)
				toolResults = append(toolResults, errorMsg)
			} else {
				toolResults = append(toolResults, result.Message)
			}

		default:
			slog.Warn("ConversationFlow unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			errorMsg := fmt.Sprintf("❌ Sorry, I don't know how to use the tool '%s'", toolCall.Function.Name)
			toolResults = append(toolResults, errorMsg)
		}
	}

	// Combine tool results into a single response
	finalResponse := ""
	if len(toolResults) == 1 {
		finalResponse = toolResults[0]
	} else if len(toolResults) > 1 {
		finalResponse = strings.Join(toolResults, "\n\n")
	} else {
		finalResponse = "✅ Done! I've processed your request."
	}

	// Add tool execution result to history
	toolResultMsg := ConversationMessage{
		Role:      "assistant",
		Content:   finalResponse,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, toolResultMsg)

	// Save updated history
	err := f.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow failed to save conversation history after tool execution", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	slog.Info("ConversationFlow completed tool execution", "participantID", participantID, "toolCount", len(toolResponse.ToolCalls), "responseLength", len(finalResponse))
	return finalResponse, nil
}

// transitionToState safely transitions to a new state with logging
func (f *ConversationFlow) transitionToState(ctx context.Context, participantID string, newState models.StateType) error {
	err := f.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, newState)
	if err != nil {
		slog.Error("ConversationFlow failed to transition state", "error", err, "participantID", participantID, "newState", newState)
		return fmt.Errorf("failed to transition to state %s: %w", newState, err)
	}
	slog.Info("ConversationFlow state transition", "participantID", participantID, "newState", newState)
	return nil
}

// getConversationHistory retrieves and parses conversation history from state storage.
func (f *ConversationFlow) getConversationHistory(ctx context.Context, participantID string) (*ConversationHistory, error) {
	historyJSON, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	var history ConversationHistory
	if historyJSON == "" {
		// Return empty history if none exists
		return &ConversationHistory{Messages: []ConversationMessage{}}, nil
	}

	err = json.Unmarshal([]byte(historyJSON), &history)
	if err != nil {
		slog.Error("ConversationFlow failed to parse conversation history", "error", err, "participantID", participantID)
		// Return empty history if parsing fails
		return &ConversationHistory{Messages: []ConversationMessage{}}, nil
	}

	return &history, nil
}

// saveConversationHistory saves conversation history to state storage.
func (f *ConversationFlow) saveConversationHistory(ctx context.Context, participantID string, history *ConversationHistory) error {
	// Optionally limit history length to prevent unbounded growth
	const maxHistoryLength = 50 // Keep last 50 messages
	if len(history.Messages) > maxHistoryLength {
		// Keep the most recent messages
		history.Messages = history.Messages[len(history.Messages)-maxHistoryLength:]
		slog.Debug("ConversationFlow trimmed history to max length", "participantID", participantID, "maxLength", maxHistoryLength)
	}

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	err = f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, string(historyJSON))
	if err != nil {
		return fmt.Errorf("failed to save conversation history: %w", err)
	}

	return nil
}

// buildOpenAIMessages creates OpenAI message array with system prompt, participant background, and conversation history.
// Follows the structure: system prompt + user background (as system message) + conversation history + current instruction
func (f *ConversationFlow) buildOpenAIMessages(ctx context.Context, participantID string, history *ConversationHistory) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// 1. Add system prompt
	if f.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(f.systemPrompt))
	}

	// 2. Get and add participant background as system message
	participantBackground, err := f.getParticipantBackground(ctx, participantID)
	if err != nil {
		slog.Warn("Failed to get participant background", "error", err, "participantID", participantID)
	} else if participantBackground != "" {
		messages = append(messages, openai.SystemMessage(participantBackground))
		slog.Debug("Added participant background to messages", "participantID", participantID, "backgroundLength", len(participantBackground))
	} else {
		slog.Debug("No participant background found", "participantID", participantID)
	}

	// 3. Add conversation history (part of stored "history")
	// Limit history to prevent token overflow (keep last 30 messages for context)
	historyMessages := history.Messages
	const maxHistoryMessages = 30
	if len(historyMessages) > maxHistoryMessages {
		historyMessages = historyMessages[len(historyMessages)-maxHistoryMessages:]
	}

	for _, msg := range historyMessages {
		if msg.Role == "user" {
			messages = append(messages, openai.UserMessage(msg.Content))
		} else if msg.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	// 4. Current instruction (for first message generation)
	// This is handled implicitly - the last user message in history serves as the current instruction

	return messages, nil
}

// getParticipantBackground retrieves participant background information from state storage
func (f *ConversationFlow) getParticipantBackground(ctx context.Context, participantID string) (string, error) {
	// Try to get participant background from state data
	background, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyParticipantBackground)
	if err != nil {
		slog.Debug("Error retrieving participant background", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get participant background: %w", err)
	}

	slog.Debug("Retrieved participant background from state", "participantID", participantID, "backgroundLength", len(background), "isEmpty", background == "")

	if background == "" {
		return "", nil
	}

	// Format as a system message
	formatted := fmt.Sprintf("PARTICIPANT BACKGROUND:\n%s", background)
	slog.Debug("Formatted participant background", "participantID", participantID, "formattedLength", len(formatted))
	return formatted, nil
}

// GetSystemPromptPath returns the default system prompt file path.
func GetSystemPromptPath() string {
	// Default to a prompts directory in the project root
	return filepath.Join("prompts", "conversation_system.txt")
}

// CreateDefaultSystemPromptFile creates a default system prompt file if it doesn't exist.
func CreateDefaultSystemPromptFile(filePath string) error {
	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		slog.Debug("System prompt file already exists", "path", filePath)
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Default system prompt content
	defaultContent := `You are a helpful, knowledgeable, and empathetic AI assistant. You engage in natural conversation with users, providing thoughtful and informative responses.

Key guidelines:
- Be conversational and friendly
- Provide helpful and accurate information
- Ask clarifying questions when needed
- Remember the context of our conversation
- Be concise but thorough in your responses
- Show empathy and understanding

Your goal is to have meaningful conversations and assist users with their questions and needs.`

	// Write default content to file
	err := os.WriteFile(filePath, []byte(defaultContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write default system prompt file: %w", err)
	}

	slog.Info("Created default system prompt file", "path", filePath)
	return nil
}

// executeSchedulerTool executes a scheduler tool call.
func (f *ConversationFlow) executeSchedulerTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (*models.ToolResult, error) {
	// Log the raw tool call for debugging
	slog.Debug("ConversationFlow executeSchedulerTool raw call",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"functionName", toolCall.Function.Name,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the scheduler parameters from the tool call
	var params models.SchedulerToolParams
	if err := json.Unmarshal(toolCall.Function.Arguments, &params); err != nil {
		slog.Error("ConversationFlow failed to parse scheduler parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return &models.ToolResult{
			Success: false,
			Message: "Failed to parse scheduling parameters",
			Error:   err.Error(),
		}, fmt.Errorf("failed to unmarshal scheduler parameters: %w", err)
	}

	// Log parsed parameters for debugging
	slog.Debug("ConversationFlow parsed scheduler parameters",
		"participantID", participantID,
		"type", params.Type,
		"fixedTime", params.FixedTime,
		"timezone", params.Timezone,
		"randomStartTime", params.RandomStartTime,
		"randomEndTime", params.RandomEndTime,
		"promptSystemPrompt", params.PromptSystemPrompt,
		"promptUserPrompt", params.PromptUserPrompt,
		"habitDescription", params.HabitDescription)

	// Auto-detect and fix missing type field based on provided parameters
	if params.Type == "" {
		if params.FixedTime != "" {
			params.Type = models.SchedulerTypeFixed
			slog.Info("ConversationFlow auto-detected scheduler type as 'fixed'", 
				"participantID", participantID, 
				"reason", "fixed_time provided")
		} else if params.RandomStartTime != "" && params.RandomEndTime != "" {
			params.Type = models.SchedulerTypeRandom
			slog.Info("ConversationFlow auto-detected scheduler type as 'random'", 
				"participantID", participantID, 
				"reason", "random start and end times provided")
		} else {
			slog.Error("ConversationFlow cannot determine scheduler type", 
				"participantID", participantID,
				"fixedTime", params.FixedTime,
				"randomStartTime", params.RandomStartTime,
				"randomEndTime", params.RandomEndTime)
			return &models.ToolResult{
				Success: false,
				Message: "Cannot determine scheduler type. Please specify either a fixed time or random time window.",
				Error:   "type field missing and cannot be auto-detected",
			}, fmt.Errorf("type field missing and cannot be auto-detected")
		}
		
		// Log the corrected parameters
		slog.Debug("ConversationFlow corrected scheduler parameters", 
			"participantID", participantID,
			"correctedType", params.Type)
	}

	// Check if phone number is available in context
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("ConversationFlow scheduler tool context check",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber)

	// Execute the scheduler tool
	return f.schedulerTool.ExecuteScheduler(ctx, participantID, params)
}

// executeInterventionTool executes an intervention tool call.
func (f *ConversationFlow) executeInterventionTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (*models.ToolResult, error) {
	// Log the raw tool call for debugging
	slog.Debug("ConversationFlow executeInterventionTool raw call",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"functionName", toolCall.Function.Name,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the intervention parameters from the tool call
	var params models.OneMinuteInterventionToolParams
	if err := json.Unmarshal(toolCall.Function.Arguments, &params); err != nil {
		slog.Error("ConversationFlow failed to parse intervention parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return &models.ToolResult{
			Success: false,
			Message: "Failed to parse intervention parameters",
			Error:   err.Error(),
		}, fmt.Errorf("failed to unmarshal intervention parameters: %w", err)
	}

	// Log parsed parameters for debugging
	slog.Debug("ConversationFlow parsed intervention parameters",
		"participantID", participantID,
		"params", fmt.Sprintf("%+v", params))

	// Get phone number from context
	phoneNumber, ok := ctx.Value(phoneNumberContextKey).(string)
	slog.Debug("ConversationFlow intervention tool context check",
		"participantID", participantID,
		"hasPhoneNumber", ok,
		"phoneNumber", phoneNumber,
		"contextValue", ctx.Value(phoneNumberContextKey))

	if !ok || phoneNumber == "" {
		slog.Error("ConversationFlow intervention tool missing phone number",
			"participantID", participantID,
			"contextHasPhoneNumber", ok,
			"phoneNumber", phoneNumber)
		return &models.ToolResult{
			Success: false,
			Message: "Phone number not available for intervention",
			Error:   "phone number not found in context",
		}, fmt.Errorf("phone number not found in context")
	}

	// Execute the intervention tool
	return f.oneMinuteInterventionTool.ExecuteOneMinuteIntervention(ctx, participantID, params)
}
