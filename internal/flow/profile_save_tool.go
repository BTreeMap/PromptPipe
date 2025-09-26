// Package flow provides profile save tool functionality for managing user profiles.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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

// ProfileSaveTool provides functionality for saving and updating user profiles.
// This tool is shared across all conversation modules (coordinator, intake, feedback).
type ProfileSaveTool struct {
	stateManager StateManager
}

// NewProfileSaveTool creates a new profile save tool instance.
func NewProfileSaveTool(stateManager StateManager) *ProfileSaveTool {
	slog.Debug("ProfileSaveTool.NewProfileSaveTool: creating profile save tool", "hasStateManager", stateManager != nil)
	return &ProfileSaveTool{
		stateManager: stateManager,
	}
}

// GetToolDefinition returns the OpenAI tool definition for saving user profiles.
func (pst *ProfileSaveTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "save_user_profile",
			Description: openai.String("Save or update the user's profile with information gathered during conversation. Use this whenever you have collected meaningful information about the user's habits, goals, motivation, timing preferences, or anchors."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"habit_domain": map[string]interface{}{
						"type":        "string",
						"description": "The specific habit or behavior the user wants to build (e.g., 'healthy eating', 'physical activity', 'better sleep')",
					},
					"prompt_anchor": map[string]interface{}{
						"type":        "string",
						"description": "Natural trigger or anchor for the habit (e.g., 'after coffee', 'before meetings', 'during breaks')",
					},
					"motivational_frame": map[string]interface{}{
						"type":        "string",
						"description": "Why this habit matters to the user personally - their deeper motivation",
					},
					"preferred_time": map[string]interface{}{
						"type":        "string",
						"description": "When the user prefers to receive habit prompts (e.g., '9am', 'morning', 'randomly')",
					},
					"additional_info": map[string]interface{}{
						"type":        "string",
						"description": "Any additional personalization information the user has shared",
					},
					"last_successful_prompt": map[string]interface{}{
						"type":        "string",
						"description": "Last prompt that worked well for the user (for feedback tracking)",
					},
					"last_motivator": map[string]interface{}{
						"type":        "string",
						"description": "Last reported motivator or reason (for feedback tracking)",
					},
					"last_blocker": map[string]interface{}{
						"type":        "string",
						"description": "Last reported barrier or challenge (for feedback tracking)",
					},
					"last_tweak": map[string]interface{}{
						"type":        "string",
						"description": "Last requested modification or adjustment (for feedback tracking)",
					},
				},
				"required": []string{
					"prompt_anchor",
					"preferred_time",
				},
			},
		},
	}
}

// ExecuteProfileSave executes the profile save tool call.
func (pst *ProfileSaveTool) ExecuteProfileSave(ctx context.Context, participantID string, args map[string]interface{}) (string, error) {
	slog.Debug("ProfileSaveTool.ExecuteProfileSave: saving profile", "participantID", participantID, "args", args)

	// Validate required dependencies
	if pst.stateManager == nil {
		slog.Error("ProfileSaveTool.ExecuteProfileSave: state manager not initialized")
		return "", fmt.Errorf("state manager not initialized")
	}

	// Get or create user profile
	profile, err := pst.GetOrCreateUserProfile(ctx, participantID)
	if err != nil {
		slog.Error("ProfileSaveTool.ExecuteProfileSave: failed to get user profile", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	// Update profile fields from arguments
	var updated bool
	if habitDomain, ok := args["habit_domain"].(string); ok && habitDomain != "" {
		profile.HabitDomain = habitDomain
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated habit domain", "participantID", participantID, "habitDomain", habitDomain)
	}

	if motivationalFrame, ok := args["motivational_frame"].(string); ok && motivationalFrame != "" {
		profile.MotivationalFrame = motivationalFrame
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated motivational frame", "participantID", participantID, "motivationalFrame", motivationalFrame)
	}

	if preferredTime, ok := args["preferred_time"].(string); ok && preferredTime != "" {
		profile.PreferredTime = preferredTime
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated preferred time", "participantID", participantID, "preferredTime", preferredTime)
	}

	if promptAnchor, ok := args["prompt_anchor"].(string); ok && promptAnchor != "" {
		profile.PromptAnchor = promptAnchor
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated prompt anchor", "participantID", participantID, "promptAnchor", promptAnchor)
	}

	if additionalInfo, ok := args["additional_info"].(string); ok && additionalInfo != "" {
		profile.AdditionalInfo = additionalInfo
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated additional info", "participantID", participantID, "additionalInfo", additionalInfo)
	}

	// Feedback tracking fields
	if lastSuccessfulPrompt, ok := args["last_successful_prompt"].(string); ok && lastSuccessfulPrompt != "" {
		profile.LastSuccessfulPrompt = lastSuccessfulPrompt
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last successful prompt", "participantID", participantID)
	}

	if lastBarrier, ok := args["last_barrier"].(string); ok && lastBarrier != "" {
		profile.LastBarrier = lastBarrier
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last barrier", "participantID", participantID)
	}

	if lastTweak, ok := args["last_tweak"].(string); ok && lastTweak != "" {
		profile.LastTweak = lastTweak
		updated = true
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last tweak", "participantID", participantID)
	}

	// Save profile if anything was updated
	if updated {
		if err := pst.saveUserProfile(ctx, participantID, profile); err != nil {
			slog.Error("ProfileSaveTool.ExecuteProfileSave: failed to save user profile", "error", err, "participantID", participantID)
			return "", fmt.Errorf("failed to save user profile: %w", err)
		}
		slog.Info("ProfileSaveTool.ExecuteProfileSave: profile saved successfully", "participantID", participantID)
		return "Profile updated successfully", nil
	}

	return "No profile changes to save", nil
}

// getOrCreateUserProfile retrieves or creates a new user profile
func (pst *ProfileSaveTool) GetOrCreateUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := pst.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("ProfileSaveTool.getOrCreateUserProfile: creating new profile", "participantID", participantID)
		// Create new profile
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		slog.Debug("ProfileSaveTool.getOrCreateUserProfile: empty profile data, creating new one", "participantID", participantID)
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Error("ProfileSaveTool.getOrCreateUserProfile: failed to unmarshal profile", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	slog.Debug("ProfileSaveTool.getOrCreateUserProfile: loaded existing profile",
		"participantID", participantID,
		"habitDomain", profile.HabitDomain,
		"motivationalFrame", profile.MotivationalFrame,
		"preferredTime", profile.PreferredTime,
		"promptAnchor", profile.PromptAnchor,
		"createdAt", profile.CreatedAt,
		"updatedAt", profile.UpdatedAt)

	return &profile, nil
}

// saveUserProfile saves the user profile to state storage
func (pst *ProfileSaveTool) saveUserProfile(ctx context.Context, participantID string, profile *UserProfile) error {
	slog.Debug("ProfileSaveTool.saveUserProfile: saving profile",
		"participantID", participantID,
		"habitDomain", profile.HabitDomain,
		"motivationalFrame", profile.MotivationalFrame,
		"preferredTime", profile.PreferredTime,
		"promptAnchor", profile.PromptAnchor,
		"additionalInfo", profile.AdditionalInfo)

	// Update timestamp
	profile.UpdatedAt = time.Now()

	// Marshal profile to JSON
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		slog.Error("ProfileSaveTool.saveUserProfile: failed to marshal profile", "error", err, "participantID", participantID)
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	// Save to state storage
	err = pst.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
	if err != nil {
		slog.Error("ProfileSaveTool.saveUserProfile: failed to save profile to storage", "error", err, "participantID", participantID)
		return fmt.Errorf("failed to save profile to storage: %w", err)
	}

	slog.Info("ProfileSaveTool.saveUserProfile: profile saved successfully", "participantID", participantID)
	return nil
}
