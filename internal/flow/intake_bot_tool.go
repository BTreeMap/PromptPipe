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

// IntakeState represents the current stage of the intake conversation
type IntakeState string

const (
	IntakeStateWelcome        IntakeState = "WELCOME"
	IntakeStateGoalArea       IntakeState = "GOAL_AREA"
	IntakeStateMotivation     IntakeState = "MOTIVATION"
	IntakeStatePreferredTime  IntakeState = "PREFERRED_TIME"
	IntakeStatePromptAnchor   IntakeState = "PROMPT_ANCHOR"
	IntakeStateAdditionalInfo IntakeState = "ADDITIONAL_INFO"
	IntakeStateComplete       IntakeState = "COMPLETE"
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
	slog.Debug("flow.NewIntakeBotTool: creating intake bot tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil, "systemPromptFile", systemPromptFile)
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
			Description: openai.String("Conduct a structured intake conversation to build a user profile for personalized habit formation. Use this to guide users through identifying their goals, motivation, timing preferences, and natural habit anchors."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_stage": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"welcome", "goal_area", "motivation", "preferred_time", "prompt_anchor", "additional_info", "complete"},
						"description": "Current stage of the intake conversation",
					},
					"user_response": map[string]interface{}{
						"type":        "string",
						"description": "The user's response to the current intake question",
					},
					"next_question": map[string]interface{}{
						"type":        "string",
						"description": "The next question to ask the user based on the intake flow",
					},
				},
				"required": []string{"conversation_stage"},
			},
		},
	}
}

// ExecuteIntakeBot executes the intake bot tool call.
func (ibt *IntakeBotTool) ExecuteIntakeBot(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("flow.ExecuteIntakeBot: processing intake", "participantID", participantID, "args", args)

	// Validate required dependencies
	if ibt.stateManager == nil {
		slog.Error("flow.ExecuteIntakeBot: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	// Extract arguments
	conversationStage, _ := args["conversation_stage"].(string)
	userResponse, _ := args["user_response"].(string)
	nextQuestion, _ := args["next_question"].(string)

	// Validate required arguments
	if conversationStage == "" {
		slog.Warn("flow.ExecuteIntakeBot: missing conversation_stage", "participantID", participantID)
		return "", fmt.Errorf("conversation_stage is required")
	}

	// Get or create user profile
	profile, err := ibt.getOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBot: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Get current intake state
	currentIntakeState, err := ibt.getIntakeState(ctx, participantID)
	if err != nil {
		slog.Debug("flow.ExecuteIntakeBot: no intake state found, starting fresh", "participantID", participantID)
		currentIntakeState = IntakeStateWelcome
	}

	// Process the intake conversation
	response, newState, err := ibt.processIntakeStageWithHistory(ctx, participantID, stringToIntakeState(conversationStage), userResponse, nextQuestion, profile, []openai.ChatCompletionMessageParamUnion{})
	if err != nil {
		slog.Error("flow.ExecuteIntakeBot: failed to process intake stage", "error", err, "participantID", participantID, "stage", conversationStage)
		return "", fmt.Errorf("failed to process intake stage: %w", err)
	}

	// Update intake state
	if newState != currentIntakeState {
		if err := ibt.setIntakeState(ctx, participantID, newState); err != nil {
			slog.Warn("flow.ExecuteIntakeBot: failed to update intake state", "error", err, "participantID", participantID)
			// Continue despite state update failure
		}
	}

	// Save updated profile if it was modified
	if err := ibt.saveUserProfile(ctx, participantID, profile); err != nil {
		slog.Error("flow.ExecuteIntakeBot: failed to save user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to save user profile: %w", err)
	}

	slog.Info("flow.ExecuteIntakeBot: intake stage processed", "participantID", participantID, "stage", conversationStage, "newState", newState)
	return response, nil
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

	// Extract required arguments
	conversationStage, hasStage := args["conversation_stage"].(string)
	if !hasStage || conversationStage == "" {
		return "", fmt.Errorf("conversation_stage is required")
	}

	// Extract optional arguments
	userResponse, _ := args["user_response"].(string)
	nextQuestion, _ := args["next_question"].(string)

	// Log extracted parameters
	slog.Debug("flow.ExecuteIntakeBotWithHistory: extracted parameters",
		"participantID", participantID,
		"conversationStage", conversationStage,
		"userResponse", userResponse,
		"nextQuestion", nextQuestion)

	// Get or create user profile
	profile, err := ibt.getOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Get current intake state
	currentIntakeState, err := ibt.getIntakeState(ctx, participantID)
	if err != nil {
		slog.Debug("flow.ExecuteIntakeBotWithHistory: no intake state found, starting fresh", "participantID", participantID)
		currentIntakeState = IntakeStateWelcome
	}

	// Process the intake conversation with history
	response, newState, err := ibt.processIntakeStageWithHistory(ctx, participantID, stringToIntakeState(conversationStage), userResponse, nextQuestion, profile, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to process intake stage", "error", err, "participantID", participantID, "stage", conversationStage)
		return "", fmt.Errorf("failed to process intake stage: %w", err)
	}

	// Update intake state
	if newState != currentIntakeState {
		if err := ibt.setIntakeState(ctx, participantID, newState); err != nil {
			slog.Warn("flow.ExecuteIntakeBotWithHistory: failed to update intake state", "error", err, "participantID", participantID)
			// Continue despite state update failure
		}
	}

	// Save updated profile if it was modified
	if err := ibt.saveUserProfile(ctx, participantID, profile); err != nil {
		slog.Error("flow.ExecuteIntakeBotWithHistory: failed to save user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to save user profile: %w", err)
	}

	slog.Info("flow.ExecuteIntakeBotWithHistory: intake stage processed", "participantID", participantID, "stage", conversationStage, "newState", newState)
	return response, nil
}

// processIntakeStage handles the logic for each stage of the intake conversation (legacy method - calls history version with empty history)
func (ibt *IntakeBotTool) processIntakeStage(ctx context.Context, participantID string, stage IntakeState, userResponse, nextQuestion string, profile *UserProfile) (string, IntakeState, error) {
	return ibt.processIntakeStageWithHistory(ctx, participantID, stage, userResponse, nextQuestion, profile, []openai.ChatCompletionMessageParamUnion{})
}

// processIntakeStageWithHistory handles the logic for each stage with conversation history
func (ibt *IntakeBotTool) processIntakeStageWithHistory(ctx context.Context, participantID string, stage IntakeState, userResponse, nextQuestion string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	switch stage {
	case IntakeStateWelcome:
		return ibt.handleWelcomeStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStateGoalArea:
		return ibt.handleGoalAreaStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStateMotivation:
		return ibt.handleMotivationStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStatePreferredTime:
		return ibt.handlePreferredTimeStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStatePromptAnchor:
		return ibt.handlePromptAnchorStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStateAdditionalInfo:
		return ibt.handleAdditionalInfoStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	case IntakeStateComplete:
		return ibt.handleCompleteStageWithHistory(ctx, participantID, userResponse, profile, chatHistory)
	default:
		slog.Error("flow.processIntakeStageWithHistory: unknown intake stage", "stage", stage, "participantID", participantID)
		return "", IntakeStateWelcome, fmt.Errorf("unknown intake stage: %s", stage)
	}
}

// handleWelcomeStageWithHistory handles the initial welcome using GenAI with conversation history
func (ibt *IntakeBotTool) handleWelcomeStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStateWelcome, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handleWelcomeStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStateWelcome, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handleWelcomeStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStateWelcome, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Determine next state based on user input
	nextState := IntakeStateWelcome
	if userResponse != "" {
		// Analyze response to determine if user consented or declined
		responseLC := strings.ToLower(strings.TrimSpace(userResponse))
		if strings.Contains(responseLC, "yes") || strings.Contains(responseLC, "ok") || 
		   strings.Contains(responseLC, "sure") || strings.Contains(responseLC, "okay") {
			nextState = IntakeStateGoalArea
		} else if strings.Contains(responseLC, "no") || strings.Contains(responseLC, "not") {
			nextState = IntakeStateComplete
		}
		// If unclear response, stay in welcome state
	}

	return response, nextState, nil
}

// handleGoalAreaStage handles goal/habit area identification
func (ibt *IntakeBotTool) handleGoalAreaStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "What's one habit you've been meaning to build or restart?\n\nYou can choose from:\n• Healthy eating\n• Physical activity\n• Mental well-being\n• Reduce screen time\n• Something else (please specify)", IntakeStateGoalArea, nil
	}

	// Parse and store the target behavior
	response := strings.ToLower(strings.TrimSpace(userResponse))

	if strings.Contains(response, "eat") || strings.Contains(response, "food") || strings.Contains(response, "diet") {
		profile.TargetBehavior = "healthy eating"
	} else if strings.Contains(response, "physical") || strings.Contains(response, "exercise") || strings.Contains(response, "move") || strings.Contains(response, "activity") {
		profile.TargetBehavior = "physical activity"
	} else if strings.Contains(response, "mental") || strings.Contains(response, "stress") || strings.Contains(response, "mindful") || strings.Contains(response, "well") {
		profile.TargetBehavior = "mental well-being"
	} else if strings.Contains(response, "screen") || strings.Contains(response, "phone") || strings.Contains(response, "digital") {
		profile.TargetBehavior = "reduce screen time"
	} else {
		// Custom response - store as-is
		profile.TargetBehavior = userResponse
	}

	return "Perfect! Why does this matter to you now? What would doing this help you feel or achieve?\n\nFor example:\n• \"I want to feel more in control of my day\"\n• \"I've been feeling stuck and need a small win\"\n• \"I want to feel healthier and more energized\"\n• \"It's something I've put off for a while, and I'm ready now\"\n• \"I want to show up better for people around me\"", IntakeStateMotivation, nil
}

// handleGoalAreaStageWithHistory handles goal/habit area identification, considering conversation history
func (ibt *IntakeBotTool) handleGoalAreaStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStateGoalArea, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handleGoalAreaStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStateGoalArea, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handleGoalAreaStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStateGoalArea, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Extract and store target behavior from user response
	if userResponse != "" {
		ibt.extractAndStoreTargetBehavior(userResponse, profile)
	}

	// Determine next state
	nextState := IntakeStateGoalArea
	if userResponse != "" {
		nextState = IntakeStateMotivation
	}

	return response, nextState, nil
}

// handleMotivationStage handles personal motivation identification
func (ibt *IntakeBotTool) handleMotivationStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "Why does this matter to you now? What would doing this help you feel or achieve?", IntakeStateMotivation, nil
	}

	// Store the motivational frame
	profile.MotivationalFrame = userResponse

	return "That's a great reason! When during the day would you like to get a 1-minute nudge from me? You can share:\n• A time block like \"8-9am\" or \"evening\"\n• An exact time like \"9:00am\"\n• \"Randomly during the day\"", IntakeStatePreferredTime, nil
}

// handleMotivationStageWithHistory handles personal motivation identification, considering conversation history
func (ibt *IntakeBotTool) handleMotivationStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStateMotivation, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handleMotivationStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStateMotivation, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handleMotivationStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStateMotivation, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Store motivational frame if provided
	if userResponse != "" {
		profile.MotivationalFrame = userResponse
	}

	// Determine next state
	nextState := IntakeStateMotivation
	if userResponse != "" {
		nextState = IntakeStatePreferredTime
	}

	return response, nextState, nil
}

// handlePreferredTimeStage handles preferred timing identification
func (ibt *IntakeBotTool) handlePreferredTimeStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "When during the day would you like to get a 1-minute nudge from me?", IntakeStatePreferredTime, nil
	}

	// Store the preferred time
	profile.PreferredTime = userResponse

	return "Got it! When do you think this habit would naturally fit into your day? For example:\n• After coffee\n• Before meetings\n• When you feel overwhelmed\n• During work breaks\n• Or anything else that would work for you", IntakeStatePromptAnchor, nil
}

// handlePreferredTimeStageWithHistory handles preferred timing identification, considering conversation history
func (ibt *IntakeBotTool) handlePreferredTimeStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStatePreferredTime, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handlePreferredTimeStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStatePreferredTime, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handlePreferredTimeStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStatePreferredTime, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Store the preferred time
	if userResponse != "" {
		profile.PreferredTime = userResponse
	}

	// Determine next state
	nextState := IntakeStatePreferredTime
	if userResponse != "" {
		nextState = IntakeStatePromptAnchor
	}

	return response, nextState, nil
}

// handlePromptAnchorStage handles habit anchor identification
func (ibt *IntakeBotTool) handlePromptAnchorStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "When do you think this habit would naturally fit into your day?", IntakeStatePromptAnchor, nil
	}

	// Store the prompt anchor
	profile.PromptAnchor = userResponse

	return "Excellent! Is there anything else you'd like me to know that would help personalize your habit suggestion even more?", IntakeStateAdditionalInfo, nil
}

// handlePromptAnchorStageWithHistory handles habit anchor identification, considering conversation history
func (ibt *IntakeBotTool) handlePromptAnchorStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStatePromptAnchor, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handlePromptAnchorStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStatePromptAnchor, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handlePromptAnchorStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStatePromptAnchor, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Store the prompt anchor
	if userResponse != "" {
		profile.PromptAnchor = userResponse
	}

	// Determine next state
	nextState := IntakeStatePromptAnchor
	if userResponse != "" {
		nextState = IntakeStateAdditionalInfo
	}

	return response, nextState, nil
}

// handleAdditionalInfoStage handles additional personalization information
func (ibt *IntakeBotTool) handleAdditionalInfoStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "Is there anything else you'd like me to know that would help personalize your habit suggestion even more?", IntakeStateAdditionalInfo, nil
	}

	// Store additional info if provided and not a simple "no"
	response := strings.ToLower(strings.TrimSpace(userResponse))
	if !strings.Contains(response, "no") && !strings.Contains(response, "nothing") && response != "" {
		profile.AdditionalInfo = userResponse
	}

	return "Great! Thank you for sharing all of that. Would you like to try a 1-minute version of this habit right now? I can send it to you.", IntakeStateComplete, nil
}

// handleAdditionalInfoStageWithHistory handles additional personalization information, considering conversation history
func (ibt *IntakeBotTool) handleAdditionalInfoStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStateAdditionalInfo, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handleAdditionalInfoStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStateAdditionalInfo, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handleAdditionalInfoStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStateAdditionalInfo, fmt.Errorf("failed to generate intake response: %w", err)
	}

	// Store additional info if provided and not a simple "no"
	if userResponse != "" {
		responseLC := strings.ToLower(strings.TrimSpace(userResponse))
		if !strings.Contains(responseLC, "no") && !strings.Contains(responseLC, "nothing") && responseLC != "" {
			profile.AdditionalInfo = userResponse
		}
	}

	// Determine next state
	nextState := IntakeStateAdditionalInfo
	if userResponse != "" {
		nextState = IntakeStateComplete
	}

	return response, nextState, nil
}

// handleCompleteStage handles the completion and potential immediate habit generation
func (ibt *IntakeBotTool) handleCompleteStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "Would you like to try a 1-minute version of this habit right now?", IntakeStateComplete, nil
	}

	response := strings.ToLower(strings.TrimSpace(userResponse))
	if strings.Contains(response, "yes") || strings.Contains(response, "sure") || strings.Contains(response, "okay") {
		// User wants to try the habit now - this would trigger the prompt generator
		return "Perfect! Let me create a personalized 1-minute habit for you right away...", IntakeStateComplete, nil
	} else {
		// User doesn't want to try now
		return "No worries — I'll remind you at your preferred time. You've already taken a great first step by building your personalized profile! 🌱", IntakeStateComplete, nil
	}
}

// handleCompleteStageWithHistory handles the completion and potential immediate habit generation, considering conversation history
func (ibt *IntakeBotTool) handleCompleteStageWithHistory(ctx context.Context, participantID string, userResponse string, profile *UserProfile, chatHistory []openai.ChatCompletionMessageParamUnion) (string, IntakeState, error) {
	// Build messages with conversation history
	messages, err := ibt.buildIntakeMessagesWithHistory(ctx, participantID, IntakeStateComplete, userResponse, chatHistory)
	if err != nil {
		slog.Error("flow.handleCompleteStageWithHistory: failed to build messages", "error", err, "participantID", participantID)
		return "", IntakeStateComplete, fmt.Errorf("failed to build intake messages: %w", err)
	}

	// Generate personalized response using GenAI
	response, err := ibt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.handleCompleteStageWithHistory: GenAI generation failed", "error", err, "participantID", participantID)
		return "", IntakeStateComplete, fmt.Errorf("failed to generate intake response: %w", err)
	}

	return response, IntakeStateComplete, nil
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

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage
func (ibt *IntakeBotTool) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	profile.UpdatedAt = time.Now()
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	return ibt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
}

// getIntakeState retrieves the current intake state
func (ibt *IntakeBotTool) getIntakeState(ctx context.Context, participantID string) (IntakeState, error) {
	stateStr, err := ibt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, "intakeState")
	if err != nil {
		return "", err
	}
	return IntakeState(stateStr), nil
}

// setIntakeState updates the intake state
func (ibt *IntakeBotTool) setIntakeState(ctx context.Context, participantID string, state IntakeState) error {
	return ibt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, "intakeState", string(state))
}

// stringToIntakeState converts a string to IntakeState, handling case variations
func stringToIntakeState(stage string) IntakeState {
	switch strings.ToUpper(strings.TrimSpace(stage)) {
	case "WELCOME":
		return IntakeStateWelcome
	case "GOAL_AREA", "GOALAREA":
		return IntakeStateGoalArea
	case "MOTIVATION":
		return IntakeStateMotivation
	case "PREFERRED_TIME", "PREFERREDTIME":
		return IntakeStatePreferredTime
	case "PROMPT_ANCHOR", "PROMPTANCHOR":
		return IntakeStatePromptAnchor
	case "ADDITIONAL_INFO", "ADDITIONALINFO":
		return IntakeStateAdditionalInfo
	case "COMPLETE":
		return IntakeStateComplete
	default:
		return IntakeState(stage) // Return as-is for error handling
	}
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

// buildIntakeMessages creates OpenAI messages for the intake conversation
func (ibt *IntakeBotTool) buildIntakeMessages(ctx context.Context, participantID string, stage IntakeState, userResponse string) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if ibt.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(ibt.systemPrompt))
	}

	// Add stage-specific context
	stageContext := ibt.getStageContext(stage, userResponse)
	if stageContext != "" {
		messages = append(messages, openai.SystemMessage(stageContext))
	}

	// Get conversation history from the parent conversation flow if available
	// For now, we'll add the current user response if provided
	if userResponse != "" {
		messages = append(messages, openai.UserMessage(userResponse))
	}

	return messages, nil
}

// buildIntakeMessagesWithHistory creates OpenAI messages with conversation history for the intake bot
func (ibt *IntakeBotTool) buildIntakeMessagesWithHistory(ctx context.Context, participantID string, stage IntakeState, userResponse string, chatHistory []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if ibt.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(ibt.systemPrompt))
	}

	// Add stage-specific context
	stageContext := ibt.getStageContext(stage, userResponse)
	if stageContext != "" {
		messages = append(messages, openai.SystemMessage(stageContext))
	}

	// Add conversation history
	messages = append(messages, chatHistory...)

	// Add current user response if provided
	if userResponse != "" {
		messages = append(messages, openai.UserMessage(userResponse))
	}

	return messages, nil
}

// getStageContext returns stage-specific instructions for the AI
func (ibt *IntakeBotTool) getStageContext(stage IntakeState, userResponse string) string {
	switch stage {
	case IntakeStateWelcome:
		if userResponse == "" {
			return "TASK: Greet the user warmly and ask if they'd like help building a 1-minute healthy habit. Explain that you'll ask a few quick questions to personalize it. Keep it brief and friendly."
		}
		return "TASK: The user has responded to your welcome. If they consent (yes/okay/sure), move to asking about what habit they want to build. If they decline, politely acknowledge and end the conversation. If unclear, ask for clarification."

	case IntakeStateGoalArea:
		if userResponse == "" {
			return "TASK: Ask what habit they want to build. Provide some common examples like healthy eating, physical activity, mental well-being, reducing screen time, but encourage them to specify their own ideas too."
		}
		return "TASK: The user has shared what habit area they want to work on. Acknowledge their choice and ask them to be more specific about what exactly they want to do. Provide concrete examples related to their chosen area."

	case IntakeStateMotivation:
		if userResponse == "" {
			return "TASK: Ask why this habit matters to them now. What would doing this help them feel or achieve? Provide examples of emotional motivations like feeling more in control, getting unstuck, feeling healthier, etc."
		}
		return "TASK: The user has shared their motivation. Acknowledge it warmly and move to asking about timing preferences for when they'd like to receive habit prompts."

	case IntakeStatePreferredTime:
		if userResponse == "" {
			return "TASK: Ask when during the day they'd like to get a 1-minute nudge. Give examples like time blocks (8-9am), exact times (9:00am), or 'randomly during the day'."
		}
		return "TASK: The user has shared their preferred timing. Acknowledge it and ask when this habit would naturally fit into their day - what could serve as a trigger or anchor (after coffee, before meetings, during breaks, etc.)."

	case IntakeStatePromptAnchor:
		if userResponse == "" {
			return "TASK: Ask when this habit would naturally fit into their day. What could serve as a trigger or anchor? Provide examples like 'after coffee', 'before meetings', 'when feeling overwhelmed', 'during work breaks'."
		}
		return "TASK: The user has shared their habit anchor. Acknowledge it and ask if there's anything else they'd like you to know to help personalize their habit suggestions even more."

	case IntakeStateAdditionalInfo:
		if userResponse == "" {
			return "TASK: Ask if there's anything else they'd like you to know that would help personalize their habit suggestions even more. Make it optional and encouraging."
		}
		return "TASK: The user has provided additional info (or said no). Thank them for sharing and ask if they'd like to try a 1-minute version of their habit right now."

	case IntakeStateComplete:
		return "TASK: The user has completed the intake. If they want to try the habit now, express enthusiasm. If not, reassure them you'll remind them at their preferred time and celebrate their progress in building their profile."

	default:
		return "TASK: Continue the intake conversation naturally, gathering information to build their personalized habit profile."
	}
}

// extractAndStoreTargetBehavior extracts target behavior from user response and stores it in profile
func (ibt *IntakeBotTool) extractAndStoreTargetBehavior(userResponse string, profile *UserProfile) {
	responseLC := strings.ToLower(strings.TrimSpace(userResponse))
	
	if strings.Contains(responseLC, "eat") || strings.Contains(responseLC, "food") || strings.Contains(responseLC, "diet") {
		profile.TargetBehavior = "healthy eating"
	} else if strings.Contains(responseLC, "physical") || strings.Contains(responseLC, "exercise") || strings.Contains(responseLC, "move") || strings.Contains(responseLC, "activity") {
		profile.TargetBehavior = "physical activity"
	} else if strings.Contains(responseLC, "mental") || strings.Contains(responseLC, "stress") || strings.Contains(responseLC, "mindful") || strings.Contains(responseLC, "well") {
		profile.TargetBehavior = "mental well-being"
	} else if strings.Contains(responseLC, "screen") || strings.Contains(responseLC, "phone") || strings.Contains(responseLC, "digital") {
		profile.TargetBehavior = "reduce screen time"
	} else {
		// Custom response - store as-is
		profile.TargetBehavior = userResponse
	}
}
