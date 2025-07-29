// Package flow provides intake bot tool functionality for building structured user profiles.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	stateManager StateManager
	genaiClient  genai.ClientInterface
	msgService   MessagingService
}

// NewIntakeBotTool creates a new intake bot tool instance.
func NewIntakeBotTool(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService) *IntakeBotTool {
	slog.Debug("flow.NewIntakeBotTool: creating intake bot tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil)
	return &IntakeBotTool{
		stateManager: stateManager,
		genaiClient:  genaiClient,
		msgService:   msgService,
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
	response, newState, err := ibt.processIntakeStage(ctx, participantID, stringToIntakeState(conversationStage), userResponse, nextQuestion, profile)
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

// processIntakeStage handles the logic for each stage of the intake conversation
func (ibt *IntakeBotTool) processIntakeStage(ctx context.Context, participantID string, stage IntakeState, userResponse, nextQuestion string, profile *UserProfile) (string, IntakeState, error) {
	switch stage {
	case IntakeStateWelcome:
		return ibt.handleWelcomeStage(ctx, participantID, userResponse, profile)
	case IntakeStateGoalArea:
		return ibt.handleGoalAreaStage(ctx, participantID, userResponse, profile)
	case IntakeStateMotivation:
		return ibt.handleMotivationStage(ctx, participantID, userResponse, profile)
	case IntakeStatePreferredTime:
		return ibt.handlePreferredTimeStage(ctx, participantID, userResponse, profile)
	case IntakeStatePromptAnchor:
		return ibt.handlePromptAnchorStage(ctx, participantID, userResponse, profile)
	case IntakeStateAdditionalInfo:
		return ibt.handleAdditionalInfoStage(ctx, participantID, userResponse, profile)
	case IntakeStateComplete:
		return ibt.handleCompleteStage(ctx, participantID, userResponse, profile)
	default:
		return "", stage, fmt.Errorf("unknown intake stage: %s", stage)
	}
}

// handleWelcomeStage handles the initial welcome and consent
func (ibt *IntakeBotTool) handleWelcomeStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		// Initial welcome message
		return "Hi! I'm your micro-coach bot here to help you build a 1-minute healthy habit that fits into your day. I'll ask a few quick questions to personalize it. Is that okay?", IntakeStateWelcome, nil
	}

	// Check if user consents
	response := strings.ToLower(strings.TrimSpace(userResponse))
	if strings.Contains(response, "yes") || strings.Contains(response, "ok") || strings.Contains(response, "sure") || strings.Contains(response, "okay") {
		return "Great! Let's start. What's one habit you've been meaning to build or restart?\n\nYou can choose from:\n• Healthy eating\n• Physical activity\n• Mental well-being\n• Reduce screen time\n• Something else (please specify)", IntakeStateGoalArea, nil
	} else if strings.Contains(response, "no") || strings.Contains(response, "not") {
		return "No problem! Feel free to come back anytime when you're ready to explore building a healthy habit. Take care!", IntakeStateComplete, nil
	} else {
		// Ask for clarification
		return "I'd love to help you build a personalized habit! Would you like me to ask a few quick questions to get started? Just say 'yes' or 'no'.", IntakeStateWelcome, nil
	}
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

// handleMotivationStage handles personal motivation identification
func (ibt *IntakeBotTool) handleMotivationStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "Why does this matter to you now? What would doing this help you feel or achieve?", IntakeStateMotivation, nil
	}

	// Store the motivational frame
	profile.MotivationalFrame = userResponse

	return "That's a great reason! When during the day would you like to get a 1-minute nudge from me? You can share:\n• A time block like \"8-9am\" or \"evening\"\n• An exact time like \"9:00am\"\n• \"Randomly during the day\"", IntakeStatePreferredTime, nil
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

// handlePromptAnchorStage handles habit anchor identification
func (ibt *IntakeBotTool) handlePromptAnchorStage(ctx context.Context, participantID string, userResponse string, profile *UserProfile) (string, IntakeState, error) {
	if userResponse == "" {
		return "When do you think this habit would naturally fit into your day?", IntakeStatePromptAnchor, nil
	}

	// Store the prompt anchor
	profile.PromptAnchor = userResponse

	return "Excellent! Is there anything else you'd like me to know that would help personalize your habit suggestion even more?", IntakeStateAdditionalInfo, nil
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
