package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/tone"
)

func TestExecuteProfileSave_ToneValidationAndGating(t *testing.T) {
	stateManager := NewMockStateManager()
	tool := NewProfileSaveTool(stateManager)
	ctx := context.Background()
	participantID := "test-user-tone"

	// Seed a minimal profile so save_user_profile succeeds (prompt_anchor and preferred_time required in schema, but ExecuteProfileSave checks them as optional strings).
	seedProfile := &UserProfile{
		HabitDomain:   "exercise",
		PromptAnchor:  "after coffee",
		PreferredTime: "9am",
		Intensity:     "normal",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	profileJSON, _ := json.Marshal(seedProfile)
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Test 1: Valid tone tags are accepted and persisted.
	args := map[string]interface{}{
		"prompt_anchor":      "after coffee",
		"preferred_time":     "9am",
		"tone_tags":          []interface{}{"concise", "formal"},
		"tone_update_source": "explicit",
	}
	result, err := tool.ExecuteProfileSave(ctx, participantID, args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	// Verify tone was persisted.
	profile, err := tool.GetOrCreateUserProfile(ctx, participantID)
	if err != nil {
		t.Fatalf("failed to get profile: %v", err)
	}
	tagSet := make(map[string]bool)
	for _, tag := range profile.Tone.Tags {
		tagSet[tag] = true
	}
	if !tagSet["concise"] || !tagSet["formal"] {
		t.Errorf("expected concise and formal in tone tags, got %v", profile.Tone.Tags)
	}

	// Test 2: Unknown tags are stripped.
	args2 := map[string]interface{}{
		"prompt_anchor":      "after coffee",
		"preferred_time":     "9am",
		"tone_tags":          []interface{}{"concise", "INJECTED_EVIL_TAG", "formal"},
		"tone_update_source": "explicit",
	}
	_, err = tool.ExecuteProfileSave(ctx, participantID, args2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	profile, _ = tool.GetOrCreateUserProfile(ctx, participantID)
	for _, tag := range profile.Tone.Tags {
		if !tone.AllTags[tag] {
			t.Errorf("unknown tag persisted: %q", tag)
		}
	}

	// Test 3: Mutual exclusion — concise XOR detailed.
	args3 := map[string]interface{}{
		"prompt_anchor":      "after coffee",
		"preferred_time":     "9am",
		"tone_tags":          []interface{}{"concise", "detailed"},
		"tone_update_source": "explicit",
	}
	_, err = tool.ExecuteProfileSave(ctx, participantID, args3)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	profile, _ = tool.GetOrCreateUserProfile(ctx, participantID)
	tagSet = make(map[string]bool)
	for _, tag := range profile.Tone.Tags {
		tagSet[tag] = true
	}
	if tagSet["concise"] && tagSet["detailed"] {
		t.Error("concise and detailed should not both be active (mutual exclusion)")
	}
}

func TestExecuteProfileSave_ImplicitToneRateLimited(t *testing.T) {
	stateManager := NewMockStateManager()
	tool := NewProfileSaveTool(stateManager)
	ctx := context.Background()
	participantID := "test-user-ratelimit"

	// Seed profile with recent tone update.
	seedProfile := &UserProfile{
		HabitDomain:   "exercise",
		PromptAnchor:  "after coffee",
		PreferredTime: "9am",
		Intensity:     "normal",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Tone: tone.ProfileTone{
			Version:       1,
			LastUpdatedAt: time.Now(), // just updated
			Scores:        map[string]float32{"concise": 0.5},
			Tags:          []string{},
		},
	}
	profileJSON, _ := json.Marshal(seedProfile)
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Implicit update immediately after last update should be rate-limited.
	args := map[string]interface{}{
		"prompt_anchor":      "after coffee",
		"preferred_time":     "9am",
		"tone_tags":          []interface{}{"formal"},
		"tone_update_source": "implicit",
	}
	result, err := tool.ExecuteProfileSave(ctx, participantID, args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Tone should not be in updated fields because of rate limiting.
	if result != "" && !isNoopOrNoToneUpdate(result) {
		// Verify profile tone hasn't changed for implicit.
		profile, _ := tool.GetOrCreateUserProfile(ctx, participantID)
		for _, tag := range profile.Tone.Tags {
			if tag == "formal" {
				t.Error("formal should not have been persisted due to rate limit")
			}
		}
	}
}

func TestExecuteProfileSave_NoEmojisOverride(t *testing.T) {
	stateManager := NewMockStateManager()
	tool := NewProfileSaveTool(stateManager)
	ctx := context.Background()
	participantID := "test-user-emoji"

	seedProfile := &UserProfile{
		PromptAnchor:  "after coffee",
		PreferredTime: "9am",
		Intensity:     "normal",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	profileJSON, _ := json.Marshal(seedProfile)
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	args := map[string]interface{}{
		"prompt_anchor":      "after coffee",
		"preferred_time":     "9am",
		"tone_tags":          []interface{}{"no_emojis", "emojis_ok"},
		"tone_update_source": "explicit",
	}
	_, err := tool.ExecuteProfileSave(ctx, participantID, args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	profile, _ := tool.GetOrCreateUserProfile(ctx, participantID)
	tagSet := make(map[string]bool)
	for _, tag := range profile.Tone.Tags {
		tagSet[tag] = true
	}
	if !tagSet["no_emojis"] {
		t.Error("no_emojis should be active")
	}
	if tagSet["emojis_ok"] {
		t.Error("emojis_ok should be overridden by no_emojis")
	}
}

func TestProfileRoundTrip_WithToneFields(t *testing.T) {
	stateManager := NewMockStateManager()
	tool := NewProfileSaveTool(stateManager)
	ctx := context.Background()
	participantID := "test-user-roundtrip"

	// Create a profile with tone data.
	original := &UserProfile{
		HabitDomain:   "exercise",
		PromptAnchor:  "after lunch",
		PreferredTime: "12pm",
		Intensity:     "normal",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Tone: tone.ProfileTone{
			Tags:          []string{"concise", "warm_supportive"},
			Scores:        map[string]float32{"concise": 0.85, "warm_supportive": 0.9},
			Version:       1,
			LastUpdatedAt: time.Now(),
			UpdateSource:  tone.SourceExplicit,
		},
	}
	profileJSON, _ := json.Marshal(original)
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, string(profileJSON))

	// Read it back.
	loaded, err := tool.GetOrCreateUserProfile(ctx, participantID)
	if err != nil {
		t.Fatalf("failed to load profile: %v", err)
	}

	// Verify tone fields round-tripped.
	if len(loaded.Tone.Tags) != 2 {
		t.Errorf("expected 2 tone tags, got %d: %v", len(loaded.Tone.Tags), loaded.Tone.Tags)
	}
	if loaded.Tone.Scores["concise"] != 0.85 {
		t.Errorf("expected concise score 0.85, got %f", loaded.Tone.Scores["concise"])
	}
	if loaded.Tone.Version != 1 {
		t.Errorf("expected tone version 1, got %d", loaded.Tone.Version)
	}
	if loaded.Tone.UpdateSource != tone.SourceExplicit {
		t.Errorf("expected explicit update source, got %q", loaded.Tone.UpdateSource)
	}
}

func TestProfileBackwardCompatibility_EmptyTone(t *testing.T) {
	stateManager := NewMockStateManager()
	tool := NewProfileSaveTool(stateManager)
	ctx := context.Background()
	participantID := "test-user-compat"

	// Simulate an old profile without tone fields.
	oldProfile := `{"habit_domain":"exercise","prompt_anchor":"after coffee","preferred_time":"9am","intensity":"normal","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","success_count":0,"total_prompts":0}`
	_ = stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile, oldProfile)

	// Load should succeed and tone fields should be defaults.
	profile, err := tool.GetOrCreateUserProfile(ctx, participantID)
	if err != nil {
		t.Fatalf("failed to load old profile: %v", err)
	}
	if profile.HabitDomain != "exercise" {
		t.Errorf("expected exercise, got %q", profile.HabitDomain)
	}
	if profile.Tone.Version != 1 {
		t.Errorf("expected tone version 1 (default), got %d", profile.Tone.Version)
	}
	if len(profile.Tone.Tags) != 0 {
		t.Errorf("expected empty tone tags, got %v", profile.Tone.Tags)
	}
}

func TestBuildToneGuide_IntegrationWithProfile(t *testing.T) {
	tags := []string{"concise", "no_emojis", "direct_coach"}
	guide := tone.BuildToneGuide(tags)
	if guide == "" {
		t.Fatal("expected non-empty guide")
	}
	if !containsSubstring(guide, "concise") {
		t.Error("guide should mention concise")
	}
	if !containsSubstring(guide, "NOT use emojis") {
		t.Error("guide should prohibit emojis")
	}
	if !containsSubstring(guide, "direct coach") {
		t.Error("guide should mention direct coach stance")
	}
	if !containsSubstring(guide, "TONE POLICY") {
		t.Error("guide should have TONE POLICY header")
	}
}

// ---- helpers ----

func isNoopOrNoToneUpdate(result string) bool {
	return containsSubstring(result, "NOOP") || !containsSubstring(result, "tone")
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
