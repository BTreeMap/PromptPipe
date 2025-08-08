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
	"github.com/openai/openai-go/packages/param"
)

// Context key for storing phone number in context
type contextKey string

const phoneNumberContextKey contextKey = "phone_number"
const debugModeContextKey contextKey = "debug_mode"

// GetPhoneNumberContextKey returns the context key used for storing phone numbers
func GetPhoneNumberContextKey() contextKey {
	return phoneNumberContextKey
}

// GetPhoneNumberFromContext retrieves the phone number from the context
func GetPhoneNumberFromContext(ctx context.Context) (string, bool) {
	phoneNumber, ok := ctx.Value(phoneNumberContextKey).(string)
	return phoneNumber, ok && phoneNumber != ""
}

// SetDebugModeInContext adds debug mode to the context
func SetDebugModeInContext(ctx context.Context, debugMode bool) context.Context {
	return context.WithValue(ctx, debugModeContextKey, debugMode)
}

// GetDebugModeFromContext retrieves debug mode from the context
func GetDebugModeFromContext(ctx context.Context) bool {
	debugMode, ok := ctx.Value(debugModeContextKey).(bool)
	return ok && debugMode
}

// SendDebugMessageIfEnabled sends a debug message if debug mode is enabled in context
func SendDebugMessageIfEnabled(ctx context.Context, participantID string, msgService MessagingService, message string) {
	if !GetDebugModeFromContext(ctx) || msgService == nil {
		return
	}

	// Get phone number from context - if not available, we can't send debug message
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	if !hasPhone || phoneNumber == "" {
		slog.Debug("SendDebugMessageIfEnabled: no phone number in context, skipping debug message", "participantID", participantID)
		return
	}

	// Format the debug message
	debugMsg := fmt.Sprintf("🐛 DEBUG: %s", message)

	// Send the debug message (don't fail if it doesn't work)
	if err := msgService.SendMessage(ctx, phoneNumber, debugMsg); err != nil {
		slog.Warn("SendDebugMessageIfEnabled: failed to send debug message",
			"participantID", participantID,
			"phoneNumber", phoneNumber,
			"error", err)
	}
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
	stateManager        StateManager
	genaiClient         genai.ClientInterface
	systemPrompt        string
	systemPromptFile    string
	chatHistoryLimit    int                  // Limit for number of history messages sent to bot tools (-1: no limit, 0: no history, positive: limit to last N messages)
	schedulerTool       *SchedulerTool       // Tool for scheduling daily prompts
	coordinatorModule   *CoordinatorModule   // Module for coordinator conversations and routing
	intakeModule        *IntakeModule        // Module for conducting intake conversations
	promptGeneratorTool *PromptGeneratorTool // Tool for generating personalized habit prompts
	feedbackModule      *FeedbackModule      // Module for tracking user feedback and updating profiles
	stateTransitionTool *StateTransitionTool // Tool for managing conversation state transitions
	profileSaveTool     *ProfileSaveTool     // Tool for saving user profiles (shared across modules)
	debugMode           bool                 // Enable debug mode for user-facing debug messages
	msgService          MessagingService     // Messaging service for sending debug messages
} // NewConversationFlow creates a new conversation flow with dependencies.
func NewConversationFlow(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string) *ConversationFlow {
	slog.Debug("ConversationFlow.NewConversationFlow: creating flow with dependencies", "systemPromptFile", systemPromptFile)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
		chatHistoryLimit: -1, // Default: no limit
	}
}

// NewConversationFlowWithScheduler creates a new conversation flow with scheduler tool support.
func NewConversationFlowWithScheduler(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool) *ConversationFlow {
	slog.Debug("ConversationFlow.NewConversationFlowWithScheduler: creating flow with scheduler tool", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasSchedulerTool", schedulerTool != nil)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
		chatHistoryLimit: -1, // Default: no limit
		schedulerTool:    schedulerTool,
	}
}

// NewConversationFlowWithAllTools creates a new conversation flow with all tools for the 3-bot architecture.
func NewConversationFlowWithAllTools(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, msgService MessagingService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile string) *ConversationFlow {
	return NewConversationFlowWithAllToolsAndTimeouts(stateManager, genaiClient, systemPromptFile, msgService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile, "15m", "3h")
}

// NewConversationFlowWithAllToolsAndTimeouts creates a new conversation flow with all tools and configurable feedback timeouts for the 3-bot architecture.
func NewConversationFlowWithAllToolsAndTimeouts(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, msgService MessagingService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile, feedbackInitialTimeout, feedbackFollowupDelay string) *ConversationFlow {
	slog.Debug("ConversationFlow.NewConversationFlowWithAllToolsAndTimeouts: creating flow with all tools and timeouts", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "intakeBotPromptFile", intakeBotPromptFile, "promptGeneratorPromptFile", promptGeneratorPromptFile, "feedbackTrackerPromptFile", feedbackTrackerPromptFile, "feedbackInitialTimeout", feedbackInitialTimeout, "feedbackFollowupDelay", feedbackFollowupDelay)

	// Create timer for scheduler
	timer := NewSimpleTimer()

	// Create shared tools
	schedulerTool := NewSchedulerToolWithGenAI(timer, msgService, genaiClient)
	stateTransitionTool := NewStateTransitionTool(stateManager, timer)
	profileSaveTool := NewProfileSaveTool(stateManager)
	promptGeneratorTool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, promptGeneratorPromptFile)

	// Create modules with shared tools
	coordinatorModule := NewCoordinatorModule(stateManager, genaiClient, msgService, systemPromptFile, schedulerTool, promptGeneratorTool, stateTransitionTool, profileSaveTool)
	intakeModule := NewIntakeModule(stateManager, genaiClient, msgService, intakeBotPromptFile, stateTransitionTool, profileSaveTool, schedulerTool)
	feedbackModule := NewFeedbackModuleWithTimeouts(stateManager, genaiClient, feedbackTrackerPromptFile, timer, msgService, feedbackInitialTimeout, feedbackFollowupDelay, stateTransitionTool, profileSaveTool, schedulerTool)

	return &ConversationFlow{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		systemPromptFile:    systemPromptFile,
		chatHistoryLimit:    -1, // Default: no limit
		schedulerTool:       schedulerTool,
		coordinatorModule:   coordinatorModule,
		intakeModule:        intakeModule,
		promptGeneratorTool: promptGeneratorTool,
		feedbackModule:      feedbackModule,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
		msgService:          msgService,
	}
}

// SetDependencies injects dependencies into the flow.
func (f *ConversationFlow) SetDependencies(deps Dependencies) {
	slog.Debug("ConversationFlow.SetDependencies: injecting dependencies")
	f.stateManager = deps.StateManager
	// Note: genaiClient needs to be set separately as it's not part of standard Dependencies
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (f *ConversationFlow) LoadSystemPrompt() error {
	slog.Debug("ConversationFlow.LoadSystemPrompt: loading system prompt from file", "file", f.systemPromptFile)

	if f.systemPromptFile == "" {
		slog.Error("ConversationFlow.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(f.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("ConversationFlow.LoadSystemPrompt: system prompt file does not exist", "file", f.systemPromptFile)
		return fmt.Errorf("system prompt file does not exist: %s", f.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(f.systemPromptFile)
	if err != nil {
		slog.Error("ConversationFlow.LoadSystemPrompt: failed to read system prompt file", "file", f.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read system prompt file: %w", err)
	}

	f.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("ConversationFlow.LoadSystemPrompt: system prompt loaded successfully", "file", f.systemPromptFile, "length", len(f.systemPrompt))
	return nil
}

// LoadToolSystemPrompts loads system prompts for all modules.
func (f *ConversationFlow) LoadToolSystemPrompts() error {
	slog.Debug("flow.LoadToolSystemPrompts: loading module system prompts")

	// Load coordinator module system prompt
	if f.coordinatorModule != nil {
		if err := f.coordinatorModule.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load coordinator module system prompt", "error", err)
			// Continue even if coordinator module prompt fails to load
		}
	}

	// Load intake module system prompt
	if f.intakeModule != nil {
		if err := f.intakeModule.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load intake module system prompt", "error", err)
			// Continue even if intake module prompt fails to load
		}
	}

	// Load prompt generator system prompt
	if f.promptGeneratorTool != nil {
		if err := f.promptGeneratorTool.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load prompt generator system prompt", "error", err)
			// Continue even if prompt generator prompt fails to load
		}
	}

	// Load feedback module system prompt
	if f.feedbackModule != nil {
		if err := f.feedbackModule.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load feedback module system prompt", "error", err)
			// Continue even if feedback module prompt fails to load
		}
	}

	slog.Info("flow.LoadToolSystemPrompts: module system prompts loaded")
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
	// Add debug mode to context
	ctx = SetDebugModeInContext(ctx, f.debugMode)

	// Log context information for debugging
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	slog.Debug("flow.ProcessResponse: checking context",
		"participantID", participantID,
		"hasPhoneNumber", hasPhone,
		"phoneNumber", phoneNumber,
		"debugMode", f.debugMode,
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
		slog.Error("ConversationFlow.processConversationMessage: failed to get conversation history", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Add user message to history
	userMsg := ConversationMessage{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, userMsg)

	// Get current conversation state to determine routing
	conversationState, err := f.getCurrentConversationState(ctx, participantID)
	if err != nil {
		slog.Error("ConversationFlow.processConversationMessage: failed to get conversation state", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get conversation state: %w", err)
	}

	slog.Debug("ConversationFlow.processConversationMessage: routing based on state",
		"participantID", participantID, "conversationState", conversationState)

	// Route to appropriate state handler
	var response string
	switch conversationState {
	case models.StateCoordinator:
		response, err = f.processCoordinatorState(ctx, participantID, userMessage, history)
	case models.StateIntake:
		response, err = f.processIntakeState(ctx, participantID, userMessage, history)
	case models.StateFeedback:
		response, err = f.processFeedbackState(ctx, participantID, userMessage, history)
	default:
		// Unknown state - fallback to coordinator
		slog.Warn("ConversationFlow.processConversationMessage: unknown conversation state, falling back to coordinator",
			"conversationState", conversationState, "participantID", participantID)
		response, err = f.processCoordinatorState(ctx, participantID, userMessage, history)
	}

	if err != nil {
		slog.Error("ConversationFlow.processConversationMessage: state handler failed",
			"error", err, "conversationState", conversationState, "participantID", participantID)
		return "", fmt.Errorf("state handler failed: %w", err)
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
		slog.Error("ConversationFlow.processConversationMessage: failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	// Return the AI response for sending
	slog.Info("ConversationFlow.processConversationMessage: generated response", "participantID", participantID, "responseLength", len(response))

	// Send separate debug message if debug mode is enabled
	if f.debugMode {
		f.sendDebugInfo(ctx, participantID, conversationState)
	}

	return response, nil
}

// handleToolCalls processes tool calls from the AI and executes them.
func (f *ConversationFlow) handleToolCalls(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, history *ConversationHistory, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (string, error) {
	// Log and debug the exact tools being executed
	var executingToolNames []string
	for _, toolCall := range toolResponse.ToolCalls {
		executingToolNames = append(executingToolNames, toolCall.Function.Name)
	}
	slog.Info("ConversationFlow.handleToolCalls: executing tools",
		"participantID", participantID,
		"toolCallCount", len(toolResponse.ToolCalls),
		"executingTools", executingToolNames)

	// Send debug message if debug mode is enabled
	debugMessage := fmt.Sprintf("Coordinator executing tools: %s", strings.Join(executingToolNames, ", "))
	SendDebugMessageIfEnabled(ctx, participantID, f.msgService, debugMessage) // Create the tool calls in OpenAI format first
	var toolCalls []openai.ChatCompletionMessageToolCallParam
	for _, toolCall := range toolResponse.ToolCalls {
		toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
			ID:   toolCall.ID,
			Type: "function",
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      toolCall.Function.Name,
				Arguments: string(toolCall.Function.Arguments),
			},
		})
	}

	// Create assistant message with both content and tool calls for OpenAI API
	assistantMessageWithToolCalls := openai.ChatCompletionAssistantMessageParam{
		Content: openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: param.NewOpt(toolResponse.Content),
		},
		ToolCalls: toolCalls,
	}

	// Add this assistant message with tool calls to the OpenAI conversation context
	// This is CRITICAL - OpenAI needs to see the assistant message with tool_calls
	// before the tool result messages that reference those tool_call_ids
	messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMessageWithToolCalls})

	// Also add assistant message with tool calls to our internal conversation history
	// Note: We store a simplified version in history since we don't need the full OpenAI format there
	assistantMsg := ConversationMessage{
		Role:      "assistant",
		Content:   toolResponse.Content,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	// Collect tool call results - use arrays sized to match tool calls to ensure proper alignment
	toolResults := make([]string, len(toolResponse.ToolCalls))
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
				toolResults[i] = errorMsg
				historyRecords = append(historyRecords, errorMsg)
			} else if !result.Success {
				slog.Warn("flow.scheduler tool returned error", "error", result.Error, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ %s", result.Message)
				toolResults[i] = errorMsg
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Scheduler success: send message to user and record in history
				toolResults[i] = result.Message
				historyRecords = append(historyRecords, result.Message)
			}

		case "generate_habit_prompt":
			result, err := f.executePromptGeneratorTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.prompt generator tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				// Check if this is a profile incomplete error and suggest state transition
				if strings.Contains(err.Error(), "profile incomplete") {
					errorMsg := "I need to learn more about your goals first to create personalized habits. Let me transition you to our intake process."
					toolResults[i] = errorMsg
					historyRecords = append(historyRecords, fmt.Sprintf("PROFILE_INCOMPLETE: %s - USE transition_state to INTAKE NEXT", err.Error()))
				} else {
					errorMsg := fmt.Sprintf("❌ Sorry, I couldn't generate your habit prompt: %s", err.Error())
					toolResults[i] = errorMsg
					historyRecords = append(historyRecords, errorMsg)
				}
			} else {
				// Prompt generator success: send prompt to user and record in history
				toolResults[i] = result
				historyRecords = append(historyRecords, result)
			}

		case "transition_state":
			result, err := f.executeStateTransitionTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.state transition tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				// State transitions are internal operations - log but don't show user detailed errors
				toolResults[i] = "State transition failed"
				historyRecords = append(historyRecords, fmt.Sprintf("[STATE_TRANSITION_FAILED: %s]", err.Error()))
			} else {
				// State transition success: record in history but use generic message for LLM
				toolResults[i] = "State transition completed"
				historyRecords = append(historyRecords, fmt.Sprintf("[STATE_TRANSITION: %s]", result))
			}

		case "save_user_profile":
			result, err := f.executeProfileSaveTool(ctx, participantID, toolCall)
			if err != nil {
				slog.Error("flow.profile save tool execution failed", "error", err, "participantID", participantID, "toolCallID", toolCall.ID)
				errorMsg := fmt.Sprintf("❌ Sorry, I couldn't save your profile: %s", err.Error())
				toolResults[i] = errorMsg
				historyRecords = append(historyRecords, errorMsg)
			} else {
				// Profile save success: record in history
				toolResults[i] = result
				historyRecords = append(historyRecords, result)
			}

		default:
			slog.Warn("flow.unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			errorMsg := fmt.Sprintf("❌ Sorry, I don't know how to use the tool '%s'", toolCall.Function.Name)
			toolResults[i] = errorMsg
			historyRecords = append(historyRecords, errorMsg)
		}
	}

	// Now add all tool call results to the OpenAI conversation context
	for i, toolCall := range toolResponse.ToolCalls {
		// Use the corresponding result from our toolResults array
		resultContent := toolResults[i]
		if resultContent == "" {
			resultContent = "Tool executed successfully"
		}

		// Add tool result message to conversation
		messages = append(messages, openai.ToolMessage(resultContent, toolCall.ID))
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
		var nonEmptyResults []string
		for _, result := range toolResults {
			if result != "" {
				nonEmptyResults = append(nonEmptyResults, result)
			}
		}

		if len(nonEmptyResults) == 1 {
			finalResponse = nonEmptyResults[0]
		} else if len(nonEmptyResults) > 1 {
			finalResponse = strings.Join(nonEmptyResults, "\n\n")
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
		slog.Error("ConversationFlow.continueAfterToolExecution: failed to save conversation history after tool execution", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	slog.Info("ConversationFlow.continueAfterToolExecution: completed tool execution with LLM-generated response",
		"participantID", participantID,
		"toolCount", len(toolResponse.ToolCalls),
		"finalResponseLength", len(finalResponse),
		"historyRecordCount", len(historyRecords))

	// Send separate debug message if debug mode is enabled
	if f.debugMode {
		// Collect the tool names that were executed
		var executedTools []string
		for _, toolCall := range toolResponse.ToolCalls {
			executedTools = append(executedTools, toolCall.Function.Name)
		}
		debugMessage := fmt.Sprintf("Executed tools: %s - Response length: %d",
			strings.Join(executedTools, ", "), len(finalResponse))
		f.sendDebugMessage(ctx, participantID, debugMessage)
	}

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
		return "PROFILE STATUS: User has no profile. Use transition_state to INTAKE to collect their information. DO NOT ask intake questions manually."
	}

	// Parse the profile to check completeness
	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return "PROFILE STATUS: User profile exists but has parsing issues. Use transition_state to INTAKE to rebuild their profile. DO NOT ask intake questions manually."
	}

	// Check required fields for habit generation
	missingFields := []string{}
	if profile.HabitDomain == "" {
		missingFields = append(missingFields, "habit domain")
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
		return fmt.Sprintf("PROFILE STATUS: User profile is incomplete, missing: %s. Use transition_state to INTAKE to complete their profile. DO NOT ask intake questions manually.", strings.Join(missingFields, ", "))
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
	if f.feedbackModule != nil {
		if scheduleErr := f.feedbackModule.ScheduleFeedbackCollection(ctx, participantID); scheduleErr != nil {
			slog.Warn("flow.executePromptGeneratorTool: failed to schedule feedback collection", "participantID", participantID, "error", scheduleErr)
			// Don't fail the prompt generation, just log the warning
		} else {
			slog.Info("flow.executePromptGeneratorTool: feedback collection scheduled", "participantID", participantID)
		}
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
	return f.profileSaveTool.ExecuteProfileSave(ctx, participantID, args)
}

// executeStateTransitionTool executes a state transition tool call.
func (f *ConversationFlow) executeStateTransitionTool(ctx context.Context, participantID string, toolCall genai.ToolCall) (string, error) {
	slog.Debug("flow.executeStateTransitionTool",
		"participantID", participantID,
		"toolCallID", toolCall.ID,
		"rawArguments", string(toolCall.Function.Arguments))

	// Parse the tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal(toolCall.Function.Arguments, &args); err != nil {
		slog.Error("flow.failed to parse state transition parameters",
			"error", err,
			"participantID", participantID,
			"rawArguments", string(toolCall.Function.Arguments))
		return "", fmt.Errorf("failed to unmarshal state transition parameters: %w", err)
	}

	// Execute the state transition tool
	return f.stateTransitionTool.ExecuteStateTransition(ctx, participantID, args)
}

// SetChatHistoryLimit sets the limit for number of history messages sent to bot tools.
// -1: no limit, 0: no history, positive: limit to last N messages
func (f *ConversationFlow) SetChatHistoryLimit(limit int) {
	f.chatHistoryLimit = limit
	slog.Debug("ConversationFlow: chat history limit set", "limit", limit)
}

// SetDebugMode enables or disables debug mode for user-facing debug messages.
func (f *ConversationFlow) SetDebugMode(enabled bool) {
	f.debugMode = enabled
	slog.Debug("ConversationFlow: debug mode set", "enabled", enabled)
}

// sendDebugMessage sends a debug message to the user if debug mode is enabled.
func (f *ConversationFlow) sendDebugMessage(ctx context.Context, participantID, message string) {
	if !f.debugMode || f.msgService == nil {
		return
	}

	// Get phone number from context to send the debug message
	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	if !hasPhone || phoneNumber == "" {
		slog.Debug("ConversationFlow.sendDebugMessage: no phone number in context", "participantID", participantID)
		return
	}

	debugMsg := fmt.Sprintf("🐛 DEBUG: %s", message)
	err := f.msgService.SendMessage(ctx, phoneNumber, debugMsg)
	if err != nil {
		slog.Error("ConversationFlow.sendDebugMessage: failed to send debug message", "error", err, "participantID", participantID)
	} else {
		slog.Debug("ConversationFlow.sendDebugMessage: sent debug message", "participantID", participantID, "message", message)
	}
}

// sendDebugInfo sends comprehensive debug information as a separate message if debug mode is enabled.
func (f *ConversationFlow) sendDebugInfo(ctx context.Context, participantID string, conversationState models.StateType) {
	if !f.debugMode {
		return
	}

	debugInfo := f.buildDebugInfo(ctx, participantID, conversationState)
	if debugInfo != "" {
		f.sendDebugMessage(ctx, participantID, debugInfo)
	}
}

// buildDebugInfo creates comprehensive debug information as a single message.
func (f *ConversationFlow) buildDebugInfo(ctx context.Context, participantID string, conversationState models.StateType) string {
	if !f.debugMode {
		return ""
	}

	var debugParts []string

	// Add header
	debugParts = append(debugParts, "DEBUG INFO:")

	// Add current state information
	debugParts = append(debugParts, fmt.Sprintf("Current State: %s", conversationState))

	// Add user profile information
	if f.profileSaveTool != nil {
		profile, err := f.profileSaveTool.GetOrCreateUserProfile(ctx, participantID)
		if err != nil {
			debugParts = append(debugParts, fmt.Sprintf("Profile: Error retrieving (%s)", err.Error()))
		} else {
			profileSummary := fmt.Sprintf("Profile: Domain=%s, Motivation=%s, Time=%s, Anchor=%s, Success=%d/%d",
				profile.HabitDomain,
				profile.MotivationalFrame,
				profile.PreferredTime,
				profile.PromptAnchor,
				profile.SuccessCount,
				profile.TotalPrompts)
			debugParts = append(debugParts, profileSummary)
		}
	}

	// Add available modules/tools with their exact function names
	var availableModules []string
	var toolDetails []string

	if f.coordinatorModule != nil {
		availableModules = append(availableModules, "CoordinatorModule")
		toolDetails = append(toolDetails, "CoordinatorModule (state transitions, profile saving, prompt generation, scheduling)")
	}
	if f.intakeModule != nil {
		availableModules = append(availableModules, "IntakeModule")
		toolDetails = append(toolDetails, "IntakeModule (state transitions, profile saving, scheduling)")
	}
	if f.feedbackModule != nil {
		availableModules = append(availableModules, "FeedbackModule")
		toolDetails = append(toolDetails, "FeedbackModule (state transitions, profile saving, scheduling)")
	}
	if f.schedulerTool != nil {
		availableModules = append(availableModules, "SchedulerTool")
		toolDetails = append(toolDetails, "SchedulerTool → scheduler")
	}
	if f.promptGeneratorTool != nil {
		availableModules = append(availableModules, "PromptGeneratorTool")
		toolDetails = append(toolDetails, "PromptGeneratorTool → generate_habit_prompt")
	}
	if f.stateTransitionTool != nil {
		availableModules = append(availableModules, "StateTransitionTool")
		toolDetails = append(toolDetails, "StateTransitionTool → transition_state")
	}
	if f.profileSaveTool != nil {
		availableModules = append(availableModules, "ProfileSaveTool")
		toolDetails = append(toolDetails, "ProfileSaveTool → save_user_profile")
	}

	if len(availableModules) > 0 {
		debugParts = append(debugParts, fmt.Sprintf("Available Tools: %s", strings.Join(availableModules, ", ")))
		debugParts = append(debugParts, fmt.Sprintf("Tool Details: %s", strings.Join(toolDetails, " | ")))
	}

	return strings.Join(debugParts, "\n")
}

// getCurrentConversationState retrieves the current conversation state for a participant.
func (f *ConversationFlow) getCurrentConversationState(ctx context.Context, participantID string) (models.StateType, error) {
	if f.stateManager == nil {
		return models.StateCoordinator, nil // Default to coordinator if no state manager
	}

	stateStr, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyConversationState)
	if err != nil {
		return "", err
	}

	// Default to COORDINATOR if no state is set
	if stateStr == "" {
		return models.StateCoordinator, nil
	}

	return models.StateType(stateStr), nil
}

// processCoordinatorState handles messages when in the coordinator state.
// The coordinator decides which tools to use and can transition to other states.
func (f *ConversationFlow) processCoordinatorState(ctx context.Context, participantID, userMessage string, history *ConversationHistory) (string, error) {
	slog.Debug("ConversationFlow.processCoordinatorState: delegating to coordinator module",
		"participantID", participantID)

	if f.coordinatorModule == nil {
		return "", fmt.Errorf("coordinator module not available")
	}

	// Convert conversation history to OpenAI format
	chatHistory, err := f.buildOpenAIMessages(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow.processCoordinatorState: failed to build chat history", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build chat history: %w", err)
	}

	// Remove the system prompt from chat history since coordinator module will add its own
	if len(chatHistory) > 0 {
		// Check if first message is system message and remove it
		if msg := chatHistory[0]; msg.OfSystem != nil {
			chatHistory = chatHistory[1:]
		}
	}

	// Delegate to coordinator module
	return f.coordinatorModule.ProcessMessage(ctx, participantID, userMessage, chatHistory)
}

// processIntakeState handles messages when in the intake state.
// The intake module directly processes the conversation without using tools.
func (f *ConversationFlow) processIntakeState(ctx context.Context, participantID, userMessage string, history *ConversationHistory) (string, error) {
	slog.Debug("ConversationFlow.processIntakeState: processing intake message",
		"participantID", participantID)

	if f.intakeModule == nil {
		return "", fmt.Errorf("intake module not available")
	}

	// Get previous chat history for context
	chatHistory, err := f.getPreviousChatHistory(ctx, participantID, 50)
	if err != nil {
		slog.Warn("flow.failed to get chat history for intake", "error", err, "participantID", participantID)
		chatHistory = []openai.ChatCompletionMessageParamUnion{}
	}

	// Execute the intake module directly with conversation history
	args := map[string]interface{}{
		"user_response": userMessage,
	}

	response, err := f.intakeModule.ExecuteIntakeBotWithHistory(ctx, participantID, args, chatHistory)
	if err != nil {
		slog.Error("ConversationFlow.processIntakeState: intake execution failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("intake processing failed: %w", err)
	}

	// Send separate debug message if debug mode is enabled
	if f.debugMode {
		f.sendDebugMessage(ctx, participantID, fmt.Sprintf("Executed IntakeModule.ExecuteIntakeBotWithHistory() - Response length: %d", len(response)))
	}

	return response, nil
}

// processFeedbackState handles messages when in the feedback state.
// The feedback tracker module directly processes the conversation without using tools.
func (f *ConversationFlow) processFeedbackState(ctx context.Context, participantID, userMessage string, history *ConversationHistory) (string, error) {
	slog.Debug("ConversationFlow.processFeedbackState: processing feedback message",
		"participantID", participantID)

	if f.feedbackModule == nil {
		return "", fmt.Errorf("feedback module not available")
	}

	// Get previous chat history for context
	chatHistory, err := f.getPreviousChatHistory(ctx, participantID, 50)
	if err != nil {
		slog.Warn("flow.failed to get chat history for feedback", "error", err, "participantID", participantID)
		chatHistory = []openai.ChatCompletionMessageParamUnion{}
	}

	// Execute the feedback tracker directly with conversation history
	args := map[string]interface{}{
		"user_response":     userMessage,
		"completion_status": "attempted", // Default, the feedback tracker will analyze the actual response
	}

	response, err := f.feedbackModule.ExecuteFeedbackTrackerWithHistory(ctx, participantID, args, chatHistory)
	if err != nil {
		slog.Error("ConversationFlow.processFeedbackState: feedback execution failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("feedback processing failed: %w", err)
	}

	// Send separate debug message if debug mode is enabled
	if f.debugMode {
		f.sendDebugMessage(ctx, participantID, fmt.Sprintf("Executed FeedbackModule.ExecuteFeedbackTrackerWithHistory() - Response length: %d", len(response)))
	}

	// Cancel any pending feedback timers since feedback was received
	if f.feedbackModule != nil {
		f.feedbackModule.CancelPendingFeedback(ctx, participantID)
		slog.Debug("ConversationFlow.processFeedbackState: cancelled pending feedback timers", "participantID", participantID)
	}

	return response, nil
}
