// Package flow provides the conversation flow coordinator for the three-bot architecture.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// ConversationFlowCoordinator orchestrates the three-bot architecture:
// intake bot, prompt generator bot, and feedback tracker bot.
type ConversationFlowCoordinator struct {
	stateManager       StateManager
	intakeBot          *IntakeBot
	promptGeneratorBot *PromptGeneratorBot
	feedbackTrackerBot *FeedbackTrackerBot
}

// NewConversationFlowCoordinator creates a new conversation flow coordinator.
func NewConversationFlowCoordinator(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessageService) *ConversationFlowCoordinator {
	return &ConversationFlowCoordinator{
		stateManager:       stateManager,
		intakeBot:          NewIntakeBot(stateManager, genaiClient),
		promptGeneratorBot: NewPromptGeneratorBot(stateManager, genaiClient, msgService),
		feedbackTrackerBot: NewFeedbackTrackerBot(stateManager, genaiClient),
	}
}

// StartConversation initiates a new conversation with the intake bot.
func (cfc *ConversationFlowCoordinator) StartConversation(ctx context.Context, participantID string) (string, error) {
	slog.Info("ConversationFlowCoordinator starting conversation", "participantID", participantID)
	return cfc.intakeBot.StartIntake(ctx, participantID)
}

// ProcessResponse routes responses to the appropriate bot based on current state.
func (cfc *ConversationFlowCoordinator) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	slog.Debug("ConversationFlowCoordinator processing response", "participantID", participantID, "response", response)

	// Get current state to determine which bot should handle the response
	currentState, err := cfc.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	// If no state exists, start intake
	if currentState == "" {
		return cfc.StartConversation(ctx, participantID)
	}

	// Route to appropriate bot based on state
	switch {
	case cfc.isIntakeState(currentState):
		return cfc.handleIntakeState(ctx, participantID, response, currentState)
	case cfc.isPromptGeneratorState(currentState):
		return cfc.promptGeneratorBot.ProcessResponse(ctx, participantID, response)
	case cfc.isFeedbackTrackerState(currentState):
		return cfc.feedbackTrackerBot.ProcessResponse(ctx, participantID, response)
	default:
		return "", fmt.Errorf("unknown conversation state: %s", currentState)
	}
}

// handleIntakeState handles intake bot states with special handling for completion.
func (cfc *ConversationFlowCoordinator) handleIntakeState(ctx context.Context, participantID, response string, currentState models.StateType) (string, error) {
	// Handle intake completion specially
	if currentState == models.StateIntakeComplete {
		return cfc.handleIntakeCompletion(ctx, participantID, response)
	}

	// Regular intake processing
	return cfc.intakeBot.ProcessResponse(ctx, participantID, response)
}

// handleIntakeCompletion handles the "try now" response after intake completion.
func (cfc *ConversationFlowCoordinator) handleIntakeCompletion(ctx context.Context, participantID, response string) (string, error) {
	response = strings.ToLower(strings.TrimSpace(response))

	if strings.Contains(response, "yes") || strings.Contains(response, "sure") || strings.Contains(response, "ok") || strings.Contains(response, "yeah") {
		// User wants to try now - generate and deliver prompt immediately
		slog.Info("User wants to try habit now, generating immediate prompt", "participantID", participantID)

		// Mark user as ready for prompts
		profile, err := cfc.getUserProfile(ctx, participantID)
		if err != nil {
			return "", fmt.Errorf("failed to get user profile: %w", err)
		}

		profile.ReadyForPrompts = true
		if err := cfc.saveUserProfile(ctx, participantID, profile); err != nil {
			slog.Warn("Failed to update profile ready status", "error", err, "participantID", participantID)
		}

		// Generate and deliver the prompt immediately
		_, err = cfc.promptGeneratorBot.GenerateAndDeliverPrompt(ctx, participantID, "immediate")
		if err != nil {
			return "", fmt.Errorf("failed to generate immediate prompt: %w", err)
		}

		return "Perfect! I've sent you a personalized habit suggestion. Give it a try when you're ready!", nil

	} else {
		// User doesn't want to try now - just store profile and end gracefully
		slog.Info("User declined to try now, ending intake", "participantID", participantID)

		profile, err := cfc.getUserProfile(ctx, participantID)
		if err != nil {
			return "", fmt.Errorf("failed to get user profile: %w", err)
		}

		profile.ReadyForPrompts = true // Still ready for scheduled prompts
		if err := cfc.saveUserProfile(ctx, participantID, profile); err != nil {
			slog.Warn("Failed to update profile ready status", "error", err, "participantID", participantID)
		}

		// Set state back to active for future interactions
		err = cfc.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)
		if err != nil {
			return "", fmt.Errorf("failed to reset state: %w", err)
		}

		return "No worries! I'll remind you at your preferred time. You've already taken a great first step by setting up your personalized habit profile!", nil
	}
}

// isIntakeState checks if the current state belongs to the intake bot.
func (cfc *ConversationFlowCoordinator) isIntakeState(state models.StateType) bool {
	intakeStates := []models.StateType{
		models.StateIntakeWelcome,
		models.StateIntakeHabitDomain,
		models.StateIntakeMotivation,
		models.StateIntakeExistingGoal,
		models.StateIntakeSuggestOptions,
		models.StateIntakePreference,
		models.StateIntakeOutcome,
		models.StateIntakeLanguage,
		models.StateIntakeTone,
		models.StateIntakeComplete,
	}

	for _, intakeState := range intakeStates {
		if state == intakeState {
			return true
		}
	}
	return false
}

// isPromptGeneratorState checks if the current state belongs to the prompt generator bot.
func (cfc *ConversationFlowCoordinator) isPromptGeneratorState(state models.StateType) bool {
	promptStates := []models.StateType{
		models.StatePromptGenerate,
		models.StatePromptDelivered,
		models.StatePromptTryNow,
	}

	for _, promptState := range promptStates {
		if state == promptState {
			return true
		}
	}
	return false
}

// isFeedbackTrackerState checks if the current state belongs to the feedback tracker bot.
func (cfc *ConversationFlowCoordinator) isFeedbackTrackerState(state models.StateType) bool {
	feedbackStates := []models.StateType{
		models.StateFeedbackAwaitingCompletion,
		models.StateFeedbackProcessing,
	}

	for _, feedbackState := range feedbackStates {
		if state == feedbackState {
			return true
		}
	}
	return false
}

// getUserProfile retrieves and parses the user profile from state storage.
func (cfc *ConversationFlowCoordinator) getUserProfile(ctx context.Context, participantID string) (*models.UserProfile, error) {
	profileJSON, err := cfc.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	if profileJSON == "" {
		return nil, fmt.Errorf("user profile not found for participant %s", participantID)
	}

	var profile models.UserProfile
	if err := profile.FromJSON(profileJSON); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage.
func (cfc *ConversationFlowCoordinator) saveUserProfile(ctx context.Context, participantID string, profile *models.UserProfile) error {
	profileJSON, err := profile.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user profile: %w", err)
	}

	return cfc.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, profileJSON)
}

// GetPromptGeneratorBot returns the prompt generator bot for external use (e.g., scheduled prompts).
func (cfc *ConversationFlowCoordinator) GetPromptGeneratorBot() *PromptGeneratorBot {
	return cfc.promptGeneratorBot
}
