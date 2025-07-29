// Package flow provides prompt generator tool functionality for creating personalized habit prompts.
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
	"github.com/openai/openai-go/shared"
)

// PromptGeneratorTool provides LLM tool functionality for generating personalized habit prompts based on user profiles.
type PromptGeneratorTool struct {
	stateManager     StateManager
	genaiClient      genai.ClientInterface
	msgService       MessagingService
	systemPromptFile string
	systemPrompt     string
}

// NewPromptGeneratorTool creates a new prompt generator tool instance.
func NewPromptGeneratorTool(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, systemPromptFile string) *PromptGeneratorTool {
	slog.Debug("flow.NewPromptGeneratorTool: creating prompt generator tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "systemPromptFile", systemPromptFile)
	return &PromptGeneratorTool{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		msgService:       msgService,
		systemPromptFile: systemPromptFile,
	}
}

// GetToolDefinition returns the OpenAI tool definition for generating habit prompts.
func (pgt *PromptGeneratorTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "generate_habit_prompt",
			Description: openai.String("Generate a personalized 1-minute habit prompt using the user's profile. Use this when the user wants to receive their habit suggestion or when triggered by the scheduler."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"delivery_mode": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"immediate", "scheduled"},
						"description": "Whether this is immediate delivery (user requested now) or scheduled delivery (triggered by timer)",
					},
					"personalization_notes": map[string]interface{}{
						"type":        "string",
						"description": "Additional context or personalization notes based on current conversation (optional)",
					},
				},
				"required": []string{"delivery_mode"},
			},
		},
	}
}

// ExecutePromptGenerator executes the prompt generator tool call.
func (pgt *PromptGeneratorTool) ExecutePromptGenerator(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("flow.ExecutePromptGenerator: generating habit prompt", "participantID", participantID, "args", args)

	// Validate required dependencies
	if pgt.stateManager == nil || pgt.genaiClient == nil {
		slog.Error("flow.ExecutePromptGenerator: dependencies not initialized")
		return "", fmt.Errorf("dependencies not properly initialized")
	}

	// Extract arguments
	deliveryMode, _ := args["delivery_mode"].(string)
	personalizationNotes, _ := args["personalization_notes"].(string)

	// Validate required arguments
	if deliveryMode == "" {
		slog.Warn("flow.ExecutePromptGenerator: missing delivery_mode", "participantID", participantID)
		return "", fmt.Errorf("delivery_mode is required")
	}

	// Get user profile
	profile, err := pgt.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecutePromptGenerator: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Validate that profile has required information
	if err := pgt.validateProfile(profile); err != nil {
		slog.Error("flow.ExecutePromptGenerator: invalid profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("profile incomplete: %w", err)
	}

	// Generate the personalized prompt
	habitPrompt, err := pgt.generatePersonalizedPrompt(ctx, profile, deliveryMode, personalizationNotes)
	if err != nil {
		slog.Error("flow.ExecutePromptGenerator: failed to generate prompt", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate habit prompt: %w", err)
	}

	// Store the generated prompt for feedback tracking
	if err := pgt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt, habitPrompt); err != nil {
		slog.Warn("flow.ExecutePromptGenerator: failed to store last prompt", "error", err, "participantID", participantID)
		// Continue despite storage failure
	}

	// Create follow-up question for completion tracking
	completionPrompt := habitPrompt + "\n\nLet me know when you've tried it, or if you'd like to adjust anything!"

	slog.Info("flow.ExecutePromptGenerator: habit prompt generated", "participantID", participantID, "deliveryMode", deliveryMode, "promptLength", len(habitPrompt))
	return completionPrompt, nil
}

// generatePersonalizedPrompt creates a personalized habit prompt using the MAP framework
func (pgt *PromptGeneratorTool) generatePersonalizedPrompt(ctx context.Context, profile *UserProfile, deliveryMode, personalizationNotes string) (string, error) {
	// Build the system prompt for habit generation
	systemPrompt := pgt.buildPromptGeneratorSystemPrompt(profile, deliveryMode, personalizationNotes)

	// Create the user prompt for habit generation
	userPrompt := fmt.Sprintf(
		"Generate a personalized 1-minute habit prompt using this profile:\n"+
			"Target Behavior: %s\n"+
			"Motivation: %s\n"+
			"Preferred Time: %s\n"+
			"Prompt Anchor: %s\n"+
			"Additional Info: %s\n\n"+
			"Use the MAP framework (Motivation, Ability, Prompt) and format as:\n"+
			"\"After/Before [prompt_anchor], try [1-minute action] — it helps you [motivational benefit]. Would that feel doable?\"\n\n"+
			"Keep it under 30 words, skimmable, and actionable.",
		profile.TargetBehavior,
		profile.MotivationalFrame,
		profile.PreferredTime,
		profile.PromptAnchor,
		profile.AdditionalInfo,
	)

	// Generate the prompt using GenAI
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	response, err := pgt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate habit prompt: %w", err)
	}

	// Clean up the response (remove any system messages or formatting)
	habitPrompt := strings.TrimSpace(response)
	habitPrompt = strings.Trim(habitPrompt, "\"'")

	return habitPrompt, nil
}

// buildPromptGeneratorSystemPrompt creates the system prompt for habit generation
func (pgt *PromptGeneratorTool) buildPromptGeneratorSystemPrompt(profile *UserProfile, deliveryMode, personalizationNotes string) string {
	// Start with the loaded system prompt from file
	systemPrompt := pgt.systemPrompt
	if systemPrompt == "" {
		// Fallback if prompt is not loaded
		systemPrompt = "You are a warm, supportive micro-coach bot. Your job is to craft short, personal, 1-minute healthy habit suggestions using the user's profile."
		slog.Warn("flow.buildPromptGeneratorSystemPrompt: using fallback system prompt", "reason", "system prompt not loaded from file")
	}

	// Add delivery mode context
	if deliveryMode == "immediate" {
		systemPrompt += "\n\nContext: User requested this habit suggestion immediately during our conversation."
	} else {
		systemPrompt += "\n\nContext: This is a scheduled delivery based on user's preferred timing."
	}

	// Add personalization notes if provided
	if personalizationNotes != "" {
		systemPrompt += fmt.Sprintf("\n\nAdditional Context: %s", personalizationNotes)
	}

	// Add success tracking context
	if profile.SuccessCount > 0 {
		systemPrompt += fmt.Sprintf("\n\nUser has successfully completed %d habit prompts so far.", profile.SuccessCount)
	}

	// Add barrier context if available
	if profile.LastBarrier != "" {
		systemPrompt += fmt.Sprintf("\n\nNote: User previously mentioned this barrier: %s. Consider this when crafting the suggestion.", profile.LastBarrier)
	}

	// Add modification context if available
	if profile.LastTweak != "" {
		systemPrompt += fmt.Sprintf("\n\nNote: User previously requested: %s. Incorporate this preference.", profile.LastTweak)
	}

	return systemPrompt
}

// validateProfile checks if the profile has the minimum required information
func (pgt *PromptGeneratorTool) validateProfile(profile *UserProfile) error {
	if profile.TargetBehavior == "" {
		return fmt.Errorf("target behavior is required")
	}
	if profile.MotivationalFrame == "" {
		return fmt.Errorf("motivational frame is required")
	}
	if profile.PromptAnchor == "" {
		return fmt.Errorf("prompt anchor is required")
	}
	if profile.PreferredTime == "" {
		return fmt.Errorf("preferred time is required")
	}
	return nil
}

// getUserProfile retrieves the user profile from state storage
func (pgt *PromptGeneratorTool) getUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := pgt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		return nil, fmt.Errorf("user profile not found - please complete intake first")
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		return nil, fmt.Errorf("user profile not found - please complete intake first")
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (pgt *PromptGeneratorTool) LoadSystemPrompt() error {
	slog.Debug("flow.PromptGeneratorTool.LoadSystemPrompt: loading system prompt from file", "file", pgt.systemPromptFile)

	if pgt.systemPromptFile == "" {
		slog.Error("flow.PromptGeneratorTool.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("prompt generator system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(pgt.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.PromptGeneratorTool.LoadSystemPrompt: system prompt file does not exist", "file", pgt.systemPromptFile)
		return fmt.Errorf("prompt generator system prompt file does not exist: %s", pgt.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(pgt.systemPromptFile)
	if err != nil {
		slog.Error("flow.PromptGeneratorTool.LoadSystemPrompt: failed to read system prompt file", "file", pgt.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read prompt generator system prompt file: %w", err)
	}

	pgt.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.PromptGeneratorTool.LoadSystemPrompt: system prompt loaded successfully", "file", pgt.systemPromptFile, "length", len(pgt.systemPrompt))
	return nil
}
