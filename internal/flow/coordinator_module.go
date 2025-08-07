// Package flow provides coordinator module functionality for managing conversation routing and tools.
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
)

// CoordinatorModule provides functionality for the coordinator conversation state.
// The coordinator is responsible for routing conversations, using tools, and deciding
// when to transition to other states (intake, feedback).
type CoordinatorModule struct {
	stateManager        StateManager
	genaiClient         genai.ClientInterface
	systemPromptFile    string
	systemPrompt        string
	schedulerTool       *SchedulerTool       // Tool for scheduling daily prompts
	promptGeneratorTool *PromptGeneratorTool // Tool for generating personalized habit prompts
	stateTransitionTool *StateTransitionTool // Tool for managing conversation state transitions
	profileSaveTool     *ProfileSaveTool     // Tool for saving user profiles (shared across modules)
}

// NewCoordinatorModule creates a new coordinator module instance.
func NewCoordinatorModule(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, schedulerTool *SchedulerTool, promptGeneratorTool *PromptGeneratorTool, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool) *CoordinatorModule {
	slog.Debug("CoordinatorModule.NewCoordinatorModule: creating coordinator module",
		"hasStateManager", stateManager != nil,
		"hasGenAI", genaiClient != nil,
		"systemPromptFile", systemPromptFile,
		"hasScheduler", schedulerTool != nil,
		"hasPromptGenerator", promptGeneratorTool != nil,
		"hasStateTransition", stateTransitionTool != nil,
		"hasProfileSave", profileSaveTool != nil)
	return &CoordinatorModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
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

// ProcessMessage handles a user message in the coordinator state.
// The coordinator has access to all tools and can decide when to transition to other states.
func (cm *CoordinatorModule) ProcessMessage(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("CoordinatorModule.ProcessMessage: processing coordinator message",
		"participantID", participantID, "historyLength", len(chatHistory))

	// Validate required dependencies
	if cm.stateManager == nil {
		slog.Error("CoordinatorModule.ProcessMessage: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if cm.genaiClient == nil {
		slog.Error("CoordinatorModule.ProcessMessage: genai client not initialized")
		return "", fmt.Errorf("genai client not initialized")
	}

	// Load system prompt if not already loaded
	if cm.systemPrompt == "" {
		slog.Debug("CoordinatorModule.ProcessMessage: loading system prompt", "participantID", participantID)
		if err := cm.LoadSystemPrompt(); err != nil {
			// If system prompt file doesn't exist or fails to load, use a default
			cm.systemPrompt = "You are a helpful AI coordinator for habit building. You can use tools to help users build habits, schedule prompts, and save their profiles. Use transition_state to move to specialized modules when needed."
			slog.Warn("CoordinatorModule.ProcessMessage: using default system prompt due to load failure", "error", err)
		}
	}

	// Build messages for LLM with current context
	messages, err := cm.buildCoordinatorMessages(ctx, participantID, userMessage, chatHistory)
	if err != nil {
		slog.Error("CoordinatorModule.ProcessMessage: failed to build messages", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build coordinator messages: %w", err)
	}

	// Create tool definitions for coordinator - it has access to all shared tools
	tools := []openai.ChatCompletionToolParam{}

	// Add state transition tool - coordinator's primary tool for routing
	if cm.stateTransitionTool != nil {
		toolDef := cm.stateTransitionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessage: added state transition tool", "participantID", participantID)
	}

	// Add profile save tool - coordinator can save user profiles
	if cm.profileSaveTool != nil {
		toolDef := cm.profileSaveTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessage: added profile save tool", "participantID", participantID)
	}

	// Add prompt generator tool - coordinator can generate prompts
	if cm.promptGeneratorTool != nil {
		toolDef := cm.promptGeneratorTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessage: added prompt generator tool", "participantID", participantID)
	}

	// Add scheduler tool
	if cm.schedulerTool != nil {
		toolDef := cm.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("CoordinatorModule.ProcessMessage: added scheduler tool", "participantID", participantID)
	}

	if len(tools) == 0 {
		// No tools available, use standard generation
		response, err := cm.genaiClient.GenerateWithMessages(ctx, messages)
		if err != nil {
			slog.Error("CoordinatorModule.ProcessMessage: GenAI generation failed", "error", err, "participantID", participantID)
			return "", fmt.Errorf("failed to generate response: %w", err)
		}
		return response, nil
	}

	// Generate response with tools
	response, err := cm.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("CoordinatorModule.ProcessMessage: GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate coordinator response: %w", err)
	}

	slog.Info("CoordinatorModule.ProcessMessage: coordinator response generated", "participantID", participantID, "responseLength", len(response))
	return response, nil
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
