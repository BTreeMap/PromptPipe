// Package flow provides coordinator module functionality for managing conversation routing and tools.
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

// CoordinatorModule provides functionality for the coordinator conversation state.
// The coordinator is responsible for routing conversations, using tools, and deciding
// when to transition to other states (intake, feedback).
type CoordinatorModule struct {
	stateManager        StateManager
	genaiClient         genai.ClientInterface
	msgService          MessagingService // Messaging service for sending responses
	systemPromptFile    string
	systemPrompt        string
	schedulerTool       *SchedulerTool       // Tool for scheduling daily prompts
	promptGeneratorTool *PromptGeneratorTool // Tool for generating personalized habit prompts
	stateTransitionTool *StateTransitionTool // Tool for managing conversation state transitions
	profileSaveTool     *ProfileSaveTool     // Tool for saving user profiles (shared across modules)
}

// NewCoordinatorModule creates a new coordinator module instance.
func NewCoordinatorModule(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, systemPromptFile string, schedulerTool *SchedulerTool, promptGeneratorTool *PromptGeneratorTool, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool) *CoordinatorModule {
	slog.Debug("CoordinatorModule.NewCoordinatorModule: creating coordinator module",
		"hasStateManager", stateManager != nil,
		"hasGenAI", genaiClient != nil,
		"hasMessaging", msgService != nil,
		"systemPromptFile", systemPromptFile,
		"hasScheduler", schedulerTool != nil,
		"hasPromptGenerator", promptGeneratorTool != nil,
		"hasStateTransition", stateTransitionTool != nil,
		"hasProfileSave", profileSaveTool != nil)
	return &CoordinatorModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		msgService:          msgService,
		systemPromptFile:    systemPromptFile,
		schedulerTool:       schedulerTool,
		promptGeneratorTool: promptGeneratorTool,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
	}
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (cm *CoordinatorModule) LoadSystemPrompt() error {
	slog.Debug("CoordinatorModule.LoadSystemPrompt: loading system prompt from file", "file", cm.systemPromptFile)

	if cm.systemPromptFile == "" {
		slog.Error("CoordinatorModule.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(cm.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("CoordinatorModule.LoadSystemPrompt: system prompt file does not exist", "file", cm.systemPromptFile)
		return fmt.Errorf("system prompt file does not exist: %s", cm.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(cm.systemPromptFile)
	if err != nil {
		slog.Error("CoordinatorModule.LoadSystemPrompt: failed to read system prompt file", "file", cm.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read system prompt file: %w", err)
	}

	cm.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("CoordinatorModule.LoadSystemPrompt: system prompt loaded successfully", "file", cm.systemPromptFile, "length", len(cm.systemPrompt))
	return nil
}

// ProcessMessageWithHistory handles a user message and can modify the conversation history directly.
func (cm *CoordinatorModule) ProcessMessageWithHistory(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
	slog.Debug("CoordinatorModule.ProcessMessageWithHistory: processing coordinator message",
		"participantID", participantID, "historyLength", len(chatHistory))

	// Validate required dependencies
	if cm.stateManager == nil {
		slog.Error("CoordinatorModule.ProcessMessageWithHistory: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if cm.genaiClient == nil {
		slog.Error("CoordinatorModule.ProcessMessageWithHistory: genai client not initialized")
		return "", fmt.Errorf("genai client not initialized")
	}

	// Load system prompt if not already loaded
	if cm.systemPrompt == "" {
		slog.Debug("CoordinatorModule.ProcessMessageWithHistory: loading system prompt", "participantID", participantID)
		if err := cm.LoadSystemPrompt(); err != nil {
			// If system prompt file doesn't exist or fails to load, use a default
			cm.systemPrompt = "You are a helpful AI coordinator for habit building. You can use tools to help users build habits, schedule prompts, and save their profiles. Use transition_state to move to specialized modules when needed."
			slog.Warn("CoordinatorModule.ProcessMessageWithHistory: using default system prompt due to load failure", "error", err)
		}
	}

	// Build messages for LLM with current context
	messages, err := cm.buildCoordinatorMessages(ctx, participantID, userMessage, chatHistory)
	if err != nil {
		slog.Error("CoordinatorModule.ProcessMessageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build coordinator messages: %w", err)
	}

	// Create tool definitions for coordinator - it has access to all shared tools
	tools := []openai.ChatCompletionToolParam{}

	// Add state transition tool - coordinator's primary tool for routing
	if cm.stateTransitionTool != nil {
		toolDef := cm.stateTransitionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessageWithHistory: added state transition tool", "participantID", participantID)
	}

	// Add profile save tool - coordinator can save user profiles
	if cm.profileSaveTool != nil {
		toolDef := cm.profileSaveTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessageWithHistory: added profile save tool", "participantID", participantID)
	}

	// Add prompt generator tool - coordinator can generate prompts
	if cm.promptGeneratorTool != nil {
		toolDef := cm.promptGeneratorTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessageWithHistory: added prompt generator tool", "participantID", participantID)
	}

	// Add scheduler tool
	if cm.schedulerTool != nil {
		toolDef := cm.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessageWithHistory: added scheduler tool", "participantID", participantID)
	}

	if len(tools) == 0 {
		// No tools available, use standard generation
		response, err := cm.genaiClient.GenerateWithMessages(ctx, messages)
		if err != nil {
			slog.Error("CoordinatorModule.ProcessMessageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
			return "", fmt.Errorf("failed to generate response: %w", err)
		}
		return response, nil
	}

	// Log the exact tools being passed to LLM
	var toolNames []string
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Function.Name)
	}
	slog.Info("CoordinatorModule.ProcessMessageWithHistory: calling LLM with tools",
		"participantID", participantID,
		"toolCount", len(tools),
		"toolNames", toolNames,
		"messageCount", len(messages))

	// Start tool call loop
	return cm.handleCoordinatorToolLoop(ctx, participantID, messages, tools, conversationHistory)
}

// buildCoordinatorMessages creates the message array for the coordinator LLM.
func (cm *CoordinatorModule) buildCoordinatorMessages(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// 1. Add system prompt
	if cm.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(cm.systemPrompt))
	}

	// 2. Add participant background as system message
	participantBackground, err := cm.getParticipantBackground(ctx, participantID)
	if err != nil {
		slog.Warn("CoordinatorModule: Failed to get participant background", "error", err, "participantID", participantID)
	} else if participantBackground != "" {
		messages = append(messages, openai.SystemMessage(participantBackground))
		slog.Debug("CoordinatorModule: Added participant background to messages", "participantID", participantID, "backgroundLength", len(participantBackground))
	}

	// 3. Add profile status information to help coordinator decide when to use intake
	profileStatus := cm.getProfileStatus(ctx, participantID)
	if profileStatus != "" {
		messages = append(messages, openai.SystemMessage(profileStatus))
		slog.Debug("CoordinatorModule: Added profile status to messages", "participantID", participantID, "profileStatus", profileStatus)
	}

	// 4. Add conversation history
	const maxHistoryMessages = 30
	if len(chatHistory) > maxHistoryMessages {
		chatHistory = chatHistory[len(chatHistory)-maxHistoryMessages:]
	}
	messages = append(messages, chatHistory...)

	// 5. Add current user message
	messages = append(messages, openai.UserMessage(userMessage))

	return messages, nil
}

// getParticipantBackground retrieves participant background information from state storage
func (cm *CoordinatorModule) getParticipantBackground(ctx context.Context, participantID string) (string, error) {
	// Try to get participant background from state data
	background, err := cm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyParticipantBackground)
	if err != nil {
		slog.Debug("CoordinatorModule: Error retrieving participant background", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get participant background: %w", err)
	}

	if background == "" {
		return "", nil
	}

	// Format as a system message
	formatted := fmt.Sprintf("PARTICIPANT BACKGROUND:\n%s", background)
	return formatted, nil
}

// getProfileStatus checks the user's profile completeness and returns status information for the coordinator
func (cm *CoordinatorModule) getProfileStatus(ctx context.Context, participantID string) string {
	// Try to get user profile
	profileJSON, err := cm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
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

// handleCoordinatorToolLoop manages the tool call loop for the coordinator module.
// It continues calling the LLM until a user-facing message is generated.
func (cm *CoordinatorModule) handleCoordinatorToolLoop(ctx context.Context, participantID string, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, conversationHistory *ConversationHistory) (string, error) {
	const maxToolRounds = 10 // Prevent infinite loops
	currentMessages := messages

	for round := 1; round <= maxToolRounds; round++ {
		slog.Debug("CoordinatorModule.handleCoordinatorToolLoop: round start", "participantID", participantID, "round", round, "messageCount", len(currentMessages))

		// Generate response with tools
		toolResponse, err := cm.genaiClient.GenerateWithTools(ctx, currentMessages, tools)
		if err != nil {
			slog.Error("CoordinatorModule.handleCoordinatorToolLoop: tool generation failed", "error", err, "participantID", participantID, "round", round)
			return "", fmt.Errorf("failed to generate response with tools: %w", err)
		}

		slog.Debug("CoordinatorModule.handleCoordinatorToolLoop: received tool response",
			"participantID", participantID,
			"round", round,
			"hasContent", toolResponse.Content != "",
			"contentLength", len(toolResponse.Content),
			"toolCallCount", len(toolResponse.ToolCalls))

		// Check if the AI wants to call tools
		if len(toolResponse.ToolCalls) > 0 {
			slog.Info("CoordinatorModule.handleCoordinatorToolLoop: processing tool calls", "participantID", participantID, "round", round, "toolCallCount", len(toolResponse.ToolCalls))

			// Execute tools and update conversation context
			updatedMessages, err := cm.executeCoordinatorToolCallsAndUpdateContext(ctx, participantID, toolResponse, currentMessages, tools, conversationHistory)
			if err != nil {
				return "", err
			}
			currentMessages = updatedMessages

			// If we have content, this is the final response
			if toolResponse.Content != "" {
				slog.Info("CoordinatorModule.handleCoordinatorToolLoop: tool round completed with user message", "participantID", participantID, "round", round, "responseLength", len(toolResponse.Content))
				return toolResponse.Content, nil
			}

			// No content yet, continue to next round
			slog.Debug("CoordinatorModule.handleCoordinatorToolLoop: no user message yet, continuing to next round", "participantID", participantID, "round", round)
			continue
		}

		// No tool calls - check if we have content
		if toolResponse.Content != "" {
			slog.Info("CoordinatorModule.handleCoordinatorToolLoop: final response without tool calls", "participantID", participantID, "round", round, "responseLength", len(toolResponse.Content))
			return toolResponse.Content, nil
		}

		// No tool calls and no content - this shouldn't happen, but handle it
		slog.Warn("CoordinatorModule.handleCoordinatorToolLoop: received empty content and no tool calls", "participantID", participantID, "round", round)
		return "I'm here to help you with your habits. What would you like to work on?", nil
	}

	// If we hit max rounds, return fallback message
	slog.Warn("CoordinatorModule.handleCoordinatorToolLoop: hit maximum tool rounds", "participantID", participantID, "maxRounds", maxToolRounds)
	return "I've completed the requested actions.", nil
}

// executeCoordinatorToolCallsAndUpdateContext executes tool calls and updates the conversation context.
func (cm *CoordinatorModule) executeCoordinatorToolCallsAndUpdateContext(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, conversationHistory *ConversationHistory) ([]openai.ChatCompletionMessageParamUnion, error) {
	// Log and debug the exact tools being executed
	var executingToolNames []string
	for _, toolCall := range toolResponse.ToolCalls {
		executingToolNames = append(executingToolNames, toolCall.Function.Name)
	}
	slog.Info("CoordinatorModule.executeCoordinatorToolCallsAndUpdateContext: executing tools",
		"participantID", participantID,
		"toolCallCount", len(toolResponse.ToolCalls),
		"executingTools", executingToolNames)

	// Send debug message if debug mode is enabled in context
	debugMessage := fmt.Sprintf("CoordinatorModule executing tools: %s", strings.Join(executingToolNames, ", "))
	SendDebugMessageIfEnabled(ctx, participantID, cm.msgService, debugMessage)

	// Create the tool calls in OpenAI format
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
	messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMessageWithToolCalls})

	// Execute each tool call and collect results
	var toolResults []string
	for _, toolCall := range toolResponse.ToolCalls {
		slog.Info("CoordinatorModule: executing tool call", "participantID", participantID, "toolName", toolCall.Function.Name, "toolCallID", toolCall.ID)

		var result string
		var err error

		switch toolCall.Function.Name {
		case "transition_state":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("CoordinatorModule: failed to parse state transition arguments", "error", parseErr, "participantID", participantID)
				result = "State transition completed"
			} else {
				result, err = cm.stateTransitionTool.ExecuteStateTransition(ctx, participantID, args)
				if err != nil {
					slog.Error("CoordinatorModule: state transition failed", "error", err, "participantID", participantID)
					result = "State transition completed"
				}
			}
			toolResults = append(toolResults, result)

		case "save_user_profile":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("CoordinatorModule: failed to parse profile save arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to save profile: invalid arguments"
			} else {
				result, err = cm.profileSaveTool.ExecuteProfileSave(ctx, participantID, args)
				if err != nil {
					slog.Error("CoordinatorModule: profile save failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to save profile: %s", err.Error())
				}
			}
			toolResults = append(toolResults, result)

		case "generate_habit_prompt":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("CoordinatorModule: failed to parse prompt generator arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to generate prompt: invalid arguments"
			} else {
				result, err = cm.promptGeneratorTool.ExecutePromptGenerator(ctx, participantID, args)
				if err != nil {
					slog.Error("CoordinatorModule: prompt generator failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to generate prompt: %s", err.Error())
				}
			}
			toolResults = append(toolResults, result)

		case "scheduler":
			// Parse arguments
			var params models.SchedulerToolParams
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &params); parseErr != nil {
				slog.Error("CoordinatorModule: failed to parse scheduler arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to set up scheduling: invalid arguments"
			} else {
				schedulerResult, err := cm.schedulerTool.ExecuteScheduler(ctx, participantID, params)
				if err != nil {
					slog.Error("CoordinatorModule: scheduler failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to set up scheduling: %s", err.Error())
				} else if !schedulerResult.Success {
					result = fmt.Sprintf("❌ %s", schedulerResult.Message)
				} else {
					result = schedulerResult.Message
				}
			}
			toolResults = append(toolResults, result)

		default:
			slog.Warn("CoordinatorModule: unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			result = fmt.Sprintf("❌ Unknown tool: %s", toolCall.Function.Name)
			toolResults = append(toolResults, result)
		}
	}

	// Add tool call results to the OpenAI conversation context
	for i, toolCall := range toolResponse.ToolCalls {
		// Use the corresponding result from our toolResults array
		resultContent := toolResults[i]
		if resultContent == "" {
			resultContent = "Tool executed successfully"
		}

		// Add tool result message to conversation
		messages = append(messages, openai.ToolMessage(resultContent, toolCall.ID))
	}

	// Also add a summary assistant message about tool execution for conversation history
	// This helps the LLM understand that tools were executed and prevents repeated calls
	if len(toolResults) > 0 {
		var toolSummary strings.Builder
		toolSummary.WriteString("I've executed the following tools: ")

		for i, toolCall := range toolResponse.ToolCalls {
			if i > 0 {
				toolSummary.WriteString(", ")
			}
			toolSummary.WriteString(toolCall.Function.Name)
			if i < len(toolResults) && toolResults[i] != "" {
				// Include a brief result summary (truncated to avoid flooding)
				result := toolResults[i]
				if len(result) > 100 {
					result = result[:97] + "..."
				}
				toolSummary.WriteString(fmt.Sprintf(" (%s)", result))
			}
		}

		// Add as assistant message so it appears in conversation history
		toolSummaryMessage := openai.AssistantMessage(toolSummary.String())
		messages = append(messages, toolSummaryMessage)

		// IMPORTANT: Also add this tool summary to persistent conversation history
		// so future LLM calls can see that tools were already executed
		if conversationHistory != nil {
			toolSummaryHistoryMsg := ConversationMessage{
				Role:      "assistant",
				Content:   toolSummary.String(),
				Timestamp: time.Now(),
			}
			conversationHistory.Messages = append(conversationHistory.Messages, toolSummaryHistoryMsg)

			slog.Debug("CoordinatorModule.executeCoordinatorToolCallsAndUpdateContext: added tool summary to conversation history",
				"participantID", participantID, "toolCount", len(toolResults), "summary", toolSummary.String())
		}
	}

	return messages, nil
}
