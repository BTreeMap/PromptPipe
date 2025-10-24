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
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"github.com/openai/openai-go"
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
	Role      string    `json:"role"`      // "user", "assistant", or "system"
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

// NewConversationFlowWithAllTools creates a new conversation flow with all tools and configurable feedback timeouts for the 3-bot architecture.
func NewConversationFlowWithAllTools(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, msgService MessagingService, intakeBotPromptFile, promptGeneratorPromptFile, feedbackTrackerPromptFile, feedbackInitialTimeout, feedbackFollowupDelay string, schedulerPrepTimeMinutes int, autoFeedbackAfterPromptEnabled bool) *ConversationFlow {
	slog.Debug("ConversationFlow.NewConversationFlowWithAllTools: creating flow with all tools and timeouts", "systemPromptFile", systemPromptFile, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "intakeBotPromptFile", intakeBotPromptFile, "promptGeneratorPromptFile", promptGeneratorPromptFile, "feedbackTrackerPromptFile", feedbackTrackerPromptFile, "feedbackInitialTimeout", feedbackInitialTimeout, "feedbackFollowupDelay", feedbackFollowupDelay, "schedulerPrepTimeMinutes", schedulerPrepTimeMinutes, "autoFeedbackAfterPromptEnabled", autoFeedbackAfterPromptEnabled)

	// Create timer for scheduler
	timer := NewSimpleTimer()

	// Create shared tools in dependency order - prompt generator first, then scheduler
	promptGeneratorTool := NewPromptGeneratorTool(stateManager, genaiClient, msgService, promptGeneratorPromptFile)
	schedulerTool := NewSchedulerTool(timer, msgService, genaiClient, stateManager, promptGeneratorTool, schedulerPrepTimeMinutes, autoFeedbackAfterPromptEnabled)
	stateTransitionTool := NewStateTransitionTool(stateManager, timer)
	profileSaveTool := NewProfileSaveTool(stateManager)

	// Create modules with shared tools (coordinator removed in new design)
	intakeModule := NewIntakeModule(stateManager, genaiClient, msgService, intakeBotPromptFile, stateTransitionTool, profileSaveTool, schedulerTool, promptGeneratorTool)
	feedbackModule := NewFeedbackModule(stateManager, genaiClient, feedbackTrackerPromptFile, timer, msgService, feedbackInitialTimeout, feedbackFollowupDelay, stateTransitionTool, profileSaveTool, schedulerTool)
	return &ConversationFlow{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		systemPromptFile:    systemPromptFile,
		chatHistoryLimit:    -1, // Default: no limit
		schedulerTool:       schedulerTool,
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
	// Coordinator removed in new design

	// Load intake module system prompt
	if f.intakeModule != nil {
		if err := f.intakeModule.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load intake module system prompt", "error", err)
			// Continue even if intake module prompt fails to load
		}
	}

	// Load prompt generator system prompt (tool)
	if f.promptGeneratorTool != nil {
		if err := f.promptGeneratorTool.LoadSystemPrompt(); err != nil {
			slog.Warn("flow.LoadToolSystemPrompts: failed to load prompt generator tool system prompt", "error", err)
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

	// Check if this is a "Done" poll response and increment success counter
	successResponse := whatsapp.GetSuccessPollResponse()
	if strings.Contains(userMessage, successResponse) {
		slog.Debug("ConversationFlow.processConversationMessage: detected success poll response, incrementing SuccessCount", "participantID", participantID, "response", successResponse)
		if err := f.incrementSuccessCount(ctx, participantID); err != nil {
			slog.Error("ConversationFlow.processConversationMessage: failed to increment SuccessCount", "error", err, "participantID", participantID)
			// Don't fail the request, just log the error
		}
	}

	// Mark daily prompt as replied if a reminder is pending
	if f.schedulerTool != nil {
		f.schedulerTool.handleDailyPromptReply(ctx, participantID, userMsg.Timestamp)
	}

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
	case models.StateIntake:
		response, err = f.processIntakeState(ctx, participantID, userMessage, history)
	case models.StateFeedback:
		response, err = f.processFeedbackState(ctx, participantID, userMessage, history)
	default:
		// Unknown state - default to INTAKE
		slog.Warn("ConversationFlow.processConversationMessage: unknown conversation state, defaulting to INTAKE",
			"conversationState", conversationState, "participantID", participantID)
		if f.stateManager != nil {
			_ = f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateIntake))
		}
		response, err = f.processIntakeState(ctx, participantID, userMessage, history)
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
//
//lint:ignore U1000 kept for potential reuse in tool-enabled flows and debug exports
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
//
//lint:ignore U1000 kept for future conversational context building and used by debug tooling
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
//
//lint:ignore U1000 kept for future conversational context building and used by debug tooling
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
		} else if msg.Role == "system" {
			messages = append(messages, openai.SystemMessage(msg.Content))
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

// GetSchedulerTool returns the scheduler tool for recovery operations.
func (f *ConversationFlow) GetSchedulerTool() *SchedulerTool {
	return f.schedulerTool
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

// incrementSuccessCount increments the success counter when user completes a habit (clicks "Done" button).
func (f *ConversationFlow) incrementSuccessCount(ctx context.Context, participantID string) error {
	if f.stateManager == nil {
		return fmt.Errorf("state manager not initialized")
	}

	// Get current profile
	profileJSON, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil || profileJSON == "" {
		slog.Debug("ConversationFlow.incrementSuccessCount: no profile found, creating new one", "participantID", participantID)
		// Create a new profile if none exists
		profile := &UserProfile{
			SuccessCount: 1,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		profileJSON, err := json.Marshal(profile)
		if err != nil {
			return fmt.Errorf("failed to marshal new profile: %w", err)
		}
		return f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
	}

	// Parse existing profile
	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return fmt.Errorf("failed to parse user profile: %w", err)
	}

	// Increment success count
	profile.SuccessCount++
	profile.UpdatedAt = time.Now()

	// Save updated profile
	updatedProfileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal updated profile: %w", err)
	}

	if err := f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(updatedProfileJSON)); err != nil {
		return fmt.Errorf("failed to save updated profile: %w", err)
	}

	slog.Info("ConversationFlow.incrementSuccessCount: success count incremented", "participantID", participantID, "newCount", profile.SuccessCount)
	return nil
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

	return strings.Join(debugParts, "\n")
}

// getCurrentConversationState retrieves the current conversation state for a participant.
func (f *ConversationFlow) getCurrentConversationState(ctx context.Context, participantID string) (models.StateType, error) {
	if f.stateManager == nil {
		return models.StateIntake, nil // Default to intake if no state manager
	}

	stateStr, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation,
		models.DataKeyConversationState)
	if err != nil {
		return "", err
	}

	// Default to INTAKE if no state is set
	if stateStr == "" {
		// Persist the default for clarity and future routing
		_ = f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateIntake))
		return models.StateIntake, nil
	}

	return models.StateType(stateStr), nil
}

// processCoordinatorState handles messages when in the coordinator state.
// The coordinator decides which tools to use and can transition to other states.
// Coordinator state removed in new design

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

	response, err := f.intakeModule.ExecuteIntakeBotWithHistoryAndConversation(ctx, participantID, args, chatHistory, history)
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

	response, err := f.feedbackModule.ExecuteFeedbackTrackerWithHistoryAndConversation(ctx, participantID, args, chatHistory, history)
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
