// Package flow provides the prompt generator bot implementation for habit nudge creation.
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

// MessageService defines the interface for sending messages to avoid import cycles.
type MessageService interface {
	// SendMessage sends a message to the specified phone number
	SendMessage(ctx context.Context, to, message string) error
	// ValidateAndCanonicalizeRecipient validates and canonicalizes a phone number
	ValidateAndCanonicalizeRecipient(recipient string) (string, error)
}

// PromptGeneratorBot implements the prompt generator bot functionality for creating
// and delivering personalized habit nudges based on user profiles.
type PromptGeneratorBot struct {
	stateManager StateManager
	genaiClient  genai.ClientInterface
	msgService   MessageService
}

// NewPromptGeneratorBot creates a new prompt generator bot instance.
func NewPromptGeneratorBot(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessageService) *PromptGeneratorBot {
	return &PromptGeneratorBot{
		stateManager: stateManager,
		genaiClient:  genaiClient,
		msgService:   msgService,
	}
}

// GenerateAndDeliverPrompt generates a personalized habit prompt and delivers it to the user.
// This can be called immediately after intake completion or via scheduled delivery.
func (pgb *PromptGeneratorBot) GenerateAndDeliverPrompt(ctx context.Context, participantID string, deliveryMode string) (string, error) {
	slog.Info("PromptGeneratorBot generating and delivering prompt", "participantID", participantID, "deliveryMode", deliveryMode)

	// Get user profile
	profile, err := pgb.getUserProfile(ctx, participantID)
	if err != nil {
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	if !profile.IntakeComplete {
		return "", fmt.Errorf("cannot generate prompt: intake not complete for participant %s", participantID)
	}

	// Generate the personalized habit prompt
	habitPrompt, err := pgb.generateHabitPrompt(ctx, profile, deliveryMode)
	if err != nil {
		return "", fmt.Errorf("failed to generate habit prompt: %w", err)
	}

	// Get phone number from context for message delivery
	phoneNumber, ok := GetPhoneNumberFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("phone number not found in context for participant %s", participantID)
	}

	// Send the habit prompt to the user
	if err := pgb.msgService.SendMessage(ctx, phoneNumber, habitPrompt); err != nil {
		return "", fmt.Errorf("failed to send habit prompt: %w", err)
	}

	// Update profile with prompt details
	profile.UpdatePrompt(habitPrompt, false) // Will be marked successful if user responds positively

	if err := pgb.saveUserProfile(ctx, participantID, profile); err != nil {
		slog.Warn("Failed to save updated profile after prompt delivery", "error", err, "participantID", participantID)
	}

	// Set state to prompt delivered and wait for completion response
	err = pgb.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StatePromptDelivered)
	if err != nil {
		return "", fmt.Errorf("failed to update state after prompt delivery: %w", err)
	}

	slog.Info("Habit prompt generated and delivered successfully", "participantID", participantID, "promptLength", len(habitPrompt))

	// Return confirmation message for logging/debugging
	return fmt.Sprintf("Habit prompt delivered successfully to participant %s", participantID), nil
}

// ProcessResponse handles responses to delivered prompts, following the design doc workflow.
func (pgb *PromptGeneratorBot) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	slog.Debug("PromptGeneratorBot processing response", "participantID", participantID, "response", response)

	// Get current state
	currentState, err := pgb.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	switch currentState {
	case models.StatePromptDelivered:
		return pgb.handlePromptResponse(ctx, participantID, response)
	case models.StatePromptTryNow:
		return pgb.handleTryNowResponse(ctx, participantID, response)
	default:
		return "", fmt.Errorf("unexpected state for prompt generator: %s", currentState)
	}
}

// handlePromptResponse processes the initial response to a delivered habit prompt.
func (pgb *PromptGeneratorBot) handlePromptResponse(ctx context.Context, participantID, response string) (string, error) {
	// Ask if they got a chance to try it
	err := pgb.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StatePromptTryNow)
	if err != nil {
		return "", err
	}

	return "Did you get a chance to try it?", nil
}

// handleTryNowResponse processes the response to "Did you get a chance to try it?"
func (pgb *PromptGeneratorBot) handleTryNowResponse(ctx context.Context, participantID, response string) (string, error) {
	// Get current profile
	profile, err := pgb.getUserProfile(ctx, participantID)
	if err != nil {
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))

	var nextMessage string
	var wasSuccessful bool

	if strings.Contains(response, "yes") || strings.Contains(response, "yeah") || strings.Contains(response, "yep") || strings.Contains(response, "tried") || strings.Contains(response, "did") {
		// They tried it - ask what made it work
		nextMessage = "What made it work well?"
		wasSuccessful = true
		// Mark the last prompt as successful
		if profile.LastPrompt != "" {
			profile.LastSuccessfulPrompt = profile.LastPrompt
		}
	} else {
		// They didn't try it - ask what got in the way
		nextMessage = "What got in the way?"
		wasSuccessful = false
	}

	// Update profile with feedback tracking
	profile.UpdateLastFeedback(response, wasSuccessful)

	// Save updated profile
	if err := pgb.saveUserProfile(ctx, participantID, profile); err != nil {
		slog.Warn("Failed to save profile after try now response", "error", err, "participantID", participantID)
	}

	// Transition to feedback tracker - this will be handled by the feedback tracker bot
	err = pgb.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateFeedbackAwaitingCompletion)
	if err != nil {
		return "", fmt.Errorf("failed to transition to feedback state: %w", err)
	}

	return nextMessage, nil
}

// generateHabitPrompt creates a personalized habit prompt using the user's profile.
func (pgb *PromptGeneratorBot) generateHabitPrompt(ctx context.Context, profile *models.UserProfile, deliveryMode string) (string, error) {
	// Build system prompt as specified in the design doc
	systemPrompt := `You are a warm, supportive micro-coach bot. Your job is to craft a short, personal, 1-minute healthy habit suggestion using the user's profile.

All suggestions must be short and skimmable in <30 words. Think in terms of what a user can read, understand, and act on in under 10 seconds. Avoid long paragraphs or explanations.

When suggesting a habit:
- Anchor it to an existing routine the user mentioned (Prompt)
- Make it extremely easy to do in 1 minute (Ability)  
- Explain why it helps the user, based on what motivates them (Motivation)

Do not skip any of the 3 parts (MAP).

Output format:
"After or before {{prompt_anchor}}, try {{1-minute action}} — it helps you {{motivational frame}}. Would that feel doable?"

Adapt the language and tone to the user's preferences.
Include only the prompt. Do not include explanations or system messages.`

	// Build user prompt with profile data
	userPrompt := fmt.Sprintf(`Generate a 1-minute healthy habit suggestion using this user profile:

Target behavior: %s
Motivational frame: %s
Preferred time: %s
Prompt anchor: %s
Additional notes: %s

Create a personalized habit prompt now.`,
		profile.TargetBehavior,
		profile.MotivationalFrame,
		profile.PreferredTime,
		profile.PromptAnchor,
		profile.AdditionalNotes)

	// Add delivery mode context
	if deliveryMode == "immediate" {
		userPrompt += "\n\nDelivery mode: Immediate (user wants to try now)"
	} else {
		userPrompt += "\n\nDelivery mode: Scheduled (delivered at preferred time)"
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	response, err := pgb.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate habit prompt with AI: %w", err)
	}

	return response, nil
}

// getUserProfile retrieves and parses the user profile from state storage.
func (pgb *PromptGeneratorBot) getUserProfile(ctx context.Context, participantID string) (*models.UserProfile, error) {
	profileJSON, err := pgb.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
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
func (pgb *PromptGeneratorBot) saveUserProfile(ctx context.Context, participantID string, profile *models.UserProfile) error {
	profileJSON, err := profile.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user profile: %w", err)
	}

	return pgb.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, profileJSON)
}
