// Package flow provides shared profile helpers to reduce duplication across modules.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// loadUserProfile retrieves a user profile from state storage. Returns nil and
// an error when no profile is found (callers that want to create a default on
// absence should handle the nil case).
func loadUserProfile(ctx context.Context, sm StateManager, participantID string) (*UserProfile, error) {
	profileJSON, err := sm.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil {
		slog.Debug("flow.loadUserProfile: state data unavailable", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("user profile not found: %w", err)
	}
	if profileJSON == "" {
		slog.Debug("flow.loadUserProfile: empty profile JSON", "participantID", participantID)
		return nil, fmt.Errorf("user profile not found")
	}

	var profile UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		slog.Error("flow.loadUserProfile: failed to unmarshal", "error", err, "participantID", participantID)
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}
	return &profile, nil
}

// loadOrCreateUserProfile retrieves a user profile, returning a default empty
// profile when none exists.
func loadOrCreateUserProfile(ctx context.Context, sm StateManager, participantID string) (*UserProfile, error) {
	profile, err := loadUserProfile(ctx, sm, participantID)
	if err != nil {
		// Return a default profile instead of an error.
		return &UserProfile{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}
	return profile, nil
}

// persistUserProfile marshals and saves a profile to state storage.
func persistUserProfile(ctx context.Context, sm StateManager, participantID string, profile *UserProfile) error {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}
	return sm.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))
}
