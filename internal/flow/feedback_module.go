// Package flow provides feedback module functionality for tracking user feedback and updating profiles.
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
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// FeedbackModule provides LLM module functionality for tracking user feedback and updating profiles.
// This module handles the feedback conversation state and has access to shared tools.
type FeedbackModule struct {
	stateManager           StateManager
	genaiClient            genai.ClientInterface
	systemPromptFile       string
	systemPrompt           string
	timer                  models.Timer         // Timer for scheduling feedback timeouts
	msgService             MessagingService     // Messaging service for sending follow-up prompts
	feedbackInitialTimeout string               // Timeout for initial feedback response (e.g., "15m")
	feedbackFollowupDelay  string               // Delay before follow-up feedback session (e.g., "3h")
	stateTransitionTool    *StateTransitionTool // Tool for transitioning back to coordinator
	profileSaveTool        *ProfileSaveTool     // Tool for saving user profiles
	schedulerTool          *SchedulerTool       // Tool for scheduling prompts
}

// NewFeedbackModule creates a new feedback module instance.
func NewFeedbackModule(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool, schedulerTool *SchedulerTool) *FeedbackModule {
	slog.Debug("FeedbackModule.NewFeedbackModule: creating feedback module", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "systemPromptFile", systemPromptFile)
	return &FeedbackModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		systemPromptFile:    systemPromptFile,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
		schedulerTool:       schedulerTool,
	}
}

// NewFeedbackModuleWithTimeouts creates a new feedback module instance with timeout configuration.
func NewFeedbackModuleWithTimeouts(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, timer models.Timer, msgService MessagingService, feedbackInitialTimeout, feedbackFollowupDelay string, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool, schedulerTool *SchedulerTool) *FeedbackModule {
	slog.Debug("flow.NewFeedbackModuleWithTimeouts: creating feedback module with timeouts",
		"hasStateManager", stateManager != nil,
		"hasGenAI", genaiClient != nil,
		"systemPromptFile", systemPromptFile,
		"hasTimer", timer != nil,
		"hasMessaging", msgService != nil,
		"feedbackInitialTimeout", feedbackInitialTimeout,
		"feedbackFollowupDelay", feedbackFollowupDelay)
	return &FeedbackModule{
		stateManager:           stateManager,
		genaiClient:            genaiClient,
		systemPromptFile:       systemPromptFile,
		timer:                  timer,
		msgService:             msgService,
		feedbackInitialTimeout: feedbackInitialTimeout,
		feedbackFollowupDelay:  feedbackFollowupDelay,
		stateTransitionTool:    stateTransitionTool,
		profileSaveTool:        profileSaveTool,
		schedulerTool:          schedulerTool,
	}
}

// GetToolDefinition returns the OpenAI tool definition for tracking feedback.
func (fm *FeedbackModule) GetToolDefinition() openai.ChatCompletionToolParam {
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
func (fm *FeedbackModule) ExecuteFeedbackTracker(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	return fm.ExecuteFeedbackTrackerWithHistory(ctx, participantID, args, []openai.ChatCompletionMessageParamUnion{})
}

// ExecuteFeedbackTrackerWithHistory executes the feedback tracking tool with conversation history.
func (fm *FeedbackModule) ExecuteFeedbackTrackerWithHistory(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	slog.Debug("flow.ExecuteFeedbackTrackerWithHistory: processing feedback with chat history", "participantID", participantID, "args", args, "historyLength", len(chatHistory))

	// Validate required dependencies
	if fm.stateManager == nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	if fm.genaiClient == nil {
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
	profile, err := fm.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Get the last prompt from state
	lastPrompt, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt)
	if err != nil {
		slog.Debug("flow.ExecuteFeedbackTrackerWithHistory: no last prompt found in state", "participantID", participantID)
		lastPrompt = "" // Use empty string if no last prompt is found
	}

	// Update profile with feedback
	updatedProfile := fm.updateProfileWithFeedback(profile, userResponse, completionStatus, barrierReason, suggestedModification, lastPrompt)

	// Save updated profile
	if err := fm.saveUserProfile(ctx, participantID, updatedProfile); err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to save updated profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to save updated profile: %w", err)
	}

	// Generate personalized feedback response using GenAI with conversation history
	response, err := fm.generatePersonalizedFeedback(ctx, participantID, updatedProfile, completionStatus, userResponse, barrierReason, suggestedModification, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to generate personalized feedback", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}

	slog.Info("flow.ExecuteFeedbackTrackerWithHistory: feedback processed successfully", "participantID", participantID, "completionStatus", completionStatus)
	return response, nil
}

// updateProfileWithFeedback updates the user profile based on their feedback
func (fm *FeedbackModule) updateProfileWithFeedback(profile *UserProfile, userResponse, completionStatus, barrierReason, suggestedModification, lastPrompt string) *UserProfile {
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
		fm.applyProfileModifications(profile, suggestedModification)
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
func (fm *FeedbackModule) applyProfileModifications(profile *UserProfile, modification string) {
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
func (fm *FeedbackModule) generateFeedbackSummary(profile *UserProfile, completionStatus, userResponse string) string {
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
func (fm *FeedbackModule) getUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
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
func (fm *FeedbackModule) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	return fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (fm *FeedbackModule) LoadSystemPrompt() error {
	slog.Debug("flow.FeedbackModule.LoadSystemPrompt: loading system prompt from file", "file", fm.systemPromptFile)

	if fm.systemPromptFile == "" {
		slog.Error("flow.FeedbackModule.LoadSystemPrompt: system prompt file not configured")
		return fmt.Errorf("feedback module system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(fm.systemPromptFile); os.IsNotExist(err) {
		slog.Debug("flow.FeedbackModule.LoadSystemPrompt: system prompt file does not exist", "file", fm.systemPromptFile)
		return fmt.Errorf("feedback module system prompt file does not exist: %s", fm.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(fm.systemPromptFile)
	if err != nil {
		slog.Error("flow.FeedbackModule.LoadSystemPrompt: failed to read system prompt file", "file", fm.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read feedback module system prompt file: %w", err)
	}

	fm.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("flow.FeedbackModule.LoadSystemPrompt: system prompt loaded successfully", "file", fm.systemPromptFile, "length", len(fm.systemPrompt))
	return nil
}

// generatePersonalizedFeedback generates a personalized feedback response using GenAI with conversation history
func (fm *FeedbackModule) generatePersonalizedFeedback(ctx context.Context, participantID string, profile *UserProfile, completionStatus, userResponse, barrierReason, suggestedModification string, chatHistory []openai.ChatCompletionMessageParamUnion) (string, error) {
	// Build messages for GenAI
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system prompt
	if fm.systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(fm.systemPrompt))
	}

	// Add feedback context
	feedbackContext := fm.buildFeedbackContext(profile, completionStatus, userResponse, barrierReason, suggestedModification)
	messages = append(messages, openai.SystemMessage(feedbackContext))

	// Add conversation history
	messages = append(messages, chatHistory...)

	// Add current user feedback as a message
	messages = append(messages, openai.UserMessage(userResponse))

	// Create tool definitions for feedback module
	tools := []openai.ChatCompletionToolParam{}

	// Add state transition tool
	if fm.stateTransitionTool != nil {
		toolDef := fm.stateTransitionTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("FeedbackModule.generatePersonalizedFeedback: added state transition tool", "participantID", participantID)
	}

	// Add profile save tool
	if fm.profileSaveTool != nil {
		toolDef := fm.profileSaveTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("FeedbackModule.generatePersonalizedFeedback: added profile save tool", "participantID", participantID)
	}

	// Add scheduler tool
	if fm.schedulerTool != nil {
		toolDef := fm.schedulerTool.GetToolDefinition()
		tools = append(tools, toolDef)
		slog.Debug("FeedbackModule.generatePersonalizedFeedback: added scheduler tool", "participantID", participantID)
	}

	// Generate response using LLM with tools
	response, err := fm.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("flow.generatePersonalizedFeedback: GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}

	// Check if there are tool calls to handle
	if len(response.ToolCalls) > 0 {
		slog.Info("FeedbackModule.generatePersonalizedFeedback: processing tool calls", "participantID", participantID, "toolCallCount", len(response.ToolCalls))
		return fm.handleFeedbackToolCalls(ctx, participantID, response, messages, tools)
	}

	// No tool calls, return direct response
	return response.Content, nil
}

// buildFeedbackContext creates context for the GenAI about the feedback situation
func (fm *FeedbackModule) buildFeedbackContext(profile *UserProfile, completionStatus, userResponse, barrierReason, suggestedModification string) string {
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
func (fm *FeedbackModule) IsSystemPromptLoaded() bool {
	return fm.systemPrompt != "" && strings.TrimSpace(fm.systemPrompt) != ""
}

// ScheduleFeedbackCollection schedules automatic feedback collection after a habit prompt.
// This should be called after a prompt generator session completes.
func (fm *FeedbackModule) ScheduleFeedbackCollection(ctx context.Context, participantID string) error {
	slog.Debug("flow.FeedbackModule.ScheduleFeedbackCollection: scheduling feedback collection", "participantID", participantID, "initialTimeout", fm.feedbackInitialTimeout)

	if fm.timer == nil {
		slog.Warn("flow.FeedbackModule.ScheduleFeedbackCollection: timer not configured, skipping feedback scheduling", "participantID", participantID)
		return nil
	}

	if fm.msgService == nil {
		slog.Warn("flow.FeedbackModule.ScheduleFeedbackCollection: messaging service not configured, skipping feedback scheduling", "participantID", participantID)
		return nil
	}

	// Parse initial timeout duration
	initialTimeout, err := time.ParseDuration(fm.feedbackInitialTimeout)
	if err != nil {
		slog.Error("flow.FeedbackModule.ScheduleFeedbackCollection: invalid initial timeout format", "timeout", fm.feedbackInitialTimeout, "error", err)
		return fmt.Errorf("invalid feedback initial timeout format: %w", err)
	}

	// Set state to indicate we're waiting for feedback
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "waiting_initial"); err != nil {
		slog.Error("flow.FeedbackModule.ScheduleFeedbackCollection: failed to set feedback state", "participantID", participantID, "error", err)
		return fmt.Errorf("failed to set feedback state: %w", err)
	}

	// Schedule initial feedback timeout
	timerID, err := fm.timer.ScheduleAfter(initialTimeout, func() {
		fm.handleInitialFeedbackTimeout(ctx, participantID)
	})
	if err != nil {
		slog.Error("flow.FeedbackModule.ScheduleFeedbackCollection: failed to schedule initial timeout", "participantID", participantID, "timeout", initialTimeout, "error", err)
		return fmt.Errorf("failed to schedule initial feedback timeout: %w", err)
	}

	// Store timer ID for potential cancellation
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID, timerID); err != nil {
		slog.Error("flow.FeedbackModule.ScheduleFeedbackCollection: failed to store timer ID", "participantID", participantID, "timerID", timerID, "error", err)
		// Don't return error as timer is already scheduled, just log warning
	}

	slog.Info("flow.FeedbackModule.ScheduleFeedbackCollection: feedback collection scheduled", "participantID", participantID, "timerID", timerID, "timeout", initialTimeout)
	return nil
}

// handleInitialFeedbackTimeout handles the case when user doesn't respond to initial feedback request
func (fm *FeedbackModule) handleInitialFeedbackTimeout(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackModule.handleInitialFeedbackTimeout: handling initial feedback timeout", "participantID", participantID)

	// Check if feedback was already received
	feedbackState, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err == nil && feedbackState != "waiting_initial" {
		slog.Debug("flow.FeedbackModule.handleInitialFeedbackTimeout: feedback already received, skipping timeout", "participantID", participantID, "currentState", feedbackState)
		return
	}

	// Send initial feedback request
	feedbackMessage := "Hi! 🌱 How did that habit suggestion work for you? I'd love to hear your thoughts - did you give it a try? Any feedback helps me make better suggestions for you!"

	phoneNumber, err := fm.getParticipantPhoneNumber(ctx, participantID)
	if err != nil {
		slog.Error("flow.FeedbackModule.handleInitialFeedbackTimeout: failed to get phone number", "participantID", participantID, "error", err)
		return
	}

	if err := fm.msgService.SendMessage(ctx, phoneNumber, feedbackMessage); err != nil {
		slog.Error("flow.FeedbackModule.handleInitialFeedbackTimeout: failed to send feedback request", "participantID", participantID, "phoneNumber", phoneNumber, "error", err)
		return
	}

	slog.Info("flow.FeedbackModule.handleInitialFeedbackTimeout: initial feedback request sent", "participantID", participantID, "phoneNumber", phoneNumber)

	// Schedule follow-up if no response
	fm.scheduleFollowupFeedback(ctx, participantID)
}

// scheduleFollowupFeedback schedules a follow-up feedback session after the delay period
func (fm *FeedbackModule) scheduleFollowupFeedback(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackModule.scheduleFollowupFeedback: scheduling follow-up feedback", "participantID", participantID, "followupDelay", fm.feedbackFollowupDelay)

	// Parse follow-up delay duration
	followupDelay, err := time.ParseDuration(fm.feedbackFollowupDelay)
	if err != nil {
		slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: invalid follow-up delay format", "delay", fm.feedbackFollowupDelay, "error", err)
		return
	}

	// Set state to indicate we're waiting for follow-up feedback
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "waiting_followup"); err != nil {
		slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to set follow-up feedback state", "participantID", participantID, "error", err)
		return
	}

	// Schedule follow-up feedback
	timerID, err := fm.timer.ScheduleAfter(followupDelay, func() {
		fm.handleFollowupFeedbackTimeout(ctx, participantID)
	})
	if err != nil {
		slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to schedule follow-up", "participantID", participantID, "delay", followupDelay, "error", err)
		return
	}

	// Store follow-up timer ID
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID, timerID); err != nil {
		slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to store follow-up timer ID", "participantID", participantID, "timerID", timerID, "error", err)
	}

	slog.Info("flow.FeedbackModule.scheduleFollowupFeedback: follow-up feedback scheduled", "participantID", participantID, "timerID", timerID, "delay", followupDelay)
}

// handleFollowupFeedbackTimeout handles the follow-up feedback session
func (fm *FeedbackModule) handleFollowupFeedbackTimeout(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackModule.handleFollowupFeedbackTimeout: handling follow-up feedback timeout", "participantID", participantID)

	// Check if feedback was already received
	feedbackState, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState)
	if err == nil && feedbackState == "completed" {
		slog.Debug("flow.FeedbackModule.handleFollowupFeedbackTimeout: feedback already received, skipping follow-up", "participantID", participantID)
		return
	}

	// Send follow-up feedback request
	followupMessage := "Hey! 👋 Just checking in - I sent you a habit suggestion earlier. Even if you didn't try it, I'd love to know what you think! Your feedback helps me learn what works best for you. 😊"

	phoneNumber, err := fm.getParticipantPhoneNumber(ctx, participantID)
	if err != nil {
		slog.Error("flow.FeedbackModule.handleFollowupFeedbackTimeout: failed to get phone number", "participantID", participantID, "error", err)
		return
	}

	if err := fm.msgService.SendMessage(ctx, phoneNumber, followupMessage); err != nil {
		slog.Error("flow.FeedbackModule.handleFollowupFeedbackTimeout: failed to send follow-up request", "participantID", participantID, "phoneNumber", phoneNumber, "error", err)
		return
	}

	// Update state to indicate follow-up was sent
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "followup_sent"); err != nil {
		slog.Error("flow.FeedbackModule.handleFollowupFeedbackTimeout: failed to update feedback state", "participantID", participantID, "error", err)
	}

	slog.Info("flow.FeedbackModule.handleFollowupFeedbackTimeout: follow-up feedback request sent", "participantID", participantID, "phoneNumber", phoneNumber)
}

// getParticipantPhoneNumber retrieves the phone number for a participant
func (fm *FeedbackModule) getParticipantPhoneNumber(ctx context.Context, participantID string) (string, error) {
	// For now, we'll assume the participantID is the phone number
	// In a more complex system, this would look up the phone number from the participant record
	if participantID == "" {
		return "", fmt.Errorf("participantID is empty")
	}
	return participantID, nil
}

// CancelPendingFeedback cancels any pending feedback timers for a participant
func (fm *FeedbackModule) CancelPendingFeedback(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelling pending feedback timers", "participantID", participantID)

	if fm.timer == nil {
		return
	}

	// Cancel initial feedback timer
	if timerID, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID); err == nil && timerID != "" {
		if err := fm.timer.Cancel(timerID); err != nil {
			slog.Debug("flow.FeedbackModule.CancelPendingFeedback: failed to cancel initial timer", "participantID", participantID, "timerID", timerID, "error", err)
		} else {
			slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelled initial timer", "participantID", participantID, "timerID", timerID)
		}
	}

	// Cancel follow-up feedback timer
	if timerID, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID); err == nil && timerID != "" {
		if err := fm.timer.Cancel(timerID); err != nil {
			slog.Debug("flow.FeedbackModule.CancelPendingFeedback: failed to cancel follow-up timer", "participantID", participantID, "timerID", timerID, "error", err)
		} else {
			slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelled follow-up timer", "participantID", participantID, "timerID", timerID)
		}
	}

	// Clear feedback state
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "completed"); err != nil {
		slog.Debug("flow.FeedbackModule.CancelPendingFeedback: failed to clear feedback state", "participantID", participantID, "error", err)
	}
}

// handleFeedbackToolCalls processes tool calls from the feedback module AI and executes them.
func (fm *FeedbackModule) handleFeedbackToolCalls(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) (string, error) {
	slog.Info("FeedbackModule.handleFeedbackToolCalls: processing tool calls", "participantID", participantID, "toolCallCount", len(toolResponse.ToolCalls))

	// Execute each tool call
	var toolResults []string
	for _, toolCall := range toolResponse.ToolCalls {
		slog.Info("FeedbackModule: executing tool call", "participantID", participantID, "toolName", toolCall.Function.Name, "toolCallID", toolCall.ID)

		var result string
		var err error

		switch toolCall.Function.Name {
		case "transition_state":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("FeedbackModule: failed to parse state transition arguments", "error", parseErr, "participantID", participantID)
				result = "State transition completed"
			} else {
				result, err = fm.stateTransitionTool.ExecuteStateTransition(ctx, participantID, args)
				if err != nil {
					slog.Error("FeedbackModule: state transition failed", "error", err, "participantID", participantID)
					result = "State transition completed"
				}
			}
			toolResults = append(toolResults, result)

		case "save_user_profile":
			// Parse arguments
			var args map[string]interface{}
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &args); parseErr != nil {
				slog.Error("FeedbackModule: failed to parse profile save arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to save profile: invalid arguments"
			} else {
				result, err = fm.profileSaveTool.ExecuteProfileSave(ctx, participantID, args)
				if err != nil {
					slog.Error("FeedbackModule: profile save failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to save profile: %s", err.Error())
				}
			}
			toolResults = append(toolResults, result)

		case "scheduler":
			// Parse arguments
			var params models.SchedulerToolParams
			if parseErr := json.Unmarshal(toolCall.Function.Arguments, &params); parseErr != nil {
				slog.Error("FeedbackModule: failed to parse scheduler arguments", "error", parseErr, "participantID", participantID)
				result = "❌ Failed to set up scheduling: invalid arguments"
			} else {
				schedulerResult, err := fm.schedulerTool.ExecuteScheduler(ctx, participantID, params)
				if err != nil {
					slog.Error("FeedbackModule: scheduler failed", "error", err, "participantID", participantID)
					result = fmt.Sprintf("❌ Failed to set up scheduling: %s", err.Error())
				} else if !schedulerResult.Success {
					result = fmt.Sprintf("❌ %s", schedulerResult.Message)
				} else {
					result = schedulerResult.Message
				}
			}
			toolResults = append(toolResults, result)

		default:
			slog.Warn("FeedbackModule: unknown tool call", "toolName", toolCall.Function.Name, "participantID", participantID)
			result = fmt.Sprintf("❌ Unknown tool: %s", toolCall.Function.Name)
			toolResults = append(toolResults, result)
		}
	}

	// Add tool calls and results to conversation context for LLM follow-up
	for i, toolCall := range toolResponse.ToolCalls {
		// Add assistant message with tool call
		assistantMsg := openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.NewOpt(toolResponse.Content),
			},
			ToolCalls: []openai.ChatCompletionMessageToolCallParam{{
				ID:   toolCall.ID,
				Type: "function",
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      toolCall.Function.Name,
					Arguments: string(toolCall.Function.Arguments),
				},
			}},
		}
		messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg})

		// Add tool result
		if i < len(toolResults) {
			messages = append(messages, openai.ToolMessage(toolResults[i], toolCall.ID))
		}
	}

	// Call LLM again to generate final response with tools available
	finalResponse, err := fm.genaiClient.GenerateWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("FeedbackModule: failed to generate final response after tool execution", "error", err, "participantID", participantID)
		// Fallback: return tool results directly
		if len(toolResults) == 1 {
			return toolResults[0], nil
		}
		return strings.Join(toolResults, "\n\n"), nil
	}

	return finalResponse.Content, nil
}
