// Package flow provides the feedback tracker bot implementation for profile updates.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// FeedbackTrackerBot implements the feedback tracker bot functionality for
// processing user feedback and updating profiles based on interactions.
type FeedbackTrackerBot struct {
	stateManager StateManager
	genaiClient  genai.ClientInterface
}

// NewFeedbackTrackerBot creates a new feedback tracker bot instance.
func NewFeedbackTrackerBot(stateManager StateManager, genaiClient genai.ClientInterface) *FeedbackTrackerBot {
	return &FeedbackTrackerBot{
		stateManager: stateManager,
		genaiClient:  genaiClient,
	}
}

// ProcessResponse processes user feedback and updates the profile accordingly.
// This implements the feedback tracker logic as specified in the design doc.
func (ftb *FeedbackTrackerBot) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	slog.Debug("FeedbackTrackerBot processing response", "participantID", participantID, "response", response)

	// Get current state
	currentState, err := ftb.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	switch currentState {
	case models.StateFeedbackAwaitingCompletion:
		return ftb.handleFeedbackResponse(ctx, participantID, response)
	case models.StateFeedbackProcessing:
		return ftb.handleFollowUpResponse(ctx, participantID, response)
	default:
		return "", fmt.Errorf("unexpected state for feedback tracker: %s", currentState)
	}
}

// handleFeedbackResponse processes the initial feedback response (what made it work / what got in the way).
func (ftb *FeedbackTrackerBot) handleFeedbackResponse(ctx context.Context, participantID, response string) (string, error) {
	// Get current profile
	profile, err := ftb.getUserProfile(ctx, participantID)
	if err != nil {
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Update profile with the feedback
	wasSuccessful := ftb.analyzeSuccessFromPreviousInteraction(profile)

	if wasSuccessful {
		// Store what made it work
		profile.LastFeedback = fmt.Sprintf("What worked: %s", response)
	} else {
		// Store what got in the way as a barrier
		profile.LastBarrier = response
		profile.LastFeedback = fmt.Sprintf("Barrier: %s", response)
	}

	// Generate an updated profile using AI analysis
	updatedProfile, summary, err := ftb.updateProfileWithAI(ctx, profile, response, wasSuccessful)
	if err != nil {
		slog.Warn("Failed to update profile with AI, using manual updates", "error", err, "participantID", participantID)
		updatedProfile = profile // Fall back to manual updates
	}

	// Save the updated profile
	if err := ftb.saveUserProfile(ctx, participantID, updatedProfile); err != nil {
		return "", fmt.Errorf("failed to save updated profile: %w", err)
	}

	// Set state back to ready for next interaction
	err = ftb.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive)
	if err != nil {
		return "", fmt.Errorf("failed to reset state: %w", err)
	}

	// Prepare response message
	var responseMessage string
	if wasSuccessful {
		responseMessage = "That's wonderful! I'm glad it worked well for you. I've noted what made it successful and will keep that in mind for future suggestions."
	} else {
		responseMessage = "I understand - that makes total sense. I've noted what got in the way and will adjust future suggestions to work better for you."
	}

	if summary != "" {
		responseMessage += fmt.Sprintf("\n\n(Profile updated: %s)", summary)
	}

	slog.Info("Feedback processed and profile updated", "participantID", participantID, "wasSuccessful", wasSuccessful)
	return responseMessage, nil
}

// handleFollowUpResponse processes follow-up responses for additional context.
func (ftb *FeedbackTrackerBot) handleFollowUpResponse(ctx context.Context, participantID, response string) (string, error) {
	// This can be used for additional feedback collection if needed
	// For now, treat it similar to the main feedback response
	return ftb.handleFeedbackResponse(ctx, participantID, response)
}

// analyzeSuccessFromPreviousInteraction determines if the previous interaction was successful
// based on the profile's feedback tracking.
func (ftb *FeedbackTrackerBot) analyzeSuccessFromPreviousInteraction(profile *models.UserProfile) bool {
	// Check if the last feedback indicated success
	if profile.LastFeedback == "" {
		return false
	}

	lastFeedback := strings.ToLower(profile.LastFeedback)
	successIndicators := []string{"yes", "tried", "did", "worked", "good", "helped", "successful"}

	for _, indicator := range successIndicators {
		if strings.Contains(lastFeedback, indicator) {
			return true
		}
	}

	return false
}

// updateProfileWithAI uses AI to analyze feedback and update the user profile intelligently.
func (ftb *FeedbackTrackerBot) updateProfileWithAI(ctx context.Context, profile *models.UserProfile, feedback string, wasSuccessful bool) (*models.UserProfile, string, error) {
	systemPrompt := `You are a feedback tracker and profile updater. You receive:
- The user's current profile
- The 1-minute prompt they received  
- Their follow-up responses: whether they tried it, why/why not, and any suggested changes

Your job is to:
1. Detect if the user was successful in doing the prompt, or adapted the prompt, why/why not
2. Update fields like prompt anchor, time, motivation, or tone if preferences changed
3. Optionally add fields like last_successful_prompt, last_barrier, or last_tweak
4. Return an updated profile analysis and a short, plain-language summary of what changed

Respond with JSON containing:
{
  "analysis": "Brief analysis of what this feedback tells us about the user",
  "suggested_updates": {
    "field_name": "new_value_if_changed"
  },
  "summary": "Plain language summary of what changed"
}

Focus on actionable insights that will improve future prompts.`

	currentProfileJSON, _ := profile.ToJSON()

	userPrompt := fmt.Sprintf(`Current user profile:
%s

Last prompt sent: %s
User feedback: %s
Was successful: %t

Analyze this feedback and suggest profile updates.`,
		currentProfileJSON,
		profile.LastPrompt,
		feedback,
		wasSuccessful)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	_, err := ftb.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return profile, "", fmt.Errorf("AI analysis failed: %w", err)
	}

	// Parse the AI response and apply updates
	// For now, return the original profile with manual updates
	// TODO: Parse JSON response and apply suggested updates

	updatedProfile := *profile // Copy the profile

	// Apply manual updates based on success/failure
	if wasSuccessful {
		updatedProfile.SuccessCount++
		if profile.LastPrompt != "" {
			updatedProfile.LastSuccessfulPrompt = profile.LastPrompt
		}
	} else {
		// Store the barrier for future reference
		updatedProfile.LastBarrier = feedback
	}

	updatedProfile.FeedbackCount++

	summary := fmt.Sprintf("Feedback processed (%d total interactions)", updatedProfile.FeedbackCount)
	if wasSuccessful {
		summary += fmt.Sprintf(", %d successful", updatedProfile.SuccessCount)
	}

	return &updatedProfile, summary, nil
}

// getUserProfile retrieves and parses the user profile from state storage.
func (ftb *FeedbackTrackerBot) getUserProfile(ctx context.Context, participantID string) (*models.UserProfile, error) {
	profileJSON, err := ftb.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
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
func (ftb *FeedbackTrackerBot) saveUserProfile(ctx context.Context, participantID string, profile *models.UserProfile) error {
	profileJSON, err := profile.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user profile: %w", err)
	}

	return ftb.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, profileJSON)
}
