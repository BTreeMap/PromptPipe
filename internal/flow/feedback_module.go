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
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/tone"
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
	jobRepo                store.JobRepo        // Durable job repo for restart-safe scheduling
	msgService             MessagingService     // Messaging service for sending follow-up prompts
	feedbackInitialTimeout string               // Timeout for initial feedback response (e.g., "15m")
	feedbackFollowupDelay  string               // Delay before follow-up feedback session (e.g., "3h")
	stateTransitionTool    *StateTransitionTool // Tool for transitioning back to coordinator
	profileSaveTool        *ProfileSaveTool     // Tool for saving user profiles
	schedulerTool          *SchedulerTool       // Tool for scheduling prompts
}

// NewFeedbackModule creates a new feedback module instance with timeout configuration.
func NewFeedbackModule(stateManager StateManager, genaiClient genai.ClientInterface, systemPromptFile string, timer models.Timer, msgService MessagingService, feedbackInitialTimeout, feedbackFollowupDelay string, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool, schedulerTool *SchedulerTool) *FeedbackModule {
	slog.Debug("flow.NewFeedbackModule: creating feedback module with timeouts",
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

// SetJobRepo sets the durable job repository for restart-safe feedback scheduling.
func (fm *FeedbackModule) SetJobRepo(repo store.JobRepo) {
	fm.jobRepo = repo
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

// ExecuteFeedbackTrackerWithHistoryAndConversation executes the feedback tracking tool and can modify the conversation history directly.
func (fm *FeedbackModule) ExecuteFeedbackTrackerWithHistoryAndConversation(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
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

	// Get user profile for context in feedback generation
	// Note: Profile updates are now handled by the LLM via ProfileSaveTool.
	// Success/failure tracking is handled elsewhere:
	// - TotalPrompts is incremented when prompts are delivered via scheduler
	// - SuccessCount is incremented when user clicks "Done" button in poll response
	profile, err := fm.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Generate personalized feedback response using GenAI with conversation history
	response, err := fm.generatePersonalizedFeedback(ctx, participantID, profile, completionStatus, userResponse, barrierReason, suggestedModification, chatHistory)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTrackerWithHistory: failed to generate personalized feedback", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}

	slog.Info("flow.ExecuteFeedbackTrackerWithHistory: feedback processed successfully", "participantID", participantID, "completionStatus", completionStatus)
	return response, nil
}

// getUserProfile retrieves the user profile from state storage
func (fm *FeedbackModule) getUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	return loadOrCreateUserProfile(ctx, fm.stateManager, participantID)
}

// saveUserProfile saves the user profile to state storage
func (fm *FeedbackModule) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	return persistUserProfile(ctx, fm.stateManager, participantID, profile)
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

	// Add tone guide if user has active tone tags
	if toneGuide := tone.BuildToneGuide(profile.Tone.Tags); toneGuide != "" {
		messages = append(messages, openai.SystemMessage(toneGuide))
	}

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

	// Log the exact tools being passed to LLM
	var toolNames []string
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Function.Name)
	}
	slog.Info("FeedbackModule.generatePersonalizedFeedback: calling LLM with tools",
		"participantID", participantID,
		"toolCount", len(tools),
		"toolNames", toolNames,
		"messageCount", len(messages))

	// Generate response using LLM with tools (thinking enabled)
	thinkingResp, err := fm.genaiClient.GenerateThinkingWithTools(ctx, messages, tools)
	if err != nil {
		slog.Error("flow.generatePersonalizedFeedback: GenAI thinking generation failed", "error", err, "participantID", participantID, "toolNames", toolNames)
		return "", fmt.Errorf("failed to generate personalized feedback: %w", err)
	}
	if thinkingResp.Thinking != "" {
		SendDebugMessageIfEnabled(ctx, participantID, fm.msgService, fmt.Sprintf("Feedback thinking (round 1): %s", thinkingResp.Thinking))
	}

	// Check if there are tool calls to handle
	if len(thinkingResp.ToolCalls) > 0 {
		slog.Info("FeedbackModule.generatePersonalizedFeedback: processing tool calls", "participantID", participantID, "toolCallCount", len(thinkingResp.ToolCalls))
		initial := &genai.ToolCallResponse{Content: thinkingResp.Content, ToolCalls: thinkingResp.ToolCalls}
		return fm.handleFeedbackToolLoop(ctx, participantID, thinkingResp, initial, messages, tools, 1)
	}

	// No tool calls, return direct response
	return thinkingResp.Content, nil
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

	// Get phone number from context for the job payload
	phoneNumber, _ := GetPhoneNumberFromContext(ctx)

	// Prefer durable job if jobRepo is available
	if fm.jobRepo != nil {
		payload, err := json.Marshal(FeedbackTimeoutPayload{
			ParticipantID: participantID,
			PhoneNumber:   phoneNumber,
		})
		if err != nil {
			return fmt.Errorf("failed to marshal feedback timeout payload: %w", err)
		}

		dedupeKey := fmt.Sprintf("feedback_timeout:%s", participantID)
		jobID, err := fm.jobRepo.EnqueueJob(JobKindFeedbackTimeout, time.Now().Add(initialTimeout), string(payload), dedupeKey)
		if err != nil {
			return fmt.Errorf("failed to enqueue feedback timeout job: %w", err)
		}

		if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID, jobID); err != nil {
			slog.Error("flow.FeedbackModule.ScheduleFeedbackCollection: failed to store job ID", "participantID", participantID, "jobID", jobID, "error", err)
		}

		slog.Info("flow.FeedbackModule.ScheduleFeedbackCollection: durable feedback timeout job scheduled", "participantID", participantID, "jobID", jobID, "timeout", initialTimeout)
		return nil
	}

	// Fallback to in-memory timer
	if fm.timer == nil {
		slog.Warn("flow.FeedbackModule.ScheduleFeedbackCollection: timer not configured, skipping feedback scheduling", "participantID", participantID)
		return nil
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

	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	if !hasPhone || phoneNumber == "" {
		slog.Error("flow.FeedbackModule.handleInitialFeedbackTimeout: no phone number in context", "participantID", participantID)
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

	// Get phone number from context for the job payload
	phoneNumber, _ := GetPhoneNumberFromContext(ctx)

	// Prefer durable job if jobRepo is available
	if fm.jobRepo != nil {
		payload, err := json.Marshal(FeedbackFollowupPayload{
			ParticipantID: participantID,
			PhoneNumber:   phoneNumber,
		})
		if err != nil {
			slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to marshal followup payload", "participantID", participantID, "error", err)
			return
		}

		dedupeKey := fmt.Sprintf("feedback_followup:%s", participantID)
		jobID, err := fm.jobRepo.EnqueueJob(JobKindFeedbackFollowup, time.Now().Add(followupDelay), string(payload), dedupeKey)
		if err != nil {
			slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to enqueue followup job", "participantID", participantID, "error", err)
			return
		}

		if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID, jobID); err != nil {
			slog.Error("flow.FeedbackModule.scheduleFollowupFeedback: failed to store followup job ID", "participantID", participantID, "jobID", jobID, "error", err)
		}

		slog.Info("flow.FeedbackModule.scheduleFollowupFeedback: durable follow-up job scheduled", "participantID", participantID, "jobID", jobID, "delay", followupDelay)
		return
	}

	// Fallback to in-memory timer
	if fm.timer == nil {
		slog.Warn("flow.FeedbackModule.scheduleFollowupFeedback: timer not configured, skipping follow-up", "participantID", participantID)
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

	phoneNumber, hasPhone := GetPhoneNumberFromContext(ctx)
	if !hasPhone || phoneNumber == "" {
		slog.Error("flow.FeedbackModule.handleFollowupFeedbackTimeout: no phone number in context", "participantID", participantID)
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

// CancelPendingFeedback cancels any pending feedback timers for a participant
func (fm *FeedbackModule) CancelPendingFeedback(ctx context.Context, participantID string) {
	slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelling pending feedback timers", "participantID", participantID)

	// Cancel initial feedback timer/job
	if timerID, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackTimerID); err == nil && timerID != "" {
		if fm.jobRepo != nil {
			fm.jobRepo.CancelJob(timerID)
		} else if fm.timer != nil {
			fm.timer.Cancel(timerID)
		}
		slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelled initial timer/job", "participantID", participantID, "timerID", timerID)
	}

	// Cancel follow-up feedback timer/job
	if timerID, err := fm.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackFollowupTimerID); err == nil && timerID != "" {
		if fm.jobRepo != nil {
			fm.jobRepo.CancelJob(timerID)
		} else if fm.timer != nil {
			fm.timer.Cancel(timerID)
		}
		slog.Debug("flow.FeedbackModule.CancelPendingFeedback: cancelled follow-up timer/job", "participantID", participantID, "timerID", timerID)
	}

	// Clear feedback state
	if err := fm.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyFeedbackState, "completed"); err != nil {
		slog.Debug("flow.FeedbackModule.CancelPendingFeedback: failed to clear feedback state", "participantID", participantID, "error", err)
	}
}

// handleFeedbackToolLoop manages the tool call loop for the feedback module.
// It continues calling the LLM until a user-facing message is generated.
func (fm *FeedbackModule) handleFeedbackToolLoop(ctx context.Context, participantID string, initialThinkingResp *genai.ThinkingToolCallResponse, initialToolResp *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, startingRound int) (string, error) {
	const maxToolRounds = 10 // Prevent infinite loops
	currentMessages := messages
	currentThinking := initialThinkingResp
	currentResponse := initialToolResp

	for round := startingRound; round <= maxToolRounds; round++ {
		slog.Debug("FeedbackModule.handleFeedbackToolLoop: round start", "participantID", participantID, "round", round, "messageCount", len(currentMessages))

		if currentThinking != nil && currentThinking.Thinking != "" {
			SendDebugMessageIfEnabled(ctx, participantID, fm.msgService, fmt.Sprintf("Feedback thinking (round %d): %s", round, currentThinking.Thinking))
		}

		// Process tool calls if any
		if len(currentResponse.ToolCalls) > 0 {
			slog.Info("FeedbackModule.handleFeedbackToolLoop: processing tool calls", "participantID", participantID, "round", round, "toolCallCount", len(currentResponse.ToolCalls))

			// Execute tools and update conversation context
			updatedMessages, err := fm.executeFeedbackToolCallsAndUpdateContext(ctx, participantID, currentResponse, currentMessages, tools)
			if err != nil {
				return "", err
			}
			currentMessages = updatedMessages

			// If we have content, this is the final response
			if currentResponse.Content != "" {
				slog.Info("FeedbackModule.handleFeedbackToolLoop: tool round completed with user message", "participantID", participantID, "round", round, "responseLength", len(currentResponse.Content))
				return currentResponse.Content, nil
			}

			// No content yet, call LLM again for next round
			slog.Debug("FeedbackModule.handleFeedbackToolLoop: no user message yet, continuing to next round", "participantID", participantID, "round", round)
		} else {
			// No tool calls - check if we have content
			if currentResponse.Content != "" {
				slog.Info("FeedbackModule.handleFeedbackToolLoop: final response without tool calls", "participantID", participantID, "round", round, "responseLength", len(currentResponse.Content))
				return currentResponse.Content, nil
			}

			// No tool calls and no content - this shouldn't happen, but handle it
			slog.Warn("FeedbackModule.handleFeedbackToolLoop: received empty content and no tool calls", "participantID", participantID, "round", round)
			return "Thank you for your feedback! I'm here to help you build better habits.", nil
		}

		// Generate next response with tools (thinking enabled)
		thinkingResp, err := fm.genaiClient.GenerateThinkingWithTools(ctx, currentMessages, tools)
		if err != nil {
			slog.Error("FeedbackModule.handleFeedbackToolLoop: tool thinking generation failed", "error", err, "participantID", participantID, "round", round)
			return "", fmt.Errorf("failed to generate response with tools: %w", err)
		}
		currentThinking = thinkingResp
		toolResponse := &genai.ToolCallResponse{Content: thinkingResp.Content, ToolCalls: thinkingResp.ToolCalls}

		slog.Debug("FeedbackModule.handleFeedbackToolLoop: received tool response",
			"participantID", participantID,
			"round", round,
			"hasContent", toolResponse.Content != "",
			"contentLength", len(toolResponse.Content),
			"toolCallCount", len(toolResponse.ToolCalls))

		currentResponse = toolResponse
	}

	// If we hit max rounds, return fallback message
	slog.Warn("FeedbackModule.handleFeedbackToolLoop: hit maximum tool rounds", "participantID", participantID, "maxRounds", maxToolRounds)
	return "I've completed the requested actions.", nil
}

// executeFeedbackToolCallsAndUpdateContext executes tool calls and updates the conversation context.
func (fm *FeedbackModule) executeFeedbackToolCallsAndUpdateContext(ctx context.Context, participantID string, toolResponse *genai.ToolCallResponse, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) ([]openai.ChatCompletionMessageParamUnion, error) {
	// Log and debug the exact tools being executed
	var executingToolNames []string
	for _, toolCall := range toolResponse.ToolCalls {
		executingToolNames = append(executingToolNames, toolCall.Function.Name)
	}
	slog.Info("FeedbackModule.executeFeedbackToolCallsAndUpdateContext: executing tools",
		"participantID", participantID,
		"toolCallCount", len(toolResponse.ToolCalls),
		"executingTools", executingToolNames)

	// Send debug message if debug mode is enabled in context
	debugMessage := fmt.Sprintf("FeedbackModule executing tools: %s", strings.Join(executingToolNames, ", "))
	SendDebugMessageIfEnabled(ctx, participantID, fm.msgService, debugMessage)

	// Create the tool calls in OpenAI format
	var toolCalls []openai.ChatCompletionMessageToolCallParam
	for _, toolCall := range toolResponse.ToolCalls {
		toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
			ID:   toolCall.ID,
			Type: "function",
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      toolCall.Function.Name,
				Arguments: string(toolCall.Function.Arguments),
			},
		})
	}

	// Create assistant message with both content and tool calls for OpenAI API
	assistantMessageWithToolCalls := openai.ChatCompletionAssistantMessageParam{
		Content: openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: param.NewOpt(toolResponse.Content),
		},
		ToolCalls: toolCalls,
	}

	// Add this assistant message with tool calls to the OpenAI conversation context
	messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMessageWithToolCalls})

	// Execute each tool call and collect results
	var toolResults []string
	for _, toolCall := range toolResponse.ToolCalls {
		slog.Info("FeedbackModule: executing tool call", "participantID", participantID, "toolName", toolCall.Function.Name, "toolCallID", toolCall.ID)
		argumentsPreview := formatToolArgumentsForLog(toolCall.Function.Arguments)
		slog.Debug("FeedbackModule: tool call arguments", "participantID", participantID, "toolName", toolCall.Function.Name, "toolCallID", toolCall.ID, "arguments", argumentsPreview)

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

	// Add tool call results to the OpenAI conversation context
	for i, toolCall := range toolResponse.ToolCalls {
		// Use the corresponding result from our toolResults array
		resultContent := toolResults[i]
		if resultContent == "" {
			resultContent = "Tool executed successfully"
		}

		// Add tool result message to conversation
		messages = append(messages, openai.ToolMessage(resultContent, toolCall.ID))
	}

	return messages, nil
}
