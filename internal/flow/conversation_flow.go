// Package flow provides conversation flow implementation for persistent conversational interactions.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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
	chatHistoryLimit          int                        // Limit for number of history messages sent to bot tools (-1: no limit, 0: no history, positive: limit to last N messages)
	schedulerTool             *SchedulerTool             // Tool for scheduling daily prompts
	oneMinuteInterventionTool *OneMinuteInterventionTool // Tool for initiating one-minute interventions
	intakeBotTool             *IntakeBotTool             // Tool for conducting intake conversations
	promptGeneratorTool       *PromptGeneratorTool       // Tool for generating personalized habit prompts
	feedbackTrackerTool       *FeedbackTrackerTool       // Tool for tracking user feedback and updating profiles
}

// NewConversationFlow creates a new conversation flow with dependencies.
func NewConversationFlow(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string) *ConversationFlow {
	slog.Debug("flow.NewConversationFlow: creating flow with dependencies", "systemPromptFile", systemPromptFile)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
		chatHistoryLimit: -1, // Default: no limit
	}
}

// NewConversationFlowWithScheduler creates a new conversation flow with scheduler tool support.
func NewConversationFlowWithScheduler(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool) *ConversationFlow {
	slog.Debug("flow.NewConversationFlowWithScheduler: creating flow with scheduler tool", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasSchedulerTool", schedulerTool != nil)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
		chatHistoryLimit: -1, // Default: no limit
		schedulerTool:    schedulerTool,
	}
}

// NewConversationFlowWithTools creates a new conversation flow with both scheduler and intervention tools.
func NewConversationFlowWithTools(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool, interventionTool *OneMinuteInterventionTool) *ConversationFlow {
	slog.Debug("flow.NewConversationFlowWithTools: creating flow with both tools", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasSchedulerTool", schedulerTool != nil, "hasInterventionTool", interventionTool != nil)
	return &ConversationFlow{
		stateManager:              stateManager,
		genaiClient:               genaiClient,
		systemPromptFile:          systemPromptFile,
		chatHistoryLimit:          -1, // Default: no limit
		schedulerTool:             schedulerTool,
		oneMinuteInterventionTool: interventionTool,
	}
}

// NewConversationFlowWithAllTools creates a new conversation flow with all tools for the 3-bot architecture.
func NewConversationFlowWithAllTools(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, msgService MessagingService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile string) *ConversationFlow {
	return NewConversationFlowWithAllToolsAndTimeouts(stateManager, genaiClient, systemPromptFile, msgService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile, "15m", "3h")
}

// NewConversationFlowWithAllToolsAndTimeouts creates a new conversation flow with all tools and configurable feedback timeouts for the 3-bot architecture.
func NewConversationFlowWithAllToolsAndTimeouts(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, msgService MessagingService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile, feedbackInitialTimeout, feedbackFollowupDelay string) *ConversationFlow {
	slog.Debug("flow.NewConversationFlowWithAllToolsAndTimeouts: creating flow with all tools and timeouts", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "intakeBotPromptFile", intakeBotPromptFile, "promptGeneratorPromptFile", promptGeneratorPromptFile, "feedbackTrackerPromptFile", feedbackTrackerPromptFile, "feedbackInitialTimeout", feedbackInitialTimeout, "feedbackFollowupDelay", feedbackFollowupDelay)

	// Create timer for scheduler
	timer := NewSimpleTimer()

	// Create all tools with their respective system prompt files
	schedulerTool := NewSchedulerToolWithGenAI(timer, msgService, genaiClient)
	interventionTool := NewOneMinuteInterventionTool(stateManager, genaiClient, msgService)
	intakeBotTool := NewIntakeBotTool(stateManager, genaiClient, msgService, intakeBotPromptFile)
	promptGeneratorTool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, promptGeneratorPromptFile)
	feedbackTrackerTool := NewFeedbackTrackerToolWithTimeouts(stateManager, genaiClient, feedbackTrackerPromptFile, timer, msgService, feedbackInitialTimeout, feedbackFollowupDelay)

	return &ConversationFlow{
		stateManager:              stateManager,
		genaiClient:               genaiClient,
		systemPromptFile:          systemPromptFile,
		chatHistoryLimit:          -1, // Default: no limit
		schedulerTool:             schedulerTool,
		oneMinuteInterventionTool: interventionTool,
		intakeBotTool:             intakeBotTool,
		promptGeneratorTool:       promptGeneratorTool,
		feedbackTrackerTool:       feedbackTrackerTool,
	}
}

// SetDependencies injects dependencies into the flow.
func (f *ConversationFlow) SetDependencies(deps Dependencies) {
	slog.Debug("flow.SetDependencies: injecting dependencies")
	f.stateManager = deps.StateManager
	// Note: genaiClient needs to be set separately as it's not part of standard Dependencies
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (f *ConversationFlow) LoadSystemPrompt() error {
	slog.Debug("flow.LoadSystemPrompt: loading system prompt from file", "file", f.systemPromptFile)

	if f.systemPromptFile == "" {
		slog.Error("flow.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(f.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.LoadSystemPrompt: system prompt file does not exist", "file", f.systemPromptFile)
		return fmt.Errorf("system prompt file does not exist: %s", f.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(f.systemPromptFile)
	if err != nil {
		slog.Error("flow.LoadSystemPrompt: failed to read system prompt file", "file", f.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read system prompt file: %w", err)
	}

	f.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.LoadSystemPrompt: system prompt loaded successfully", "file", f.systemPromptFile, "length", len(f.systemPrompt))
	return nil
}

// LoadToolSystemPrompts loads system prompts for all tools.
func (f *ConversationFlow) LoadToolSystemPrompts() error {
	slog.Debug("flow.LoadToolSystemPrompts: loading tool system prompts")

	// Load intake bot system prompt
	if f.intakeBotTool != nil {
		if err := f.intakeBotTool.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load intake bot system prompt", "error", err)
			// Continue even if intake bot prompt fails to load
		}
	}

	// Load prompt generator system prompt
	if f.promptGeneratorTool != nil {
		if err := f.promptGeneratorTool.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load prompt generator system prompt", "error", err)
			// Continue even if prompt generator prompt fails to load
		}
	}

	// Load feedback tracker system prompt
	if f.feedbackTrackerTool != nil {
		if err := f.feedbackTrackerTool.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load feedback tracker system prompt", "error", err)
			// Continue even if feedback tracker prompt fails to load
		}
	}

	slog.Info("flow.LoadToolSystemPrompts: tool system prompts loaded")
	return nil
}

// Generate generates conversation responses based on user input and history.
func (f *ConversationFlow) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("flow.Generate: generating response", "to", p.To, "userPrompt", p.UserPrompt)

	// For simple message generation without state operations, dependencies are not required
	// Dependencies are only needed for stateful operations like maintaining conversation history
	switch p.State {
	case "", models.StateConversationActive:
		// Return a generic response - actual conversation logic happens in ProcessResponse
		return "I'm ready to chat! Send me a message to start our conversation.", nil
	default:
		slog.Error("flow.Generate: unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf("unsupported conversation flow state '%s'", p.State)
	}
}

// ProcessResponse handles participant responses and maintains conversation state.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	// Log context information for debugging
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("flow.ProcessResponse: checking context",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber,
		"responseLength", len(response))

	// Validate dependencies for stateful operations
	if f.stateManager == nil || f.genaiClient == nil {
		slog.Error("flow.ProcessResponse: dependencies not initialized for state operations")
		return "", fmt.Errorf("flow dependencies not properly initialized for state operations")
	}

	// Load system prompt if not already loaded
	if f.systemPrompt == "" {
		slog.Debug("flow.ProcessResponse: loading system prompt", "participantID", participantID)
		if err := f.LoadSystemPrompt(); err != nil {
			// If system prompt file doesn't exist or fails to load, use a default
			f.systemPrompt = "You are a helpful AI assistant. Engage in natural conversation with the user."
			slog.Warn("flow.ProcessResponse: using default system prompt due to load failure", "error", err)
		}
	}

	// Get current state
	currentState, err := f.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		slog.Error("flow.ProcessResponse: failed to get current state", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	// If no state exists, initialize the conversation
	if currentState == "" {
		slog.Debug("flow.ProcessResponse: initializing new conversation", "participantID", participantID)
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
		slog.Debug("flow.ProcessResponse: initialized new conversation", "participantID", participantID)
	}

	slog.Debug("flow.ProcessResponse: processing conversation message", "participantID", participantID, "response", response, "currentState", currentState)

	// Process the conversation response
	switch currentState {
	case models.StateConversationActive:
		return f.processConversationMessage(ctx, participantID, response)
	default:
		slog.Warn("flow.ProcessResponse: unhandled state", "state", currentState, "participantID", participantID)
		return "", fmt.Errorf("unhandled conversation state: %s", currentState)
	}
}

// processConversationMessage handles a user message and generates an AI response.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) processConversationMessage(ctx context.Context, participantID, userMessage string) (string, error) {
	// Get conversation history
	history, err := f.getConversationHistory(ctx, participantID)
	if err != nil {
		slog.Error("flow.failed to get conversation history", "error", err, "participantID", participantID)
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
		slog.Error("flow.failed to build OpenAI messages", "error", err, "participantID", participantID)
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
		slog.Error("flow.GenAI generation failed", "error", err, "participantID", participantID)
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
		slog.Error("flow.failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	// Return the AI response for sending
	slog.Info("flow.generated response", "participantID", participantID, "responseLength", len(response))
	return response, nil
}

// processWithTools handles conversation with tool calling capability.
func (f *ConversationFlow) processWithTools(ctx context.Context, participantID string, messages []openai.ChatCompletionMessageParamUnion, history *ConversationHistory) (string, error) {
	slog.Debug("flow.processWithTools", "participantID", participantID, "messageCount", len(messages))

	// Create tool definitions
	tools := []openai.ChatCompletionToolParam{}

	if f.schedulerTool != nil {
		toolDef := f.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("flow.added scheduler tool",
			"participantID", participantID,
			"toolName", "scheduler")
	} else {
		slog.Debug("flow.scheduler tool not available", "participantID", participantID)
	}

	if f.oneMinuteInterventionTool != nil {
		toolDef := f.oneMinuteInterventionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("flow.added intervention tool",
			"participantID", participantID,
			"toolName", "initiate_intervention")
	} else {
		slog.Debug("flow.intervention tool not available", "participantID", participantID)
	}

	if f.intakeBotTool != nil {
		toolDef := f.intakeBotTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("flow.added intake bot tool",
			"participantID", participantID,
			"toolName", "conduct_intake")

		// Also add the profile save tool when intake bot is available
		profileSaveToolDef := f.intakeBotTool.GetProfileSaveToolDefinition()
		tools = append(tools, profileSaveToolDef)
		slog.Debug("flow.added profile save tool",
			"participantID", participantID,
			"toolName", "save_user_profile")
	} else {
		slog.Debug("flow.intake bot tool not available", "participantID", participantID)
	}

	if f.promptGeneratorTool != nil {
		toolDef := f.promptGeneratorTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("flow.added prompt generator tool",
			"participantID", participantID,
			"toolName", "generate_habit_prompt")
	} else {
		slog.Debug("flow.prompt generator tool not available", "participantID", participantID)
	}

	if f.feedbackTrackerTool != nil {
		toolDef := f.feedbackTrackerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("flow.added feedback tracker tool",
			"participantID", participantID,
			"toolName", "track_feedback")
	} else {
		slog.Debug("flow.feedback tracker tool not available", "participantID", participantID)
	}

	slog.Info("flow.calling GenAI with tools",
		"participantID", participantID,
		"toolCount", len(tools),
		"messageCount", len(messages))

	// Generate response with tools
	toolResponse, err := f.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("flow.tool generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate response with tools: %w", err)
	}

	slog.Debug("flow.received tool response",
		"participantID", participantID,
		"hasContent", toolResponse.Content != "",
		"contentLength", len(toolResponse.Content),
		"toolCallCount", len(toolResponse.ToolCalls))

	// Check if the AI wants to call tools
	if len(toolResponse.ToolCalls) > 0 {
		slog.Info("flow.processing tool calls", "participantID", participantID, "toolCallCount", len(toolResponse.ToolCalls))
		return f.handleToolCalls(ctx, participantID, toolResponse, history, messages, tools)
	}

	// No tool calls, process as regular response
	if toolResponse.Content == "" {
		slog.Warn("flow.received empty content and no tool calls", "participantID", participantID)
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
		slog.Error("flow.failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	slog.Info("flow.generated tool-enabled response", "participantID", participantID, "responseLength", len(toolResponse.Content))
	return toolResponse.Content, nil
}

// handleToolCalls processes tool calls from the AI and executes them.
func (f *ConversationFlow) handleToolCalls(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, history *ConversationHistory, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (string, error) {
	// Add assistant message with tool calls to conversation history
	assistantMsg := ConversationMessage{
		Role:      "assistant", // May be empty if only tool calls
		Content:   toolResponse.Content,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	// Add assistant message with tool calls to OpenAI conversation context
	// For tool calls, we need to add the assistant message (even if empty content)
	// The OpenAI API expects this structure: assistant message -> tool result messages
	messages = append(messages, openai.AssistantMessage(toolResponse.Content))

	// Collect tool call results for adding to OpenAI conversation
	var userMessages []string
	var historyRecords []string

	// Execute each tool call
	for i, toolCall := range toolResponse.ToolCalls {
		slog.Info("flow.executing tool call",
			"participantID", participantID,
			"toolName", toolCall.Function.Name,
			"toolCallID", toolCall.ID,
			"toolIndex", i,
			"totalTools", len(toolResponse.ToolCalls))

		// Log the tool call details for debugging
		slog.Debug("flow.tool call details",
			"participantID", participantID,
			"toolCallID", toolCall.ID,
			"functionName", toolCall.Function.Name,
			"argumentsLength", len(toolCall.Function.Arguments),
			"arguments", string(toolCall.Function.Arguments))

		switch toolCall.Function.Name {
		case "scheduler":
			result, err := f.executeSchedulerTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.scheduler tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't set up your scheduling: %s", err.Error())
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else if !result.Success {
				slog.Warn("flow.scheduler tool returned error", "error", result.Error, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ %s", result.Message)
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Scheduler success: send message to user and record in history
				userMessages = append(userMessages, result.Message)
				historyRecords = append(historyRecords, result.Message)
			}

		case "initiate_intervention":
			result, err := f.executeInterventionTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.intervention tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't start your intervention: %s", err.Error())
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else if !result.Success {
				slog.Warn("flow.intervention tool returned error", "error", result.Error, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ %s", result.Message)
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Intervention success: record in history but don't send message to user
				// (the tool already sent the intervention content directly)
				slog.Info("flow.intervention tool executed successfully",
					"participantID", participantID,
					"toolCallID", toolCall.ID,
					"successMessage", result.Message)

				// No user message (intervention was sent directly)
				// Record in history so AI knows intervention was sent
				historyRecords = append(historyRecords, fmt.Sprintf("[INTERVENTION_SENT: %s]", result.Message))
			}

		case "conduct_intake":
			result, err := f.executeIntakeBotTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.intake bot tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't conduct the intake: %s", err.Error())
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Intake bot success: send response to user and record in history
				userMessages = append(userMessages, result)
				historyRecords = append(historyRecords, result)
			}

		case "save_user_profile":
			result, err := f.executeProfileSaveTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.profile save tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				// Profile save failures should be logged but not shown to user since it's internal
				historyRecords = append(historyRecords, fmt.Sprintf("[PROFILE_SAVE_FAILED: %s]", err.Error()))
			} else {
				// Profile save success: record in history but don't send message to user (internal operation)
				historyRecords = append(historyRecords, fmt.Sprintf("[PROFILE_SAVED: %s]", result))
			}

		case "generate_habit_prompt":
			result, err := f.executePromptGeneratorTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.prompt generator tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				// Check if this is a profile incomplete error and suggest intake
				if strings.Contains(err.Error(), "profile incomplete") {
					errorMsg := "I need to learn more about your goals first to create personalized habits. Let me start our intake process to gather some quick information."
					userMessages = append(userMessages, errorMsg)
					historyRecords = append(historyRecords, fmt.Sprintf("PROFILE_INCOMPLETE: %s - USE conduct_intake TOOL NEXT", err.Error()))
				} else {
					errorMsg := fmt.Sprintf("❌ Sorry, I couldn't generate your habit prompt: %s", err.Error())
					userMessages = append(userMessages, errorMsg)
					historyRecords = append(historyRecords, errorMsg)
				}
			} else {
				// Prompt generator success: send prompt to user and record in history
				userMessages = append(userMessages, result)
				historyRecords = append(historyRecords, result)
			}

		case "track_feedback":
			result, err := f.executeFeedbackTrackerTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.feedback tracker tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't track your feedback: %s", err.Error())
				userMessages = append(userMessages, errorMsg)
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Feedback tracker success: send summary to user and record in history
				userMessages = append(userMessages, result)
				historyRecords = append(historyRecords, result)
			}

		default:
			slog.Warn("flow.unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			errorMsg := fmt.Sprintf("❌ Sorry, I don't know how to use the tool '%s'", toolCall.Function.Name)
			userMessages = append(userMessages, errorMsg)
			historyRecords = append(historyRecords, errorMsg)
		}
	}

	// Now add all tool call results to the OpenAI conversation context
	for i, toolCall := range toolResponse.ToolCalls {
		// Find corresponding result
		var resultContent string
		if i < len(userMessages) && userMessages[i] != "" {
			resultContent = userMessages[i]
		} else {
			resultContent = "Tool executed successfully"
		}

		// Add tool result message to conversation
		messages = append(messages, openai.ToolMessage(toolCall.ID, resultContent))
	}

	// Add history records to conversation history
	for _, record := range historyRecords {
		historyMsg := ConversationMessage{
			Role:      "assistant",
			Content:   record,
			Timestamp: time.Now(),
		}
		history.Messages = append(history.Messages, historyMsg)
	}

	// Now call LLM again with the updated conversation that includes tool results
	// The LLM will see the tool calls and their results and generate a proper user-facing response
	slog.Info("flow.calling LLM again after tool execution",
		"participantID", participantID,
		"toolCount", len(toolResponse.ToolCalls),
		"messageCount", len(messages))

	finalResponse, err := f.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.failed to generate final response after tool execution", "error", err, "participantID", participantID)

		// Fallback: if LLM call fails, return the collected tool results directly
		if len(userMessages) == 1 {
			finalResponse = userMessages[0]
		} else if len(userMessages) > 1 {
			finalResponse = strings.Join(userMessages, "\n\n")
		} else {
			finalResponse = "I've completed the requested actions."
		}
	}

	// Add the LLM's final response to conversation history
	finalAssistantMsg := ConversationMessage{
		Role:      "assistant",
		Content:   finalResponse,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, finalAssistantMsg)

	// Save updated history
	err = f.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("flow.failed to save conversation history after tool execution", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	slog.Info("flow.completed tool execution with LLM-generated response",
		"participantID", participantID,
		"toolCount", len(toolResponse.ToolCalls),
		"finalResponseLength", len(finalResponse),
		"historyRecordCount", len(historyRecords))

	return finalResponse, nil
}

// transitionToState safely transitions to a new state with logging
func (f *ConversationFlow) transitionToState(ctx context.Context, participantID string, newState models.StateType) error {
	err := f.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, newState)
	if err != nil {
		slog.Error("flow.failed to transition state", "error", err, "participantID", participantID, "newState", newState)
		return fmt.Errorf("failed to transition to state %s: %w", newState, err)
	}
	slog.Info("flow.state transition", "participantID", participantID, "newState", newState)
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
		slog.Error("flow.failed to parse conversation history", "error", err, "participantID", participantID)
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
		slog.Debug("flow.trimmed history to max length", "participantID", participantID, "maxLength", maxHistoryLength)
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

	// 2.5. Add profile status information to help AI decide when to use intake
	profileStatus := f.getProfileStatus(ctx, participantID)
	if profileStatus != "" {
		messages = append(messages, openai.SystemMessage(profileStatus))
		slog.Debug("Added profile status to messages", "participantID", participantID, "profileStatus", profileStatus)
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

// getProfileStatus checks the user's profile completeness and returns status information for the AI
func (f *ConversationFlow) getProfileStatus(ctx context.Context, participantID string) string {
	// Try to get user profile
	profileJSON, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil || profileJSON == "" {
		return "PROFILE STATUS: User has no profile. IMMEDIATELY use conduct_intake tool to collect their information. DO NOT ask intake questions manually."
	}

	// Parse the profile to check completeness
	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return "PROFILE STATUS: User profile exists but has parsing issues. IMMEDIATELY use conduct_intake tool to rebuild their profile. DO NOT ask intake questions manually."
	}

	// Check required fields for habit generation
	missingFields := []string{}
	if profile.TargetBehavior == "" {
		missingFields = append(missingFields, "target behavior")
	}
	if profile.MotivationalFrame == "" {
		missingFields = append(missingFields, "motivation")
	}
	if profile.PreferredTime == "" {
		missingFields = append(missingFields, "preferred time")
	}
	if profile.PromptAnchor == "" {
		missingFields = append(missingFields, "habit anchor")
	}

	if len(missingFields) > 0 {
		return fmt.Sprintf("PROFILE STATUS: User profile is incomplete, missing: %s. IMMEDIATELY use conduct_intake tool to complete their profile. DO NOT ask intake questions manually.", strings.Join(missingFields, ", "))
	}

	return "PROFILE STATUS: User profile is complete. You can generate_habit_prompt for this user."
}

// getPreviousChatHistory retrieves and formats previous chat history for bot tools.
// Returns OpenAI messages formatted with proper roles (user/assistant) for context.
func (f *ConversationFlow) getPreviousChatHistory(ctx context.Context, participantID string, maxMessages int) ([]openai.ChatCompletionMessageParamUnion, error) {
	// Apply the configured chat history limit
	effectiveLimit := maxMessages
	if f.chatHistoryLimit >= 0 {
		// If chatHistoryLimit is 0 or positive, it overrides the maxMessages parameter
		if f.chatHistoryLimit == 0 {
			// No history should be sent
			slog.Debug("Chat history disabled by configuration", "participantID", participantID, "chatHistoryLimit", f.chatHistoryLimit)
			return []openai.ChatCompletionMessageParamUnion{}, nil
		}
		// Use the smaller of the two limits
		if f.chatHistoryLimit < maxMessages {
			effectiveLimit = f.chatHistoryLimit
		}
	}
	// If chatHistoryLimit is -1, use maxMessages as provided (no limit from config)

	// Get conversation history
	history, err := f.getConversationHistory(ctx, participantID)
	if err != nil {
		slog.Warn("Failed to get conversation history for bot tool", "error", err, "participantID", participantID)
		return []openai.ChatCompletionMessageParamUnion{}, nil // Return empty array instead of failing
	}

	// Limit history to prevent token overflow
	historyMessages := history.Messages
	if len(historyMessages) > effectiveLimit && effectiveLimit > 0 {
		historyMessages = historyMessages[len(historyMessages)-effectiveLimit:]
	}

	// Convert to OpenAI message format
	var messages []openai.ChatCompletionMessageParamUnion
	for _, msg := range historyMessages {
		if msg.Role == "user" {
			messages = append(messages, openai.UserMessage(msg.Content))
		} else if msg.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	slog.Debug("Retrieved previous chat history for bot tool",
		"participantID", participantID,
		"totalHistoryMessages", len(history.Messages),
		"includedMessages", len(messages),
		"requestedMaxMessages", maxMessages,
		"configuredLimit", f.chatHistoryLimit,
		"effectiveLimit", effectiveLimit)

	return messages, nil
}

// executeSchedulerTool executes a scheduler tool call.
func (f *ConversationFlow) executeSchedulerTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (*models.ToolResult, error) {
	// Log the raw tool call for debugging
	slog.Debug("flow.executeSchedulerTool raw call",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"functionName", toolCall.Function.Name,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the scheduler parameters from the tool call
	var params models.SchedulerToolParams
	if err := json.Unmarshal(toolCall.Function.Arguments, &params); err != nil {
		slog.Error("flow.failed to parse scheduler parameters",
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
	slog.Debug("flow.parsed scheduler parameters",
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
	slog.Debug("flow.auto-detection check",
		"participantID", participantID,
		"typeIsEmpty", params.Type == "",
		"typeValue", string(params.Type),
		"fixedTimeProvided", params.FixedTime != "",
		"fixedTimeValue", params.FixedTime,
		"randomStartProvided", params.RandomStartTime != "",
		"randomEndProvided", params.RandomEndTime != "")

	if params.Type == "" {
		slog.Debug("flow.type field is empty, attempting auto-detection", "participantID", participantID)

		if params.FixedTime != "" {
			slog.Debug("flow.auto-detecting fixed type", "participantID", participantID, "fixedTime", params.FixedTime)
			params.Type = models.SchedulerTypeFixed
			slog.Info("flow.auto-detected scheduler type as 'fixed'",
				"participantID", participantID,
				"reason", "fixed_time provided",
				"newType", string(params.Type))
		} else if params.RandomStartTime != "" && params.RandomEndTime != "" {
			slog.Debug("flow.auto-detecting random type", "participantID", participantID)
			params.Type = models.SchedulerTypeRandom
			slog.Info("flow.auto-detected scheduler type as 'random'",
				"participantID", participantID,
				"reason", "random start and end times provided",
				"newType", string(params.Type))
		} else {
			slog.Error("flow.cannot determine scheduler type",
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
		slog.Debug("flow.corrected scheduler parameters",
			"participantID", participantID,
			"correctedType", string(params.Type))
	} else {
		slog.Debug("flow.type field already provided",
			"participantID", participantID,
			"typeValue", string(params.Type))
	}

	// Check if phone number is available in context
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("flow.scheduler tool context check",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber)

	// Final validation log before executing scheduler tool
	slog.Debug("flow.final parameters before scheduler execution",
		"participantID", participantID,
		"finalType", string(params.Type),
		"finalTypeIsEmpty", params.Type == "",
		"fixedTime", params.FixedTime)

	// Execute the scheduler tool
	return f.schedulerTool.ExecuteScheduler(ctx, participantID, params)
}

// executeInterventionTool executes an intervention tool call.
func (f *ConversationFlow) executeInterventionTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (*models.ToolResult, error) {
	// Log the raw tool call for debugging
	slog.Debug("flow.executeInterventionTool raw call",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"functionName", toolCall.Function.Name,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the intervention parameters from the tool call
	var params models.OneMinuteInterventionToolParams
	if err := json.Unmarshal(toolCall.Function.Arguments, &params); err != nil {
		slog.Error("flow.failed to parse intervention parameters",
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
	slog.Debug("flow.parsed intervention parameters",
		"participantID", participantID,
		"params", fmt.Sprintf("%+v", params))

	// Get phone number from context
	phoneNumber, ok := ctx.Value(phoneNumberContextKey).(string)
	slog.Debug("flow.intervention tool context check",
		"participantID", participantID,
		"hasPhoneNumber", ok,
		"phoneNumber", phoneNumber,
		"contextValue", ctx.Value(phoneNumberContextKey))

	if !ok || phoneNumber == "" {
		slog.Error("flow.intervention tool missing phone number",
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

// executeIntakeBotTool executes an intake bot tool call.
func (f *ConversationFlow) executeIntakeBotTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (string, error) {
	slog.Debug("flow.executeIntakeBotTool",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal(toolCall.Function.Arguments, &args); err != nil {
		slog.Error("flow.failed to parse intake bot parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return "", fmt.Errorf("failed to unmarshal intake bot parameters: %w", err)
	}

	// Get previous chat history for context (configurable limit, default fallback for safety)
	chatHistory, err := f.getPreviousChatHistory(ctx, participantID, 50)
	if err != nil {
		slog.Warn("flow.failed to get chat history for intake bot", "error", err, "participantID", participantID)
		// Continue without history rather than failing
		chatHistory = []openai.ChatCompletionMessageParamUnion{}
	}

	// Execute the intake bot tool with conversation history
	return f.intakeBotTool.ExecuteIntakeBotWithHistory(ctx, participantID, args, chatHistory)
}

// executePromptGeneratorTool executes a prompt generator tool call.
func (f *ConversationFlow) executePromptGeneratorTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (string, error) {
	slog.Debug("flow.executePromptGeneratorTool",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal(toolCall.Function.Arguments, &args); err != nil {
		slog.Error("flow.failed to parse prompt generator parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return "", fmt.Errorf("failed to unmarshal prompt generator parameters: %w", err)
	}

	// Get previous chat history for context (configurable limit, default fallback for safety)
	chatHistory, err := f.getPreviousChatHistory(ctx, participantID, 50)
	if err != nil {
		slog.Warn("flow.failed to get chat history for prompt generator", "error", err, "participantID", participantID)
		// Continue without history rather than failing
		chatHistory = []openai.ChatCompletionMessageParamUnion{}
	}

	// Execute the prompt generator tool with conversation history
	result, err := f.promptGeneratorTool.ExecutePromptGeneratorWithHistory(ctx, participantID, args, chatHistory)
	if err != nil {
		return result, err
	}

	// After successful prompt generation, schedule automatic feedback collection
	if f.feedbackTrackerTool != nil {
		if scheduleErr := f.feedbackTrackerTool.ScheduleFeedbackCollection(ctx, participantID); scheduleErr != nil {
			slog.Warn("flow.executePromptGeneratorTool: failed to schedule feedback collection", "participantID", participantID, "error", scheduleErr)
			// Don't fail the prompt generation, just log the warning
		} else {
			slog.Info("flow.executePromptGeneratorTool: feedback collection scheduled", "participantID", participantID)
		}
	}

	return result, err
}

// executeFeedbackTrackerTool executes a feedback tracker tool call.
func (f *ConversationFlow) executeFeedbackTrackerTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (string, error) {
	slog.Debug("flow.executeFeedbackTrackerTool",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal(toolCall.Function.Arguments, &args); err != nil {
		slog.Error("flow.failed to parse feedback tracker parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return "", fmt.Errorf("failed to unmarshal feedback tracker parameters: %w", err)
	}

	// Get previous chat history for context (configurable limit, default fallback for safety)
	chatHistory, err := f.getPreviousChatHistory(ctx, participantID, 50)
	if err != nil {
		slog.Warn("flow.failed to get chat history for feedback tracker", "error", err, "participantID", participantID)
		// Continue without history rather than failing
		chatHistory = []openai.ChatCompletionMessageParamUnion{}
	}

	// Execute the feedback tracker tool with conversation history
	result, err := f.feedbackTrackerTool.ExecuteFeedbackTrackerWithHistory(ctx, participantID, args, chatHistory)
	if err != nil {
		return result, err
	}

	// Cancel any pending feedback timers since feedback was received
	if f.feedbackTrackerTool != nil {
		f.feedbackTrackerTool.CancelPendingFeedback(ctx, participantID)
		slog.Debug("flow.executeFeedbackTrackerTool: cancelled pending feedback timers", "participantID", participantID)
	}

	return result, err
}

// executeProfileSaveTool executes a profile save tool call.
func (f *ConversationFlow) executeProfileSaveTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (string, error) {
	slog.Debug("flow.executeProfileSaveTool",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal(toolCall.Function.Arguments, &args); err != nil {
		slog.Error("flow.failed to parse profile save parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return "", fmt.Errorf("failed to unmarshal profile save parameters: %w", err)
	}

	// Execute the profile save tool
	return f.intakeBotTool.ExecuteProfileSave(ctx, participantID, args)
}

// SetChatHistoryLimit sets the limit for number of history messages sent to bot tools.
// -1: no limit, 0: no history, positive: limit to last N messages
func (f *ConversationFlow) SetChatHistoryLimit(limit int) {
	f.chatHistoryLimit = limit
	slog.Debug("ConversationFlow: chat history limit set", "limit", limit)
}
