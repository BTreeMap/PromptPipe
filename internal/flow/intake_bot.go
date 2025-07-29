// Package flow provides the intake bot implementation for user profile building.
package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// IntakeBot implements the intake bot functionality for building user profiles
// through a structured conversation as specified in the design doc.
type IntakeBot struct {
	stateManager StateManager
	genaiClient  genai.ClientInterface
}

// NewIntakeBot creates a new intake bot instance.
func NewIntakeBot(stateManager StateManager, genaiClient genai.ClientInterface) *IntakeBot {
	return &IntakeBot{
		stateManager: stateManager,
		genaiClient:  genaiClient,
	}
}

// StartIntake initiates the intake conversation with a participant using LLM.
func (ib *IntakeBot) StartIntake(ctx context.Context, participantID string) (string, error) {
	slog.Info("IntakeBot starting intake conversation", "participantID", participantID)

	// Initialize empty user profile
	profile := &models.UserProfile{
		ParticipantID: participantID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Save the profile
	if err := ib.saveUserProfile(ctx, participantID, profile); err != nil {
		return "", fmt.Errorf("failed to save initial profile: %w", err)
	}

	// Set initial state to intake welcome
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeWelcome)
	if err != nil {
		return "", fmt.Errorf("failed to set initial state: %w", err)
	}

	// Initialize conversation history with system prompt
	history := &ConversationHistory{
		Messages: []ConversationMessage{},
	}

	// Save empty conversation history
	if err := ib.saveConversationHistory(ctx, participantID, history); err != nil {
		return "", fmt.Errorf("failed to save initial conversation history: %w", err)
	}

	// Generate initial welcome message using LLM with system prompt
	systemPrompt := ib.getSystemPrompt()
	userPrompt := "START_INTAKE_CONVERSATION" // Special trigger for initial message

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	response, err := ib.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate welcome message: %w", err)
	}

	// Save assistant's welcome message to history
	history.Messages = append(history.Messages, ConversationMessage{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	if err := ib.saveConversationHistory(ctx, participantID, history); err != nil {
		slog.Warn("Failed to save welcome message to history", "error", err)
	}

	slog.Debug("IntakeBot sent LLM-generated welcome message", "participantID", participantID)
	return response, nil
}

// ProcessResponse processes a user response during the intake conversation using LLM.
func (ib *IntakeBot) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	slog.Debug("IntakeBot processing response with LLM", "participantID", participantID, "response", response)

	// Get current conversation history
	history, err := ib.getConversationHistory(ctx, participantID)
	if err != nil {
		return "", fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Add user's response to history
	history.Messages = append(history.Messages, ConversationMessage{
		Role:      "user",
		Content:   response,
		Timestamp: time.Now(),
	})

	// Build messages for LLM including system prompt and conversation history
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(ib.getSystemPrompt()),
	}

	// Add conversation history to messages
	for _, msg := range history.Messages {
		if msg.Role == "user" {
			messages = append(messages, openai.UserMessage(msg.Content))
		} else if msg.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	// Generate response using LLM
	llmResponse, err := ib.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate LLM response: %w", err)
	}

	// Add assistant's response to history
	history.Messages = append(history.Messages, ConversationMessage{
		Role:      "assistant",
		Content:   llmResponse,
		Timestamp: time.Now(),
	})

	// Save updated conversation history
	if err := ib.saveConversationHistory(ctx, participantID, history); err != nil {
		slog.Warn("Failed to save updated conversation history", "error", err)
	}

	// Check if intake is complete by analyzing the response
	if ib.isIntakeComplete(llmResponse) {
		// Update user profile based on the conversation
		if err := ib.updateUserProfileFromConversation(ctx, participantID, history); err != nil {
			slog.Warn("Failed to update user profile from conversation", "error", err)
		}

		// Set state to intake complete
		if err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeComplete); err != nil {
			slog.Warn("Failed to set intake complete state", "error", err)
		}
	}

	slog.Debug("IntakeBot generated LLM response", "participantID", participantID, "responseLength", len(llmResponse))
	return llmResponse, nil
}

// handleWelcomeResponse processes the response to the welcome message.
func (ib *IntakeBot) handleWelcomeResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Check if user agrees to proceed
	response = strings.ToLower(strings.TrimSpace(response))
	if strings.Contains(response, "yes") || strings.Contains(response, "ok") || strings.Contains(response, "sure") || strings.Contains(response, "yeah") {
		// Move to habit domain question
		err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeHabitDomain)
		if err != nil {
			return "", err
		}

		return "Great! Which of these four areas would you like to focus on?\n\n• Eating healthy\n• Mindful screen time\n• Physical activity\n• Mental well-being", nil
	}

	// If they decline, end gracefully
	return "No worries! Feel free to reach out anytime if you'd like help building a healthy habit. Take care!", nil
}

// handleHabitDomainResponse processes the habit domain selection.
func (ib *IntakeBot) handleHabitDomainResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Store the target behavior
	profile.TargetBehavior = response
	profile.UpdatedAt = time.Now()

	if err := ib.saveUserProfile(ctx, participantID, profile); err != nil {
		return "", err
	}

	// Move to motivation question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeMotivation)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Great choice! Why did you pick %s? What makes this important to you right now?", strings.ToLower(response)), nil
}

// handleMotivationResponse processes the motivation response.
func (ib *IntakeBot) handleMotivationResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Store the motivational frame
	profile.MotivationalFrame = response
	profile.UpdatedAt = time.Now()

	if err := ib.saveUserProfile(ctx, participantID, profile); err != nil {
		return "", err
	}

	// Move to existing goal question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeExistingGoal)
	if err != nil {
		return "", err
	}

	return "That makes total sense! What's one habit you've been meaning to build or restart?", nil
}

// handleExistingGoalResponse processes the existing goal response.
func (ib *IntakeBot) handleExistingGoalResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Generate 2-3 activity options based on their domain and goal using GenAI
	suggestions, err := ib.generateActivitySuggestions(ctx, profile.TargetBehavior, response)
	if err != nil {
		slog.Warn("Failed to generate AI suggestions, using fallback", "error", err, "participantID", participantID)
		suggestions = ib.getFallbackSuggestions(profile.TargetBehavior)
	}

	// Move to suggest options state
	err = ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeSuggestOptions)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Perfect! Here are 2-3 simple 1-minute options for %s:\n\n%s\n\nWhich one feels most doable for you?", profile.TargetBehavior, suggestions), nil
}

// handleOptionsResponse processes the option selection response.
func (ib *IntakeBot) handleOptionsResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Move to preference question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakePreference)
	if err != nil {
		return "", err
	}

	return "Great choice! Why did you pick that one? What makes it feel right for you?", nil
}

// handlePreferenceResponse processes the preference response.
func (ib *IntakeBot) handlePreferenceResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Move to outcome question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeOutcome)
	if err != nil {
		return "", err
	}

	return "What outcome or feeling would make this habit worth doing? For example: to feel calm, energized, more focused, or something else entirely?", nil
}

// handleOutcomeResponse processes the outcome response.
func (ib *IntakeBot) handleOutcomeResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Move to language question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeLanguage)
	if err != nil {
		return "", err
	}

	return "What kind of language or benefit helps you take action? For example: 'to feel calm' vs 'to move with energy' - what resonates with you?", nil
}

// handleLanguageResponse processes the language preference response.
func (ib *IntakeBot) handleLanguageResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Move to tone question
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeTone)
	if err != nil {
		return "", err
	}

	return "Last question! What tone do you prefer for your habit reminders? For example: reflective, direct, playful, or something else?", nil
}

// handleToneResponse processes the final tone response and completes intake.
func (ib *IntakeBot) handleToneResponse(ctx context.Context, participantID, response string, profile *models.UserProfile) (string, error) {
	// Store the final pieces and mark intake complete
	profile.AdditionalNotes = fmt.Sprintf("Tone preference: %s", response)
	profile.IntakeComplete = true
	profile.UpdatedAt = time.Now()

	if err := ib.saveUserProfile(ctx, participantID, profile); err != nil {
		return "", err
	}

	// Move to completion state
	err := ib.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateIntakeComplete)
	if err != nil {
		return "", err
	}

	// Ask if they want to try the habit now
	return "Perfect! Thank you for sharing all of that. I have everything I need to create personalized habit nudges for you.\n\nGreat! Would you like to try a 1-minute version of this habit right now? I can send it to you.", nil
}

// generateActivitySuggestions uses GenAI to generate personalized activity suggestions.
func (ib *IntakeBot) generateActivitySuggestions(ctx context.Context, targetBehavior, existingGoal string) (string, error) {
	systemPrompt := `You are a micro-habit expert. Generate 2-3 simple, 1-minute activity suggestions based on the user's target behavior and existing goal. Each suggestion should be:
- Extremely easy to do in 1 minute
- Specific and actionable
- Relevant to their target behavior
- Formatted as bullet points

Keep suggestions concise and practical.`

	userPrompt := fmt.Sprintf("Target behavior: %s\nExisting goal: %s\n\nGenerate 2-3 simple 1-minute activity options:", targetBehavior, existingGoal)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	response, err := ib.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		return "", err
	}

	return response, nil
}

// getFallbackSuggestions provides fallback suggestions if GenAI fails.
func (ib *IntakeBot) getFallbackSuggestions(targetBehavior string) string {
	behavior := strings.ToLower(targetBehavior)

	switch {
	case strings.Contains(behavior, "eating") || strings.Contains(behavior, "nutrition"):
		return "• Drink a full glass of water before your next meal\n• Take 3 mindful bites, chewing slowly\n• Add one piece of fruit to your next snack"
	case strings.Contains(behavior, "physical") || strings.Contains(behavior, "exercise"):
		return "• Do 10 jumping jacks or arm circles\n• Take a 1-minute walk around your space\n• Stretch your arms and shoulders"
	case strings.Contains(behavior, "mental") || strings.Contains(behavior, "well-being"):
		return "• Take 5 deep breaths with eyes closed\n• Write down one thing you're grateful for\n• Do a 1-minute body scan"
	case strings.Contains(behavior, "screen") || strings.Contains(behavior, "technology"):
		return "• Look away from your screen for 20 seconds\n• Do the 20-20-20 rule (look 20 feet away for 20 seconds)\n• Close one unnecessary browser tab or app"
	default:
		return "• Take 3 deep breaths and set an intention\n• Write down one small goal for today\n• Spend 1 minute organizing your immediate space"
	}
}

// getUserProfile retrieves and parses the user profile from state storage.
func (ib *IntakeBot) getUserProfile(ctx context.Context, participantID string) (*models.UserProfile, error) {
	profileJSON, err := ib.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	var profile models.UserProfile
	if profileJSON == "" {
		// Return empty profile if none exists
		return &models.UserProfile{
			ParticipantID: participantID,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}, nil
	}

	if err := profile.FromJSON(profileJSON); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage.
func (ib *IntakeBot) saveUserProfile(ctx context.Context, participantID string, profile *models.UserProfile) error {
	profileJSON, err := profile.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize user profile: %w", err)
	}

	return ib.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, profileJSON)
}
