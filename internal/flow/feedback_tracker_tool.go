// Package flow provides feedback tracker tool functionality for conversation flows.
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

// UserProfile represents the structured user profile built by the intake bot
type UserProfile struct {
	HabitDomain       string    `json:"habit_domain"`       // e.g., "healthy eating", "physical activity"
	MotivationalFrame string    `json:"motivational_frame"` // User's personal motivation
	PreferredTime     string    `json:"preferred_time"`     // Time window for nudging
	PromptAnchor      string    `json:"prompt_anchor"`      // When habit fits naturally
	AdditionalInfo    string    `json:"additional_info"`    // Any extra personalization info
	CreatedAt         time.Time `json:"created_at"`         // When profile was created
	UpdatedAt         time.Time `json:"updated_at"`         // Last profile update

	// Feedback tracking fields
	LastSuccessfulPrompt string `json:"last_successful_prompt,omitempty"` // Last prompt that worked
	LastBarrier          string `json:"last_barrier,omitempty"`           // Last reported barrier
	LastTweak            string `json:"last_tweak,omitempty"`             // Last requested modification
	SuccessCount         int    `json:"success_count"`                    // Number of successful completions
	TotalPrompts         int    `json:"total_prompts"`                    // Total prompts sent
}

// FeedbackTrackerTool provides LLM tool functionality for tracking user feedback and updating profiles.
type FeedbackTrackerTool struct {
	stateManager           StateManager
	genaiClient            genai.ClientInterface
	systemPromptFile       string
	systemPrompt           string
	timer                  models.Timer     // Timer for scheduling feedback timeouts
	msgService             MessagingService // Messaging service for sending follow-up prompts
	feedbackInitialTimeout string           // Timeout for initial feedback response (e.g., "15m")
	feedbackFollowupDelay  string           // Delay before follow-up feedback session (e.g., "3h")
}

// NewFeedbackTrackerTool creates a new feedback tracker tool instance.
func NewFeedbackTrackerTool(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string) *FeedbackTrackerTool {
	slog.Debug("FeedbackTrackerTool.NewFeedbackTrackerTool: creating feedback tracker tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "systemPromptFile", systemPromptFile)
	return &FeedbackTrackerTool{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
	}
}

// NewFeedbackTrackerToolWithTimeouts creates a new feedback tracker tool instance with timeout configuration.
func NewFeedbackTrackerToolWithTimeouts(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, timer models.Timer, msgService MessagingService, feedbackInitialTimeout, feedbackFollowupDelay string) *FeedbackTrackerTool {
	slog.Debug("flow.NewFeedbackTrackerToolWithTimeouts: creating feedback tracker tool with timeouts",
		"hasStateManager", stateManager != nil,
		"hasGenAI", genaiClient != nil,
		"systemPromptFile", systemPromptFile,
		"hasTimer", timer != nil,
		"hasMessaging", msgService != nil,
		"feedbackInitialTimeout", feedbackInitialTimeout,
		"feedbackFollowupDelay", feedbackFollowupDelay)
	return &FeedbackTrackerTool{
		stateManager:           stateManager,
		genaiClient:            genaiClient,
		systemPromptFile:       systemPromptFile,
		timer:                  timer,
		msgService:             msgService,
		feedbackInitialTimeout: feedbackInitialTimeout,
		feedbackFollowupDelay:  feedbackFollowupDelay,
	}
}

// GetToolDefinition returns the OpenAI tool definition for tracking feedback.
func (ftt *FeedbackTrackerTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "track_feedback",
			Description: openai.String("Track user feedback and update their profile based on their response to habit prompts. Use this when the user provides feedback about whether they completed a habit, what barriers they faced, or suggests modifications."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"user_response": map[string]interface{}{
						"type":        "string",
						"description": "The user's response about their habit attempt (whether they tried it, barriers faced, suggestions, etc.)",
					},
					"completion_status": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"completed", "attempted", "skipped", "rejected", "modified"},
						"description": "Status of the habit attempt: completed (fully done), attempted (tried but not completed), skipped (didn't try), rejected (didn't like the prompt), modified (wants changes)",
					},
					"barrier_reason": map[string]interface{}{
						"type":        "string",
						"description": "If not completed, the reason or barrier mentioned by user (optional)",
					},
					"suggested_modification": map[string]interface{}{
						"type":        "string",
						"description": "Any modifications or tweaks suggested by the user (optional)",
					},
				},
				"required": []string{"user_response", "completion_status"},
			},
		},
	}
}

// ExecuteFeedbackTracker executes the feedback tracking tool call (legacy method - calls history version with empty history).
func (ftt *FeedbackTrackerTool) ExecuteFeedbackTracker(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	return ftt.ExecuteFeedbackTrackerWithHistory(ctx, participantID, args, []openai.ChatCompletionMessageParamUnion{})
}

// ExecuteFeedbackTrackerWithHistory executes the feedback tracking tool with conversation history.
func (ftt *FeedbackTrackerTool) ExecuteFeedbackTrackerWithHistory(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("flow.ExecuteFeedbackTrackerWithHistory: processing feedback with chat history", "participantID", participantID, "args", args, "historyLength", len(chatHistory))

	// Validate required dependencies
	if ftt.stateManager == nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if ftt.genaiClient == nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: genai client not initialized")
		return "", fmt.Errorf("genai client not initialized")
	}

	// Extract arguments
	userResponse, _ := args["user_response"].(string)
	completionStatus, _ := args["completion_status"].(string)
	barrierReason, _ := args["barrier_reason"].(string)
	suggestedModification, _ := args["suggested_modification"].(string)

	// Validate required parameters
	if userResponse == "" {
		return "", fmt.Errorf("user_response is required")
	}
	if completionStatus == "" {
		return "", fmt.Errorf("completion_status is required")
	}

	slog.Debug("flow.ExecuteFeedbackTrackerWithHistory: parsed parameters",
		"participantID", participantID,
		"userResponse", userResponse,
		"completionStatus", completionStatus,
		"barrierReason", barrierReason,
		"suggestedModification", suggestedModification)

	// Get user profile
	profile, err := ftt.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Get the last prompt from state
	lastPrompt, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt)
	if err != nil {
		slog.Debug("flow.ExecuteFeedbackTrackerWithHistory: no last prompt found in state", "participantID", participantID)
		lastPrompt = "" // Use empty string if no last prompt is found
	}

	// Update profile with feedback
	updatedProfile := ftt.updateProfileWithFeedback(profile, userResponse, completionStatus, barrierReason, suggestedModification, lastPrompt)

	// Save updated profile
	if err := ftt.saveUserProfile(ctx, participantID, updatedProfile); err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to save updated profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to save updated profile: %w", err)
	}

	// Generate personalized feedback response using GenAI with conversation history
	response, err := ftt.generatePersonalizedFeedback(ctx, participantID, updatedProfile, completionStatus, userResponse, barrierReason, suggestedModification, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to generate personalized feedback", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}

	slog.Info("flow.ExecuteFeedbackTrackerWithHistory: feedback processed successfully", "participantID", participantID, "completionStatus", completionStatus)
	return response, nil
}

// updateProfileWithFeedback updates the user profile based on their feedback
func (ftt *FeedbackTrackerTool) updateProfileWithFeedback(profile *UserProfile, userResponse, completionStatus, barrierReason, suggestedModification, lastPrompt string) *UserProfile {
	// Increment total prompts count
	profile.TotalPrompts++

	// Update success count for completed attempts
	if completionStatus == "completed" {
		profile.SuccessCount++
		if lastPrompt != "" {
			profile.LastSuccessfulPrompt = lastPrompt
		}
	}

	// Track barriers
	if barrierReason != "" {
		profile.LastBarrier = barrierReason
	}

	// Track suggested modifications
	if suggestedModification != "" {
		profile.LastTweak = suggestedModification

		// Try to update profile fields based on suggested modifications
		ftt.applyProfileModifications(profile, suggestedModification)
	}

	// Update timestamp
	profile.UpdatedAt = time.Now()

	slog.Debug("flow.updateProfileWithFeedback: profile updated",
		"totalPrompts", profile.TotalPrompts,
		"successCount", profile.SuccessCount,
		"hasBarrier", barrierReason != "",
		"hasTweak", suggestedModification != "")

	return profile
}

// applyProfileModifications attempts to update profile fields based on user suggestions
func (ftt *FeedbackTrackerTool) applyProfileModifications(profile *UserProfile, modification string) {
	// Simple keyword-based modification detection
	// In a production system, this could use NLP or more sophisticated parsing

	modification = strings.ToLower(modification)

	// Time-related modifications
	if strings.Contains(modification, "morning") || strings.Contains(modification, "am") {
		if !strings.Contains(profile.PreferredTime, "morning") && !strings.Contains(profile.PreferredTime, "am") {
			profile.PreferredTime = "morning"
			slog.Debug("flow.applyProfileModifications: updated preferred time to morning")
		}
	} else if strings.Contains(modification, "evening") || strings.Contains(modification, "pm") {
		if !strings.Contains(profile.PreferredTime, "evening") && !strings.Contains(profile.PreferredTime, "pm") {
			profile.PreferredTime = "evening"
			slog.Debug("flow.applyProfileModifications: updated preferred time to evening")
		}
	}

	// Anchor-related modifications
	if strings.Contains(modification, "after") || strings.Contains(modification, "before") {
		// Extract potential new anchor from the modification
		if strings.Contains(modification, "coffee") {
			profile.PromptAnchor = "coffee time"
			slog.Debug("flow.applyProfileModifications: updated prompt anchor to coffee time")
		} else if strings.Contains(modification, "work") || strings.Contains(modification, "meeting") {
			profile.PromptAnchor = "work breaks"
			slog.Debug("flow.applyProfileModifications: updated prompt anchor to work breaks")
		}
	}
}

// generateFeedbackSummary creates a conversational response based on the feedback processed
func (ftt *FeedbackTrackerTool) generateFeedbackSummary(profile *UserProfile, completionStatus, userResponse string) string {
	switch completionStatus {
	case "completed":
		return fmt.Sprintf("Great job! 🎉 That's %d successful habit completions so far. I've noted what worked well for future prompts.", profile.SuccessCount)
	case "attempted":
		return "Thanks for trying! Even attempting is progress. I've updated your profile to help make the next prompt more doable."
	case "skipped":
		return "No worries - life happens! I've noted the barrier you mentioned to help adjust future prompts."
	case "rejected":
		return "Thanks for the honest feedback! I've noted your preferences and will adjust the next habit suggestion accordingly."
	case "modified":
		return "Perfect! I've updated your profile with your suggested changes. The next prompt will be more tailored to what works for you."
	default:
		return "Thanks for your feedback! I've updated your profile to better personalize future habit suggestions."
	}
}

// getUserProfile retrieves the user profile from state storage
func (ftt *FeedbackTrackerTool) getUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("flow.getUserProfile: no existing profile found, creating new one", "participantID", participantID)
		// Return a new profile if none exists
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		slog.Debug("flow.getUserProfile: empty profile data, creating new one", "participantID", participantID)
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Error("flow.getUserProfile: failed to unmarshal profile", "error", err, "participantID", participantID, "profileJSON", profileJSON)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage
func (ftt *FeedbackTrackerTool) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	return ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (ftt *FeedbackTrackerTool) LoadSystemPrompt() error {
	slog.Debug("flow.FeedbackTrackerTool.LoadSystemPrompt: loading system prompt from file", "file", ftt.systemPromptFile)

	if ftt.systemPromptFile == "" {
		slog.Error("flow.FeedbackTrackerTool.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("feedback tracker system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(ftt.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.FeedbackTrackerTool.LoadSystemPrompt: system prompt file does not exist", "file", ftt.systemPromptFile)
		return fmt.Errorf("feedback tracker system prompt file does not exist: %s", ftt.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(ftt.systemPromptFile)
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.LoadSystemPrompt: failed to read system prompt file", "file", ftt.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read feedback tracker system prompt file: %w", err)
	}

	ftt.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.FeedbackTrackerTool.LoadSystemPrompt: system prompt loaded successfully", "file", ftt.systemPromptFile, "length", len(ftt.systemPrompt))
	return nil
}

// generatePersonalizedFeedback generates a personalized feedback response using GenAI with conversation history
func (ftt *FeedbackTrackerTool) generatePersonalizedFeedback(ctx context.Context, participantID string, profile *UserProfile, completionStatus, userResponse, barrierReason, suggestedModification string, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	// Build messages for GenAI
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if ftt.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(ftt.systemPrompt))
	}

	// Add feedback context
	feedbackContext := ftt.buildFeedbackContext(profile, completionStatus, userResponse, barrierReason, suggestedModification)
	messages = append(messages, openai.SystemMessage(feedbackContext))

	// Add conversation history
	messages = append(messages, chatHistory...)

	// Add current user feedback as a message
	messages = append(messages, openai.UserMessage(userResponse))

	// Generate personalized response
	response, err := ftt.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("flow.generatePersonalizedFeedback: GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}

	return response, nil
}

// buildFeedbackContext creates context for the GenAI about the feedback situation
func (ftt *FeedbackTrackerTool) buildFeedbackContext(profile *UserProfile, completionStatus, userResponse, barrierReason, suggestedModification string) string {
	context := fmt.Sprintf(`FEEDBACK TRACKING CONTEXT:
- User's habit domain: %s
- Motivational frame: %s
- Completion status: %s
- Total prompts sent: %d
- Success count: %d
- User response: %s`,
		profile.HabitDomain,
		profile.MotivationalFrame,
		completionStatus,
		profile.TotalPrompts,
		profile.SuccessCount,
		userResponse)

	if barrierReason != "" {
		context += fmt.Sprintf("\n- Barrier mentioned: %s", barrierReason)
	}

	if suggestedModification != "" {
		context += fmt.Sprintf("\n- Suggested modification: %s", suggestedModification)
	}

	context += "\n\nTASK: Generate a warm, encouraging response that acknowledges their feedback and provides motivational support. If they succeeded, celebrate it. If they faced barriers, empathize and offer encouragement. If they have suggestions, acknowledge them positively. Keep it personal and supportive."

	return context
}

// IsSystemPromptLoaded checks if the system prompt is loaded and not empty
func (ftt *FeedbackTrackerTool) IsSystemPromptLoaded() bool {
	return ftt.systemPrompt != "" && strings.TrimSpace(ftt.systemPrompt) != ""
}

// ScheduleFeedbackCollection schedules automatic feedback collection after a habit prompt.
// This should be called after a prompt generator session completes.
func (ftt *FeedbackTrackerTool) ScheduleFeedbackCollection(ctx context.Context, participantID string) error {
	slog.Debug("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: scheduling feedback collection", "participantID", participantID, "initialTimeout", ftt.feedbackInitialTimeout)

	if ftt.timer == nil {
		slog.Warn("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: timer not configured, skipping feedback scheduling", "participantID", participantID)
		return nil
	}

	if ftt.msgService == nil {
		slog.Warn("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: messaging service not configured, skipping feedback scheduling", "participantID", participantID)
		return nil
	}

	// Parse initial timeout duration
	initialTimeout, err := time.ParseDuration(ftt.feedbackInitialTimeout)
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: invalid initial timeout format", "timeout", ftt.feedbackInitialTimeout, "error", err)
		return fmt.Errorf("invalid feedback initial timeout format: %w", err)
	}

	// Set state to indicate we're waiting for feedback
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "waiting_initial"); err != nil {
		slog.Error("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: failed to set feedback state", "participantID", participantID, "error", err)
		return fmt.Errorf("failed to set feedback state: %w", err)
	}

	// Schedule initial feedback timeout
	timerID, err := ftt.timer.ScheduleAfter(initialTimeout, func() {
		ftt.handleInitialFeedbackTimeout(ctx, participantID)
	})
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: failed to schedule initial timeout", "participantID", participantID, "timeout", initialTimeout, "error", err)
		return fmt.Errorf("failed to schedule initial feedback timeout: %w", err)
	}

	// Store timer ID for potential cancellation
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID, timerID); err != nil {
		slog.Error("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: failed to store timer ID", "participantID", participantID, "timerID", timerID, "error", err)
		// Don't return error as timer is already scheduled, just log warning
	}

	slog.Info("flow.FeedbackTrackerTool.ScheduleFeedbackCollection: feedback collection scheduled", "participantID", participantID, "timerID", timerID, "timeout", initialTimeout)
	return nil
}

// handleInitialFeedbackTimeout handles the case when user doesn't respond to initial feedback request
func (ftt *FeedbackTrackerTool) handleInitialFeedbackTimeout(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackTrackerTool.handleInitialFeedbackTimeout: handling initial feedback timeout", "participantID", participantID)

	// Check if feedback was already received
	feedbackState, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err == nil && feedbackState != "waiting_initial" {
		slog.Debug("flow.FeedbackTrackerTool.handleInitialFeedbackTimeout: feedback already received, skipping timeout", "participantID", participantID, "currentState", feedbackState)
		return
	}

	// Send initial feedback request
	feedbackMessage := "Hi! 🌱 How did that habit suggestion work for you? I'd love to hear your thoughts - did you give it a try? Any feedback helps me make better suggestions for you!"

	phoneNumber, err := ftt.getParticipantPhoneNumber(ctx, participantID)
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.handleInitialFeedbackTimeout: failed to get phone number", "participantID", participantID, "error", err)
		return
	}

	if err := ftt.msgService.SendMessage(ctx, phoneNumber, feedbackMessage); err != nil {
		slog.Error("flow.FeedbackTrackerTool.handleInitialFeedbackTimeout: failed to send feedback request", "participantID", participantID, "phoneNumber", phoneNumber, "error", err)
		return
	}

	slog.Info("flow.FeedbackTrackerTool.handleInitialFeedbackTimeout: initial feedback request sent", "participantID", participantID, "phoneNumber", phoneNumber)

	// Schedule follow-up if no response
	ftt.scheduleFollowupFeedback(ctx, participantID)
}

// scheduleFollowupFeedback schedules a follow-up feedback session after the delay period
func (ftt *FeedbackTrackerTool) scheduleFollowupFeedback(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackTrackerTool.scheduleFollowupFeedback: scheduling follow-up feedback", "participantID", participantID, "followupDelay", ftt.feedbackFollowupDelay)

	// Parse follow-up delay duration
	followupDelay, err := time.ParseDuration(ftt.feedbackFollowupDelay)
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.scheduleFollowupFeedback: invalid follow-up delay format", "delay", ftt.feedbackFollowupDelay, "error", err)
		return
	}

	// Set state to indicate we're waiting for follow-up feedback
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "waiting_followup"); err != nil {
		slog.Error("flow.FeedbackTrackerTool.scheduleFollowupFeedback: failed to set follow-up feedback state", "participantID", participantID, "error", err)
		return
	}

	// Schedule follow-up feedback
	timerID, err := ftt.timer.ScheduleAfter(followupDelay, func() {
		ftt.handleFollowupFeedbackTimeout(ctx, participantID)
	})
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.scheduleFollowupFeedback: failed to schedule follow-up", "participantID", participantID, "delay", followupDelay, "error", err)
		return
	}

	// Store follow-up timer ID
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID, timerID); err != nil {
		slog.Error("flow.FeedbackTrackerTool.scheduleFollowupFeedback: failed to store follow-up timer ID", "participantID", participantID, "timerID", timerID, "error", err)
	}

	slog.Info("flow.FeedbackTrackerTool.scheduleFollowupFeedback: follow-up feedback scheduled", "participantID", participantID, "timerID", timerID, "delay", followupDelay)
}

// handleFollowupFeedbackTimeout handles the follow-up feedback session
func (ftt *FeedbackTrackerTool) handleFollowupFeedbackTimeout(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: handling follow-up feedback timeout", "participantID", participantID)

	// Check if feedback was already received
	feedbackState, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err == nil && feedbackState == "completed" {
		slog.Debug("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: feedback already received, skipping follow-up", "participantID", participantID)
		return
	}

	// Send follow-up feedback request
	followupMessage := "Hey! 👋 Just checking in - I sent you a habit suggestion earlier. Even if you didn't try it, I'd love to know what you think! Your feedback helps me learn what works best for you. 😊"

	phoneNumber, err := ftt.getParticipantPhoneNumber(ctx, participantID)
	if err != nil {
		slog.Error("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: failed to get phone number", "participantID", participantID, "error", err)
		return
	}

	if err := ftt.msgService.SendMessage(ctx, phoneNumber, followupMessage); err != nil {
		slog.Error("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: failed to send follow-up request", "participantID", participantID, "phoneNumber", phoneNumber, "error", err)
		return
	}

	// Update state to indicate follow-up was sent
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "followup_sent"); err != nil {
		slog.Error("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: failed to update feedback state", "participantID", participantID, "error", err)
	}

	slog.Info("flow.FeedbackTrackerTool.handleFollowupFeedbackTimeout: follow-up feedback request sent", "participantID", participantID, "phoneNumber", phoneNumber)
}

// getParticipantPhoneNumber retrieves the phone number for a participant
func (ftt *FeedbackTrackerTool) getParticipantPhoneNumber(ctx context.Context, participantID string) (string, error) {
	// For now, we'll assume the participantID is the phone number
	// In a more complex system, this would look up the phone number from the participant record
	if participantID == "" {
		return "", fmt.Errorf("participantID is empty")
	}
	return participantID, nil
}

// CancelPendingFeedback cancels any pending feedback timers for a participant
func (ftt *FeedbackTrackerTool) CancelPendingFeedback(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: cancelling pending feedback timers", "participantID", participantID)

	if ftt.timer == nil {
		return
	}

	// Cancel initial feedback timer
	if timerID, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID); err == nil && timerID != "" {
		if err := ftt.timer.Cancel(timerID); err != nil {
			slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: failed to cancel initial timer", "participantID", participantID, "timerID", timerID, "error", err)
		} else {
			slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: cancelled initial timer", "participantID", participantID, "timerID", timerID)
		}
	}

	// Cancel follow-up feedback timer
	if timerID, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID); err == nil && timerID != "" {
		if err := ftt.timer.Cancel(timerID); err != nil {
			slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: failed to cancel follow-up timer", "participantID", participantID, "timerID", timerID, "error", err)
		} else {
			slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: cancelled follow-up timer", "participantID", participantID, "timerID", timerID)
		}
	}

	// Clear feedback state
	if err := ftt.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "completed"); err != nil {
		slog.Debug("flow.FeedbackTrackerTool.CancelPendingFeedback: failed to clear feedback state", "participantID", participantID, "error", err)
	}
}
