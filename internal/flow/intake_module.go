// Package flow provides intake module functionality for building structured user profiles.
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

// IntakeModule provides LLM module functionality for conducting intake conversations and building user profiles.
// This module handles the intake conversation state and has access to shared tools.
type IntakeModule struct {
	stateManager        StateManager
	genaiClient         genai.ClientInterface
	msgService          MessagingService // Messaging service for sending responses
	systemPromptFile    string
	systemPrompt        string
	stateTransitionTool *StateTransitionTool // Tool for transitioning back to coordinator
	profileSaveTool     *ProfileSaveTool     // Tool for saving user profiles
	schedulerTool       *SchedulerTool       // Tool for scheduling prompts
	promptGeneratorTool *PromptGeneratorTool // Tool for generating personalized habit prompts
}

// NewIntakeModule creates a new intake module instance.
func NewIntakeModule(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, systemPromptFile string, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool, schedulerTool *SchedulerTool, promptGeneratorTool *PromptGeneratorTool) *IntakeModule {
	slog.Debug("IntakeModule.NewIntakeModule: creating intake module", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "systemPromptFile", systemPromptFile)
	return &IntakeModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		msgService:          msgService,
		systemPromptFile:    systemPromptFile,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
		schedulerTool:       schedulerTool,
		promptGeneratorTool: promptGeneratorTool,
	}
}

// ExecuteIntakeBotWithHistory executes the intake bot tool with conversation history context.
func (im *IntakeModule) ExecuteIntakeBotWithHistory(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	return im.ExecuteIntakeBotWithHistoryAndConversation(ctx, participantID, args, chatHistory, nil)
}

// ExecuteIntakeBotWithHistoryAndConversation executes the intake bot tool and can modify the conversation history directly.
func (im *IntakeModule) ExecuteIntakeBotWithHistoryAndConversation(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
	slog.Debug("flow.ExecuteIntakeBotWithHistoryAndConversation: processing intake with chat history", "participantID", participantID, "args", args, "historyLength", len(chatHistory))

	// Validate required dependencies
	if im.stateManager == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistoryAndConversation: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if im.genaiClient == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistoryAndConversation: genai client not initialized")
		return "", fmt.Errorf("genai client not initialized")
	}

	// Extract optional user response
	userResponse, _ := args["user_response"].(string)

	// Get current user profile to understand what information we already have
	profile, err := im.profileSaveTool.GetOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Build messages for LLM with current profile context
	messages, err := im.buildIntakeMessagesWithContext(ctx, participantID, userResponse, profile, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Create tool definitions for intake module
	tools := []openai.ChatCompletionToolParam{}

	// Add state transition tool
	if im.stateTransitionTool != nil {
		toolDef := im.stateTransitionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("IntakeModule.ExecuteIntakeBotWithHistory: added state transition tool", "participantID", participantID)
	}

	// Add profile save tool
	if im.profileSaveTool != nil {
		toolDef := im.profileSaveTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("IntakeModule.ExecuteIntakeBotWithHistory: added profile save tool", "participantID", participantID)
	}

	// Add scheduler tool
	if im.schedulerTool != nil {
		toolDef := im.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("IntakeModule.ExecuteIntakeBotWithHistory: added scheduler tool", "participantID", participantID)
	}

	// Add prompt generator tool - intake can generate prompts
	if im.promptGeneratorTool != nil {
		toolDef := im.promptGeneratorTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("IntakeModule.ExecuteIntakeBotWithHistory: added prompt generator tool", "participantID", participantID)
	}

	// Log the exact tools being passed to LLM
	var toolNames []string
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Function.Name)
	}
	slog.Info("IntakeModule.ExecuteIntakeBotWithHistory: calling LLM with tools",
		"participantID", participantID,
		"toolCount", len(tools),
		"toolNames", toolNames,
		"messageCount", len(messages))

	// Generate response using LLM with tools
	response, err := im.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: GenAI generation failed", "error", err, "participantID", participantID, "toolNames", toolNames)
		return "", fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Check if there are tool calls to handle
	if len(response.ToolCalls) > 0 {
		slog.Info("IntakeModule.ExecuteIntakeBotWithHistory: processing tool calls", "participantID", participantID, "toolCallCount", len(response.ToolCalls))
		return im.handleIntakeToolLoop(ctx, participantID, response, messages, tools, conversationHistory)
	}

	// No tool calls, return direct response
	slog.Info("flow.ExecuteIntakeBotWithHistory: intake response generated", "participantID", participantID, "responseLength", len(response.Content))
	return response.Content, nil
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (im *IntakeModule) LoadSystemPrompt() error {
	slog.Debug("flow.IntakeModule.LoadSystemPrompt: loading system prompt from file", "file", im.systemPromptFile)

	if im.systemPromptFile == "" {
		slog.Error("flow.IntakeModule.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("intake module system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(im.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.IntakeModule.LoadSystemPrompt: system prompt file does not exist", "file", im.systemPromptFile)
		return fmt.Errorf("intake module system prompt file does not exist: %s", im.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(im.systemPromptFile)
	if err != nil {
		slog.Error("flow.IntakeModule.LoadSystemPrompt: failed to read system prompt file", "file", im.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read intake module system prompt file: %w", err)
	}

	im.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.IntakeModule.LoadSystemPrompt: system prompt loaded successfully", "file", im.systemPromptFile, "length", len(im.systemPrompt))
	return nil
}

// buildIntakeMessagesWithContext creates OpenAI messages with current profile context for intelligent intake
func (im *IntakeModule) buildIntakeMessagesWithContext(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if im.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(im.systemPrompt))
	}

	// Add intelligent intake context based on current profile state
	contextMessage := im.buildIntakeContext(profile)
	messages = append(messages, openai.SystemMessage(contextMessage))

	// Add conversation history
	messages = append(messages, chatHistory...)

	// Add current user response if provided
	if userResponse != "" {
		messages = append(messages, openai.UserMessage(userResponse))
	}

	return messages, nil
}

// buildIntakeContext creates context instructions for the LLM based on current profile completeness
func (im *IntakeModule) buildIntakeContext(profile *UserProfile) string {
	var missing []string
	var present []string

	if profile.HabitDomain == "" {
		missing = append(missing, "habit domain")
	} else {
		present = append(present, fmt.Sprintf("habit domain: %s", profile.HabitDomain))
	}

	if profile.MotivationalFrame == "" {
		missing = append(missing, "motivation/why this matters")
	} else {
		present = append(present, fmt.Sprintf("motivation: %s", profile.MotivationalFrame))
	}

	if profile.PreferredTime == "" {
		missing = append(missing, "preferred timing for prompts")
	} else {
		present = append(present, fmt.Sprintf("preferred time: %s", profile.PreferredTime))
	}

	if profile.PromptAnchor == "" {
		missing = append(missing, "natural anchor/trigger for the habit")
	} else {
		present = append(present, fmt.Sprintf("habit anchor: %s", profile.PromptAnchor))
	}

	context := "INTAKE CONVERSATION CONTEXT:\n\n"

	if len(present) > 0 {
		context += "CURRENT PROFILE INFORMATION:\n"
		for _, info := range present {
			context += "• " + info + "\n"
		}
		context += "\n"
	}

	if len(missing) > 0 {
		context += "STILL NEEDED TO COMPLETE PROFILE:\n"
		for _, need := range missing {
			context += "• " + need + "\n"
		}
		context += "\n"
	}

	context += "INSTRUCTIONS:\n"
	context += "• Continue the intake conversation naturally and conversationally\n"
	context += "• If this is the first message, welcome the user warmly and offer to help build a 1-minute habit\n"
	context += "• Focus on gathering missing information in a natural, engaging way\n"
	context += "• Ask follow-up questions to get specific, actionable details\n"
	context += "• Use the save_user_profile tool whenever you collect meaningful information\n"
	context += "• Once you have enough information, offer to create their first habit prompt\n"
	context += "• Keep responses warm, encouraging, and concise\n"

	return context
}

// handleIntakeToolLoop manages the tool call loop for the intake module.
// It continues calling the LLM until a user-facing message is generated.
func (im *IntakeModule) handleIntakeToolLoop(ctx context.Context, participantID string, initialResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, conversationHistory *ConversationHistory) (string, error) {
	const maxToolRounds = 10 // Prevent infinite loops
	currentMessages := messages
	currentResponse := initialResponse

	for round := 1; round <= maxToolRounds; round++ {
		slog.Debug("IntakeModule.handleIntakeToolLoop: round start", "participantID", participantID, "round", round, "messageCount", len(currentMessages))

		// Process tool calls if any
		if len(currentResponse.ToolCalls) > 0 {
			slog.Info("IntakeModule.handleIntakeToolLoop: processing tool calls", "participantID", participantID, "round", round, "toolCallCount", len(currentResponse.ToolCalls))

			// Execute tools and update conversation context
			updatedMessages, err := im.executeIntakeToolCallsAndUpdateContext(ctx, participantID, currentResponse, currentMessages, tools, conversationHistory)
			if err != nil {
				return "", err
			}
			currentMessages = updatedMessages

			// If we have content, this is the final response
			if currentResponse.Content != "" {
				slog.Info("IntakeModule.handleIntakeToolLoop: tool round completed with user message", "participantID", participantID, "round", round, "responseLength", len(currentResponse.Content))
				return currentResponse.Content, nil
			}

			// No content yet, call LLM again for next round
			slog.Debug("IntakeModule.handleIntakeToolLoop: no user message yet, continuing to next round", "participantID", participantID, "round", round)
		} else {
			// No tool calls - check if we have content
			if currentResponse.Content != "" {
				slog.Info("IntakeModule.handleIntakeToolLoop: final response without tool calls", "participantID", participantID, "round", round, "responseLength", len(currentResponse.Content))
				return currentResponse.Content, nil
			}

			// No tool calls and no content - this shouldn't happen, but handle it
			slog.Warn("IntakeModule.handleIntakeToolLoop: received empty content and no tool calls", "participantID", participantID, "round", round)
			return "I'm here to help you build better habits. What would you like to work on?", nil
		}

		// Generate next response with tools
		toolResponse, err := im.genaiClient.GenerateWithTools(ctx, currentMessages, tools)
		if err != nil {
			slog.Error("IntakeModule.handleIntakeToolLoop: tool generation failed", "error", err, "participantID", participantID, "round", round)
			return "", fmt.Errorf("failed to generate response with tools: %w", err)
		}

		slog.Debug("IntakeModule.handleIntakeToolLoop: received tool response",
			"participantID", participantID,
			"round", round,
			"hasContent", toolResponse.Content != "",
			"contentLength", len(toolResponse.Content),
			"toolCallCount", len(toolResponse.ToolCalls))

		currentResponse = toolResponse
	}

	// If we hit max rounds, return fallback message
	slog.Warn("IntakeModule.handleIntakeToolLoop: hit maximum tool rounds", "participantID", participantID, "maxRounds", maxToolRounds)
	return "I've completed the requested actions.", nil
}

// executeIntakeToolCallsAndUpdateContext executes tool calls and updates the conversation context.
func (im *IntakeModule) executeIntakeToolCallsAndUpdateContext(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, conversationHistory *ConversationHistory) ([]openai.ChatCompletionMessageParamUnion, error) {
	// Log and debug the exact tools being executed
	var executingToolNames []string
	for _, toolCall := range toolResponse.ToolCalls {
		executingToolNames = append(executingToolNames, toolCall.Function.Name)
	}
	slog.Info("IntakeModule.executeIntakeToolCallsAndUpdateContext: executing tools",
		"participantID", participantID,
		"toolCallCount", len(toolResponse.ToolCalls),
		"executingTools", executingToolNames)

	// Send debug message if debug mode is enabled in context
	debugMessage := fmt.Sprintf("IntakeModule executing tools: %s", strings.Join(executingToolNames, ", "))
	SendDebugMessageIfEnabled(ctx, participantID, im.msgService, debugMessage)

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
		slog.Info("IntakeModule: executing tool call", "participantID", participantID, "toolName", toolCall.Function.Name, "toolCallID", toolCall.ID)

		var result string
		var err error

		switch toolCall.Function.Name {
		case "transition_state":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("IntakeModule: failed to parse state transition arguments", "error", parseErr, "participantID", participantID)
				result = "State transition completed"
			} else {
				result, err = im.stateTransitionTool.ExecuteStateTransition(ctx, participantID, args)
				if err != nil {
					slog.Error("IntakeModule: state transition failed", "error", err, "participantID", participantID)
					result = "State transition completed"
				}
			}
			toolResults = append(toolResults, result)

		case "save_user_profile":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("IntakeModule: failed to parse profile save arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to save profile: invalid arguments"
			} else {
				result, err = im.profileSaveTool.ExecuteProfileSave(ctx, participantID, args)
				if err != nil {
					slog.Error("IntakeModule: profile save failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to save profile: %s", err.Error())
				}
			}
			toolResults = append(toolResults, result)

		case "scheduler":
			// Parse arguments
			var params models.SchedulerToolParams
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &params); parseErr != nil {
				slog.Error("IntakeModule: failed to parse scheduler arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to set up scheduling: invalid arguments"
			} else {
				schedulerResult, err := im.schedulerTool.ExecuteScheduler(ctx, participantID, params)
				if err != nil {
					slog.Error("IntakeModule: scheduler failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to set up scheduling: %s", err.Error())
				} else if !schedulerResult.Success {
					result = fmt.Sprintf("❌ %s", schedulerResult.Message)
				} else {
					result = schedulerResult.Message
				}
			}
			toolResults = append(toolResults, result)

		case "generate_habit_prompt":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("IntakeModule: failed to parse prompt generator arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to generate prompt: invalid arguments"
			} else {
				result, err = im.promptGeneratorTool.ExecutePromptGenerator(ctx, participantID, args)
				if err != nil {
					slog.Error("IntakeModule: prompt generator failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to generate prompt: %s", err.Error())
				}
			}
			toolResults = append(toolResults, result)

		default:
			slog.Warn("IntakeModule: unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
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

			slog.Debug("IntakeModule.executeIntakeToolCallsAndUpdateContext: added tool summary to conversation history",
				"participantID", participantID, "toolCount", len(toolResults), "summary", toolSummary.String())
		}
	}

	return messages, nil
}
