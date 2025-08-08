// Package flow provides intake module functionality for building structured user profiles.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
}

// NewIntakeModule creates a new intake module instance.
func NewIntakeModule(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, systemPromptFile string, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool, schedulerTool *SchedulerTool) *IntakeModule {
	slog.Debug("IntakeModule.NewIntakeModule: creating intake module", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "systemPromptFile", systemPromptFile)
	return &IntakeModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		msgService:          msgService,
		systemPromptFile:    systemPromptFile,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
		schedulerTool:       schedulerTool,
	}
}

// ExecuteIntakeBotWithHistory executes the intake bot tool with conversation history context.
func (im *IntakeModule) ExecuteIntakeBotWithHistory(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("flow.ExecuteIntakeBotWithHistory: processing intake with chat history", "participantID", participantID, "args", args, "historyLength", len(chatHistory))

	// Validate required dependencies
	if im.stateManager == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if im.genaiClient == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: genai client not initialized")
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

	// Generate response using LLM with tools
	response, err := im.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Check if there are tool calls to handle
	if len(response.ToolCalls) > 0 {
		slog.Info("IntakeModule.ExecuteIntakeBotWithHistory: processing tool calls", "participantID", participantID, "toolCallCount", len(response.ToolCalls))
		return im.handleIntakeToolCalls(ctx, participantID, response, messages, tools)
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

// handleIntakeToolCalls processes tool calls from the intake module AI and executes them.
func (im *IntakeModule) handleIntakeToolCalls(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (string, error) {
	slog.Info("IntakeModule.handleIntakeToolCalls: processing tool calls", "participantID", participantID, "toolCallCount", len(toolResponse.ToolCalls))

	// Execute each tool call
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

		default:
			slog.Warn("IntakeModule: unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			result = fmt.Sprintf("❌ Unknown tool: %s", toolCall.Function.Name)
			toolResults = append(toolResults, result)
		}
	}

	// Add tool calls and results to conversation context for LLM follow-up
	for i, toolCall := range toolResponse.ToolCalls {
		// Add assistant message with tool call
		assistantMsg := openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.NewOpt(toolResponse.Content),
			},
			ToolCalls: []openai.ChatCompletionMessageToolCallParam{{
				ID:   toolCall.ID,
				Type: "function",
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      toolCall.Function.Name,
					Arguments: string(toolCall.Function.Arguments),
				},
			}},
		}
		messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg})

		// Add tool result
		if i < len(toolResults) {
			messages = append(messages, openai.ToolMessage(toolResults[i], toolCall.ID))
		}
	}

	// Call LLM again to generate final response with tools available
	finalResponse, err := im.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("IntakeModule: failed to generate final response after tool execution", "error", err, "participantID", participantID)
		// Fallback: return tool results directly
		if len(toolResults) == 1 {
			return toolResults[0], nil
		}
		return strings.Join(toolResults, "\n\n"), nil
	}

	return finalResponse.Content, nil
}
