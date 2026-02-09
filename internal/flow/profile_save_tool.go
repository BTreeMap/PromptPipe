// Package flow provides profile save tool functionality for managing user profiles.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/tone"
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
	Intensity         string    `json:"intensity"`          // Intervention intensity: "low", "normal", or "high" (default: "normal")
	CreatedAt         time.Time `json:"created_at"`         // When profile was created
	UpdatedAt         time.Time `json:"updated_at"`         // Last profile update

	// Feedback tracking fields
	LastSuccessfulPrompt string `json:"last_successful_prompt,omitempty"` // Last prompt that worked
	LastBarrier          string `json:"last_barrier,omitempty"`           // Last reported barrier
	LastTweak            string `json:"last_tweak,omitempty"`             // Last requested modification
	LastMotivator        string `json:"last_motivator,omitempty"`         // Last motivational reason reported
	SuccessCount         int    `json:"success_count"`                    // Number of successful completions
	TotalPrompts         int    `json:"total_prompts"`                    // Total prompts sent

	// Tone adaptation fields (whitelist-only, server-validated)
	Tone tone.ProfileTone `json:"tone,omitempty"`
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
					"last_barrier": map[string]interface{}{
						"type":        "string",
						"description": "Last reported barrier or blocker (for feedback tracking)",
					},
					"last_motivator": map[string]interface{}{
						"type":        "string",
						"description": "Last reported motivator or reason (for feedback tracking)",
					},
					"last_tweak": map[string]interface{}{
						"type":        "string",
						"description": "Last requested modification or adjustment (for feedback tracking)",
					},
					"tone_tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Proposed tone tags from the whitelist: concise, detailed, formal, casual, no_emojis, emojis_ok, bullet_points, one_question_at_a_time, warm_supportive, neutral_professional, direct_coach, gentle_coach, confirm_before_acting, default_actionable, high_autonomy",
					},
					"tone_update_source": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"explicit", "implicit"},
						"description": "Whether the tone update is from an explicit user request ('explicit') or inferred from conversation patterns ('implicit')",
					},
					"tone_confidence": map[string]interface{}{
						"type":        "number",
						"description": "Confidence level for implicit tone inference (0.0-1.0)",
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
	var (
		updated       bool
		updatedFields []string
	)

	// Handle deprecated alias "last_blocker" by mapping it to "last_barrier" when absent
	if rawAlias, ok := args["last_blocker"]; ok {
		if aliasVal, ok := rawAlias.(string); ok && aliasVal != "" {
			if existing, exists := args["last_barrier"]; !exists {
				args["last_barrier"] = aliasVal
			} else if existingStr, ok := existing.(string); !ok || existingStr == "" {
				args["last_barrier"] = aliasVal
			}
		}
		delete(args, "last_blocker")
	}

	if habitDomain, ok := args["habit_domain"].(string); ok && habitDomain != "" && habitDomain != profile.HabitDomain {
		profile.HabitDomain = habitDomain
		updated = true
		updatedFields = append(updatedFields, "habit_domain")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated habit domain", "participantID", participantID, "habitDomain", habitDomain)
	}

	if motivationalFrame, ok := args["motivational_frame"].(string); ok && motivationalFrame != "" && motivationalFrame != profile.MotivationalFrame {
		profile.MotivationalFrame = motivationalFrame
		updated = true
		updatedFields = append(updatedFields, "motivational_frame")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated motivational frame", "participantID", participantID, "motivationalFrame", motivationalFrame)
	}

	if aliasMotivator, ok := args["last_motivator"].(string); ok && aliasMotivator != "" && aliasMotivator != profile.LastMotivator {
		profile.LastMotivator = aliasMotivator
		updated = true
		updatedFields = append(updatedFields, "last_motivator")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last motivator", "participantID", participantID)
	}

	if preferredTime, ok := args["preferred_time"].(string); ok && preferredTime != "" && preferredTime != profile.PreferredTime {
		profile.PreferredTime = preferredTime
		updated = true
		updatedFields = append(updatedFields, "preferred_time")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated preferred time", "participantID", participantID, "preferredTime", preferredTime)
	}

	if promptAnchor, ok := args["prompt_anchor"].(string); ok && promptAnchor != "" && promptAnchor != profile.PromptAnchor {
		profile.PromptAnchor = promptAnchor
		updated = true
		updatedFields = append(updatedFields, "prompt_anchor")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated prompt anchor", "participantID", participantID, "promptAnchor", promptAnchor)
	}

	if additionalInfo, ok := args["additional_info"].(string); ok && additionalInfo != "" && additionalInfo != profile.AdditionalInfo {
		profile.AdditionalInfo = additionalInfo
		updated = true
		updatedFields = append(updatedFields, "additional_info")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated additional info", "participantID", participantID, "additionalInfo", additionalInfo)
	}

	// Feedback tracking fields
	if lastSuccessfulPrompt, ok := args["last_successful_prompt"].(string); ok && lastSuccessfulPrompt != "" && lastSuccessfulPrompt != profile.LastSuccessfulPrompt {
		profile.LastSuccessfulPrompt = lastSuccessfulPrompt
		updated = true
		updatedFields = append(updatedFields, "last_successful_prompt")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last successful prompt", "participantID", participantID)
	}

	if lastBarrier, ok := args["last_barrier"].(string); ok && lastBarrier != "" && lastBarrier != profile.LastBarrier {
		profile.LastBarrier = lastBarrier
		updated = true
		updatedFields = append(updatedFields, "last_barrier")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last barrier", "participantID", participantID)
	}

	if lastTweak, ok := args["last_tweak"].(string); ok && lastTweak != "" && lastTweak != profile.LastTweak {
		profile.LastTweak = lastTweak
		updated = true
		updatedFields = append(updatedFields, "last_tweak")
		slog.Debug("ProfileSaveTool.ExecuteProfileSave: updated last tweak", "participantID", participantID)
	}

	// Tone adaptation fields — server-side validated and gated
	if toneTagsRaw, hasTone := args["tone_tags"]; hasTone {
		proposal := pst.parseToneProposal(args)
		proposal = tone.ValidateProposal(proposal)
		if len(proposal.Tags) > 0 || len(proposal.Scores) > 0 {
			if tone.UpdateProfileTone(&profile.Tone, proposal, time.Now()) {
				updated = true
				updatedFields = append(updatedFields, "tone")
				slog.Info("ProfileSaveTool.ExecuteProfileSave: tone updated", "participantID", participantID, "tags", profile.Tone.Tags, "source", proposal.Source)
			} else {
				slog.Debug("ProfileSaveTool.ExecuteProfileSave: tone proposal rejected (rate limit or no change)", "participantID", participantID)
			}
		} else {
			slog.Debug("ProfileSaveTool.ExecuteProfileSave: tone proposal empty after validation", "participantID", participantID, "rawTags", toneTagsRaw)
		}
	}

	// Save profile if anything was updated
	if updated {
		if err := pst.saveUserProfile(ctx, participantID, profile); err != nil {
			slog.Error("ProfileSaveTool.ExecuteProfileSave: failed to save user profile", "error", err, "participantID", participantID)
			return "", fmt.Errorf("failed to save user profile: %w", err)
		}
		slog.Info("ProfileSaveTool.ExecuteProfileSave: profile saved successfully", "participantID", participantID, "updatedFields", updatedFields)

		humanSummary := fmt.Sprintf("SUCCESS: profile saved (%d field(s) updated: %s)", len(updatedFields), strings.Join(updatedFields, ", "))
		payload := map[string]interface{}{
			"status":           "success",
			"updated_fields":   updatedFields,
			"profile_snapshot": profile,
		}
		if payloadJSON, err := json.Marshal(payload); err == nil {
			return fmt.Sprintf("%s\n%s", humanSummary, string(payloadJSON)), nil
		}
		return humanSummary, nil
	}

	// No changes detected but still respond clearly so the agent can continue
	payload := map[string]interface{}{
		"status":           "noop",
		"message":          "Profile already contained the supplied values; no new save was required.",
		"profile_snapshot": profile,
	}
	humanSummary := "NOOP: profile already up to date; no changes were saved."
	if payloadJSON, err := json.Marshal(payload); err == nil {
		return fmt.Sprintf("%s\n%s", humanSummary, string(payloadJSON)), nil
	}
	return humanSummary, nil
}

// getOrCreateUserProfile retrieves or creates a new user profile
func (pst *ProfileSaveTool) GetOrCreateUserProfile(ctx context.Context, participantID string) (*UserProfile, error) {
	profileJSON, err := pst.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("ProfileSaveTool.getOrCreateUserProfile: creating new profile", "participantID", participantID)
		// Create new profile with default intensity
		return &UserProfile{
			Intensity: "normal", // Default intensity
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Handle empty string (no profile exists yet)
	if profileJSON == "" {
		slog.Debug("ProfileSaveTool.getOrCreateUserProfile: empty profile data, creating new one", "participantID", participantID)
		return &UserProfile{
			Intensity: "normal", // Default intensity
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Error("ProfileSaveTool.getOrCreateUserProfile: failed to unmarshal profile", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	// Backwards compatibility: migrate legacy stored last_blocker -> last_barrier
	if profile.LastBarrier == "" {
		var legacy struct {
			LastBlocker string `json:"last_blocker"`
		}
		if err := json.Unmarshal([]byte(profileJSON), &legacy); err == nil && legacy.LastBlocker != "" {
			profile.LastBarrier = legacy.LastBlocker
			slog.Debug("ProfileSaveTool.getOrCreateUserProfile: migrated legacy last_blocker to last_barrier", "participantID", participantID)
		}
	}

	// Backwards compatibility: set default intensity for existing profiles without it
	if profile.Intensity == "" {
		profile.Intensity = "normal"
		slog.Debug("ProfileSaveTool.getOrCreateUserProfile: set default intensity for existing profile", "participantID", participantID)
	}

	// Backwards compatibility: ensure tone version is set
	if profile.Tone.Version == 0 {
		profile.Tone.Version = 1
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

// parseToneProposal extracts tone proposal fields from tool call arguments.
func (pst *ProfileSaveTool) parseToneProposal(args map[string]interface{}) tone.Proposal {
	p := tone.Proposal{}

	// Parse tone_tags
	if rawTags, ok := args["tone_tags"]; ok {
		switch v := rawTags.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					p.Tags = append(p.Tags, s)
				}
			}
		case []string:
			p.Tags = v
		}
	}

	// Parse tone_update_source
	if src, ok := args["tone_update_source"].(string); ok {
		switch src {
		case "explicit":
			p.Source = tone.SourceExplicit
		default:
			p.Source = tone.SourceImplicit
		}
	}

	// Parse tone_confidence
	switch v := args["tone_confidence"].(type) {
	case float64:
		p.Confidence = float32(v)
	case float32:
		p.Confidence = v
	}

	return p
}
