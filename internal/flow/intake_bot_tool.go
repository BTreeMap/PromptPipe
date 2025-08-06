// Package flow provides intake bot tool functionality for building structured user profiles.
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
	"github.com/openai/openai-go/shared"
)

// IntakeBotTool provides LLM tool functionality for conducting intake conversations and building user profiles.
type IntakeBotTool struct {
	stateManager     StateManager
	genaiClient      genai.ClientInterface
	msgService       MessagingService
	systemPromptFile string
	systemPrompt     string
}

// NewIntakeBotTool creates a new intake bot tool instance.
func NewIntakeBotTool(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, systemPromptFile string) *IntakeBotTool {
	slog.Debug("IntakeBotTool.NewIntakeBotTool: creating intake bot tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "systemPromptFile", systemPromptFile)
	return &IntakeBotTool{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		msgService:       msgService,
		systemPromptFile: systemPromptFile,
	}
}

// GetToolDefinition returns the OpenAI tool definition for conducting intake conversations.
func (ibt *IntakeBotTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "conduct_intake",
			Description: openai.String("Conduct a structured intake conversation to build a user profile for personalized habit formation. Use this to guide users through identifying their goals, motivation, timing preferences, and natural habit anchors. The conversation is flexible and adaptive."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"user_response": map[string]interface{}{
						"type":        "string",
						"description": "The user's response to continue the intake conversation",
					},
				},
				"required": []string{},
			},
		},
	}
}

// GetProfileSaveToolDefinition returns the OpenAI tool definition for saving user profiles.
func (ibt *IntakeBotTool) GetProfileSaveToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "save_user_profile",
			Description: openai.String("Save or update the user's profile with information gathered during intake. Use this whenever you have collected meaningful information about the user's habits, goals, motivation, timing preferences, or anchors."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"target_behavior": map[string]interface{}{
						"type":        "string",
						"description": "The specific habit or behavior the user wants to build (e.g., 'healthy eating', 'physical activity', 'better sleep')",
					},
					"motivational_frame": map[string]interface{}{
						"type":        "string",
						"description": "Why this habit matters to the user personally - their deeper motivation",
					},
					"preferred_time": map[string]interface{}{
						"type":        "string",
						"description": "When the user prefers to receive habit prompts (e.g., '9am', 'morning', 'randomly')",
					},
					"prompt_anchor": map[string]interface{}{
						"type":        "string",
						"description": "Natural trigger or anchor for the habit (e.g., 'after coffee', 'before meetings', 'during breaks')",
					},
					"additional_info": map[string]interface{}{
						"type":        "string",
						"description": "Any additional personalization information the user has shared",
					},
				},
				"required": []string{},
			},
		},
	}
}

// ExecuteIntakeBotWithHistory executes the intake bot tool with conversation history context.
func (ibt *IntakeBotTool) ExecuteIntakeBotWithHistory(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("flow.ExecuteIntakeBotWithHistory: processing intake with chat history", "participantID", participantID, "args", args, "historyLength", len(chatHistory))

	// Validate required dependencies
	if ibt.stateManager == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if ibt.genaiClient == nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: genai client not initialized")
		return "", fmt.Errorf("genai client not initialized")
	}

	// Extract optional user response
	userResponse, _ := args["user_response"].(string)

	// Get current user profile to understand what information we already have
	profile, err := ibt.getOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Build messages for LLM with current profile context
	messages, err := ibt.buildIntakeMessagesWithContext(ctx, participantID, userResponse, profile, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate response using LLM
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate intake response: %w", err)
	}

	slog.Info("flow.ExecuteIntakeBotWithHistory: intake response generated", "participantID", participantID, "responseLength", len(response))
	return response, nil
}

// ExecuteProfileSave executes the profile save tool call.
func (ibt *IntakeBotTool) ExecuteProfileSave(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("flow.ExecuteProfileSave: saving profile", "participantID", participantID, "args", args)

	// Validate required dependencies
	if ibt.stateManager == nil {
		slog.Error("flow.ExecuteProfileSave: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	// Get or create user profile
	profile, err := ibt.getOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteProfileSave: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Update profile fields from arguments
	var updated bool
	if targetBehavior, ok := args["target_behavior"].(string); ok && targetBehavior != "" {
		profile.TargetBehavior = targetBehavior
		updated = true
		slog.Debug("flow.ExecuteProfileSave: updated target behavior", "participantID", participantID, "targetBehavior", targetBehavior)
	}

	if motivationalFrame, ok := args["motivational_frame"].(string); ok && motivationalFrame != "" {
		profile.MotivationalFrame = motivationalFrame
		updated = true
		slog.Debug("flow.ExecuteProfileSave: updated motivational frame", "participantID", participantID, "motivationalFrame", motivationalFrame)
	}

	if preferredTime, ok := args["preferred_time"].(string); ok && preferredTime != "" {
		profile.PreferredTime = preferredTime
		updated = true
		slog.Debug("flow.ExecuteProfileSave: updated preferred time", "participantID", participantID, "preferredTime", preferredTime)
	}

	if promptAnchor, ok := args["prompt_anchor"].(string); ok && promptAnchor != "" {
		profile.PromptAnchor = promptAnchor
		updated = true
		slog.Debug("flow.ExecuteProfileSave: updated prompt anchor", "participantID", participantID, "promptAnchor", promptAnchor)
	}

	if additionalInfo, ok := args["additional_info"].(string); ok && additionalInfo != "" {
		profile.AdditionalInfo = additionalInfo
		updated = true
		slog.Debug("flow.ExecuteProfileSave: updated additional info", "participantID", participantID, "additionalInfo", additionalInfo)
	}

	// Save profile if anything was updated
	if updated {
		if err := ibt.saveUserProfile(ctx, participantID, profile); err != nil {
			slog.Error("flow.ExecuteProfileSave: failed to save user profile", "error", err, "participantID", participantID)
			return "", fmt.Errorf("failed to save user profile: %w", err)
		}
		slog.Info("flow.ExecuteProfileSave: profile saved successfully", "participantID", participantID)
		return "Profile updated successfully", nil
	}

	return "No profile changes to save", nil
}

// getOrCreateUserProfile retrieves or creates a new user profile
func (ibt *IntakeBotTool) getOrCreateUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := ibt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("flow.getOrCreateUserProfile: creating new profile", "participantID", participantID)
		// Create new profile
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		slog.Debug("flow.getOrCreateUserProfile: empty profile data, creating new one", "participantID", participantID)
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Error("flow.getOrCreateUserProfile: failed to unmarshal profile", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	slog.Debug("flow.getOrCreateUserProfile: loaded existing profile",
		"participantID", participantID,
		"targetBehavior", profile.TargetBehavior,
		"motivationalFrame", profile.MotivationalFrame,
		"preferredTime", profile.PreferredTime,
		"promptAnchor", profile.PromptAnchor,
		"createdAt", profile.CreatedAt,
		"updatedAt", profile.UpdatedAt)

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage
func (ibt *IntakeBotTool) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	slog.Debug("flow.saveUserProfile: saving profile",
		"participantID", participantID,
		"targetBehavior", profile.TargetBehavior,
		"motivationalFrame", profile.MotivationalFrame,
		"preferredTime", profile.PreferredTime,
		"promptAnchor", profile.PromptAnchor,
		"additionalInfo", profile.AdditionalInfo)

	profile.UpdatedAt = time.Now()
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		slog.Error("flow.saveUserProfile: failed to marshal profile", "error", err, "participantID", participantID)
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	slog.Debug("flow.saveUserProfile: marshaled profile", "participantID", participantID, "profileJSON", string(profileJSON))

	err = ibt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
	if err != nil {
		slog.Error("flow.saveUserProfile: failed to save to state manager", "error", err, "participantID", participantID)
		return err
	}

	slog.Debug("flow.saveUserProfile: profile saved successfully", "participantID", participantID)
	return nil
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (ibt *IntakeBotTool) LoadSystemPrompt() error {
	slog.Debug("flow.IntakeBotTool.LoadSystemPrompt: loading system prompt from file", "file", ibt.systemPromptFile)

	if ibt.systemPromptFile == "" {
		slog.Error("flow.IntakeBotTool.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("intake bot system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(ibt.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.IntakeBotTool.LoadSystemPrompt: system prompt file does not exist", "file", ibt.systemPromptFile)
		return fmt.Errorf("intake bot system prompt file does not exist: %s", ibt.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(ibt.systemPromptFile)
	if err != nil {
		slog.Error("flow.IntakeBotTool.LoadSystemPrompt: failed to read system prompt file", "file", ibt.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read intake bot system prompt file: %w", err)
	}

	ibt.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.IntakeBotTool.LoadSystemPrompt: system prompt loaded successfully", "file", ibt.systemPromptFile, "length", len(ibt.systemPrompt))
	return nil
}

// buildIntakeMessagesWithContext creates OpenAI messages with current profile context for intelligent intake
func (ibt *IntakeBotTool) buildIntakeMessagesWithContext(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if ibt.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(ibt.systemPrompt))
	}

	// Add intelligent intake context based on current profile state
	contextMessage := ibt.buildIntakeContext(profile)
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
func (ibt *IntakeBotTool) buildIntakeContext(profile *UserProfile) string {
	var missing []string
	var present []string

	if profile.TargetBehavior == "" {
		missing = append(missing, "target behavior/habit")
	} else {
		present = append(present, fmt.Sprintf("target behavior: %s", profile.TargetBehavior))
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
