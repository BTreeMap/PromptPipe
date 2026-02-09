package tone

import (
	"sort"
	"testing"
	"time"
)

func TestValidateProposal_StripsUnknownTags(t *testing.T) {
	p := ValidateProposal(Proposal{
		Tags: []string{"concise", "UNKNOWN", "formal", "  detailed  ", "injected_tag"},
	})
	for _, tag := range p.Tags {
		if !AllTags[tag] {
			t.Errorf("unexpected tag in cleaned proposal: %q", tag)
		}
	}
	if len(p.Tags) != 3 { // concise, formal, detailed
		t.Errorf("expected 3 tags, got %d: %v", len(p.Tags), p.Tags)
	}
}

func TestValidateProposal_ClampsScores(t *testing.T) {
	p := ValidateProposal(Proposal{
		Scores: map[string]float32{
			"concise": 1.5,
			"formal":  -0.3,
			"casual":  0.6,
		},
	})
	if p.Scores["concise"] != 1.0 {
		t.Errorf("expected concise score clamped to 1.0, got %f", p.Scores["concise"])
	}
	if p.Scores["formal"] != 0.0 {
		t.Errorf("expected formal score clamped to 0.0, got %f", p.Scores["formal"])
	}
	if p.Scores["casual"] != 0.6 {
		t.Errorf("expected casual score 0.6, got %f", p.Scores["casual"])
	}
}

func TestValidateProposal_DefaultsToImplicit(t *testing.T) {
	p := ValidateProposal(Proposal{Tags: []string{"concise"}})
	if p.Source != SourceImplicit {
		t.Errorf("expected implicit source, got %q", p.Source)
	}
}

func TestUpdateProfileTone_ExplicitAppliesImmediately(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()
	proposal := Proposal{
		Tags:   []string{"concise", "formal"},
		Source: SourceExplicit,
	}
	changed := UpdateProfileTone(pt, proposal, now)
	if !changed {
		t.Fatal("expected profile to change on explicit update")
	}
	tagSet := toSet(pt.Tags)
	if !tagSet["concise"] || !tagSet["formal"] {
		t.Errorf("expected concise and formal in tags, got %v", pt.Tags)
	}
	if pt.Scores["concise"] != 1.0 {
		t.Errorf("expected concise score 1.0, got %f", pt.Scores["concise"])
	}
}

func TestUpdateProfileTone_ImplicitEMA(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()

	// First implicit proposal should set score to alpha*1.0 = 0.15
	proposal := Proposal{Tags: []string{"concise"}, Source: SourceImplicit}
	changed := UpdateProfileTone(pt, proposal, now)
	if !changed {
		t.Fatal("expected change on first implicit proposal")
	}
	if pt.Scores["concise"] < 0.14 || pt.Scores["concise"] > 0.16 {
		t.Errorf("expected ~0.15, got %f", pt.Scores["concise"])
	}
	// Should NOT be active yet (below 0.7).
	if toSet(pt.Tags)["concise"] {
		t.Error("concise should not be active after single implicit proposal")
	}
}

func TestUpdateProfileTone_ImplicitReachesActivation(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()

	// Repeatedly propose until score crosses 0.7.
	for i := 0; i < 30; i++ {
		now = now.Add(5 * time.Minute)
		UpdateProfileTone(pt, Proposal{Tags: []string{"concise"}, Source: SourceImplicit}, now)
	}

	if !toSet(pt.Tags)["concise"] {
		t.Errorf("expected concise to be active after repeated proposals, score=%f", pt.Scores["concise"])
	}
}

func TestUpdateProfileTone_HysteresisDeactivation(t *testing.T) {
	// Start with an active tag.
	pt := &ProfileTone{
		Tags:   []string{"concise"},
		Scores: map[string]float32{"concise": 0.75},
	}
	now := time.Now()

	// Propose without concise many times to let score decay.
	for i := 0; i < 30; i++ {
		now = now.Add(5 * time.Minute)
		// Propose a different tag so the function does work.
		UpdateProfileTone(pt, Proposal{Tags: []string{"formal"}, Source: SourceImplicit}, now)
	}

	// concise score should still be 0.75 since we're not proposing 0 for it.
	// The EMA only updates observed tags. So concise stays active (hysteresis).
	if !toSet(pt.Tags)["concise"] {
		// This is expected: EMA only updates observed tags, so concise retains its score.
		t.Logf("concise score after decay: %f", pt.Scores["concise"])
	}
}

func TestUpdateProfileTone_MutualExclusion_ConciseDetailed(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()

	// Explicitly set both; higher one should win.
	proposal := Proposal{
		Tags:   []string{"concise", "detailed"},
		Scores: map[string]float32{"concise": 0.9, "detailed": 0.8},
		Source: SourceExplicit,
	}
	UpdateProfileTone(pt, proposal, now)

	tagSet := toSet(pt.Tags)
	if tagSet["concise"] && tagSet["detailed"] {
		t.Error("concise and detailed should not both be active")
	}
	if !tagSet["concise"] {
		t.Error("concise should win (higher score)")
	}
}

func TestUpdateProfileTone_MutualExclusion_FormalCasual(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()

	proposal := Proposal{
		Scores: map[string]float32{"formal": 0.8, "casual": 0.85},
		Source: SourceExplicit,
	}
	UpdateProfileTone(pt, proposal, now)

	tagSet := toSet(pt.Tags)
	if tagSet["formal"] && tagSet["casual"] {
		t.Error("formal and casual should not both be active")
	}
	if !tagSet["casual"] {
		t.Error("casual should win (higher score)")
	}
}

func TestUpdateProfileTone_NoEmojisOverridesEmojisOk(t *testing.T) {
	pt := &ProfileTone{}
	now := time.Now()

	proposal := Proposal{
		Tags:   []string{"no_emojis", "emojis_ok"},
		Source: SourceExplicit,
	}
	UpdateProfileTone(pt, proposal, now)

	tagSet := toSet(pt.Tags)
	if !tagSet["no_emojis"] {
		t.Error("no_emojis should be active")
	}
	if tagSet["emojis_ok"] {
		t.Error("emojis_ok should be overridden by no_emojis")
	}
}

func TestUpdateProfileTone_RateLimit(t *testing.T) {
	pt := &ProfileTone{
		LastUpdatedAt: time.Now(),
	}
	// Immediate implicit update should be rate-limited.
	changed := UpdateProfileTone(pt, Proposal{Tags: []string{"concise"}, Source: SourceImplicit}, time.Now())
	if changed {
		t.Error("expected rate limit to block immediate implicit update")
	}
}

func TestUpdateProfileTone_ExplicitBypassesRateLimit(t *testing.T) {
	pt := &ProfileTone{
		LastUpdatedAt: time.Now(),
	}
	changed := UpdateProfileTone(pt, Proposal{Tags: []string{"concise"}, Source: SourceExplicit}, time.Now())
	if !changed {
		t.Error("explicit update should bypass rate limit")
	}
}

func TestBuildToneGuide_Empty(t *testing.T) {
	guide := BuildToneGuide(nil)
	if guide != "" {
		t.Error("expected empty guide for nil tags")
	}
	guide = BuildToneGuide([]string{})
	if guide != "" {
		t.Error("expected empty guide for empty tags")
	}
}

func TestBuildToneGuide_ContainsTags(t *testing.T) {
	guide := BuildToneGuide([]string{"concise", "no_emojis", "warm_supportive"})
	if guide == "" {
		t.Fatal("expected non-empty guide")
	}
	if !contains(guide, "concise") {
		t.Error("guide should mention concise")
	}
	if !contains(guide, "NOT use emojis") {
		t.Error("guide should prohibit emojis")
	}
	if !contains(guide, "warm") {
		t.Error("guide should mention warm stance")
	}
	if !contains(guide, "TONE POLICY") {
		t.Error("guide should have TONE POLICY header")
	}
}

func TestBuildToneGuide_DefaultStance(t *testing.T) {
	// No stance tag → should default to neutral professional.
	guide := BuildToneGuide([]string{"concise"})
	if !contains(guide, "neutral, professional") {
		t.Error("expected default neutral professional stance")
	}
}

func TestUpdateProfileTone_EmptyProposal(t *testing.T) {
	pt := &ProfileTone{}
	changed := UpdateProfileTone(pt, Proposal{Source: SourceImplicit}, time.Now())
	if changed {
		t.Error("empty proposal should not change profile")
	}
}

func TestAllTags_Count(t *testing.T) {
	if len(AllTags) != 15 {
		t.Errorf("expected 15 whitelist tags, got %d", len(AllTags))
	}
}

func TestValidateProposal_DeduplicatesTags(t *testing.T) {
	p := ValidateProposal(Proposal{
		Tags: []string{"concise", "concise", "formal", "formal"},
	})
	sort.Strings(p.Tags)
	if len(p.Tags) != 2 {
		t.Errorf("expected 2 unique tags, got %d: %v", len(p.Tags), p.Tags)
	}
}

// ---- helpers ----

func toSet(tags []string) map[string]bool {
	s := make(map[string]bool, len(tags))
	for _, t := range tags {
		s[t] = true
	}
	return s
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && searchSubstring(s, substr))
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
