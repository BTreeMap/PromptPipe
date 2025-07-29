// Package flow provides feedback tracker tool functionality for conversation flows.
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

// UserProfile represents the structured user profile built by the intake bot
type UserProfile struct {
	TargetBehavior    string    `json:"target_behavior"`    // e.g., "healthy eating", "physical activity"
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
	stateManager StateManager
	genaiClient  genai.ClientInterface
}

// NewFeedbackTrackerTool creates a new feedback tracker tool instance.
func NewFeedbackTrackerTool(stateManager StateManager, genaiClient genai.ClientInterface) *FeedbackTrackerTool {
	slog.Debug("flow.NewFeedbackTrackerTool: creating feedback tracker tool", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil)
	return &FeedbackTrackerTool{
		stateManager: stateManager,
		genaiClient:  genaiClient,
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

// ExecuteFeedbackTracker executes the feedback tracking tool call.
func (ftt *FeedbackTrackerTool) ExecuteFeedbackTracker(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("flow.ExecuteFeedbackTracker: processing feedback", "participantID", participantID, "args", args)

	// Validate required dependencies
	if ftt.stateManager == nil {
		slog.Error("flow.ExecuteFeedbackTracker: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	// Extract arguments
	userResponse, _ := args["user_response"].(string)
	completionStatus, _ := args["completion_status"].(string)
	barrierReason, _ := args["barrier_reason"].(string)
	suggestedModification, _ := args["suggested_modification"].(string)

	// Validate required arguments
	if userResponse == "" || completionStatus == "" {
		slog.Warn("flow.ExecuteFeedbackTracker: missing required arguments", "participantID", participantID, "userResponse", userResponse, "completionStatus", completionStatus)
		return "", fmt.Errorf("user_response and completion_status are required")
	}

	// Get current user profile
	profile, err := ftt.getUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("flow.ExecuteFeedbackTracker: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Get the last prompt sent to user
	lastPrompt, err := ftt.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyLastHabitPrompt)
	if err != nil {
		slog.Warn("flow.ExecuteFeedbackTracker: could not retrieve last prompt", "error", err, "participantID", participantID)
		lastPrompt = "" // Continue without last prompt
	}

	// Update profile based on feedback
	updatedProfile := ftt.updateProfileWithFeedback(profile, userResponse, completionStatus, barrierReason, suggestedModification, lastPrompt)

	// Save updated profile
	if err := ftt.saveUserProfile(ctx, participantID, updatedProfile); err != nil {
		slog.Error("flow.ExecuteFeedbackTracker: failed to save updated profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to save updated profile: %w", err)
	}

	// Generate response summary for the conversation
	summary := ftt.generateFeedbackSummary(updatedProfile, completionStatus, userResponse)

	slog.Info("flow.ExecuteFeedbackTracker: feedback processed successfully", "participantID", participantID, "status", completionStatus, "totalPrompts", updatedProfile.TotalPrompts, "successCount", updatedProfile.SuccessCount)
	return summary, nil
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
