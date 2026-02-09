// Package tone provides a fixed whitelist of user tone tags, validation,
// EMA-based smoothing, mutual-exclusion enforcement, and prompt-guide
// construction for the implicit user-tone feature.
package tone

import (
	"math"
	"strings"
	"time"
)

// ---- Whitelist ----

// AllTags is the hard-coded set of safe tone tags.
var AllTags = map[string]bool{
	// Style
	"concise":               true,
	"detailed":              true,
	"formal":                true,
	"casual":                true,
	"no_emojis":             true,
	"emojis_ok":             true,
	"bullet_points":         true,
	"one_question_at_a_time": true,
	// Stance
	"warm_supportive":       true,
	"neutral_professional":  true,
	"direct_coach":          true,
	"gentle_coach":          true,
	// Interaction
	"confirm_before_acting": true,
	"default_actionable":    true,
	"high_autonomy":         true,
}

// mutuallyExclusivePairs defines tags where at most one may be active.
var mutuallyExclusivePairs = [][2]string{
	{"concise", "detailed"},
	{"formal", "casual"},
	{"direct_coach", "gentle_coach"},
}

// ---- Data types ----

// UpdateSource enumerates how a tone update was triggered.
type UpdateSource string

const (
	SourceExplicit UpdateSource = "explicit"
	SourceImplicit UpdateSource = "implicit"
)

// Proposal is what the LLM sends when it proposes tone tags.
type Proposal struct {
	Tags       []string           `json:"tone_tags,omitempty"`
	Scores     map[string]float32 `json:"tone_scores,omitempty"`
	Source     UpdateSource       `json:"tone_update_source,omitempty"`
	Confidence float32            `json:"tone_confidence,omitempty"`
}

// ProfileTone stores the persistent tone fields inside a user profile.
type ProfileTone struct {
	Tags            []string           `json:"tone_tags,omitempty"`
	Scores          map[string]float32 `json:"tone_scores,omitempty"`
	Version         int                `json:"tone_version"`
	LastUpdatedAt   time.Time          `json:"tone_last_updated_at,omitempty"`
	UpdateSource    UpdateSource       `json:"tone_update_source,omitempty"`
	OverrideUntil   *time.Time         `json:"tone_override_until,omitempty"`
}

// ---- Constants for EMA / hysteresis ----

const (
	alpha             = float32(0.15)
	activateThreshold = float32(0.7)
	deactivateThresh  = float32(0.4)
	// Rate-limit: minimum interval between implicit persists.
	minImplicitInterval = 3 * time.Minute
)

// ---- Public API ----

// ValidateProposal strips unknown tags, clamps scores, and returns a cleaned proposal.
func ValidateProposal(p Proposal) Proposal {
	cleaned := Proposal{
		Source:     p.Source,
		Confidence: p.Confidence,
	}
	if cleaned.Source == "" {
		cleaned.Source = SourceImplicit
	}

	// Filter tags.
	seen := map[string]bool{}
	for _, t := range p.Tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if AllTags[t] && !seen[t] {
			cleaned.Tags = append(cleaned.Tags, t)
			seen[t] = true
		}
	}

	// Filter and clamp scores.
	if len(p.Scores) > 0 {
		cleaned.Scores = make(map[string]float32, len(p.Scores))
		for k, v := range p.Scores {
			k = strings.TrimSpace(strings.ToLower(k))
			if !AllTags[k] {
				continue
			}
			cleaned.Scores[k] = clamp(v)
		}
	}

	return cleaned
}

// UpdateProfileTone applies a validated proposal to the profile tone using EMA smoothing
// and hysteresis. It enforces mutual exclusion and rate limits. Returns true if the
// profile was actually mutated.
func UpdateProfileTone(pt *ProfileTone, proposal Proposal, now time.Time) bool {
	if pt.Version == 0 {
		pt.Version = 1
	}
	if pt.Scores == nil {
		pt.Scores = make(map[string]float32)
	}

	// Rate-limit implicit updates.
	if proposal.Source == SourceImplicit {
		if !pt.LastUpdatedAt.IsZero() && now.Sub(pt.LastUpdatedAt) < minImplicitInterval {
			return false
		}
	}

	// Build observation map from proposal.
	obs := make(map[string]float32)
	for _, t := range proposal.Tags {
		obs[t] = 1.0
	}
	// Merge explicit scores if provided (they override the binary 1.0 presence).
	for k, v := range proposal.Scores {
		obs[k] = v
	}

	if len(obs) == 0 {
		return false
	}

	changed := false

	if proposal.Source == SourceExplicit {
		// Explicit: apply immediately with full weight.
		for tag, v := range obs {
			prev := pt.Scores[tag]
			pt.Scores[tag] = clamp(v)
			if pt.Scores[tag] != prev {
				changed = true
			}
		}
	} else {
		// Implicit: EMA smoothing for observed tags.
		for tag, v := range obs {
			prev := pt.Scores[tag]
			pt.Scores[tag] = clamp((1-alpha)*prev + alpha*v)
			if pt.Scores[tag] != prev {
				changed = true
			}
		}
		// Decay non-observed tags toward 0 so deactivation can occur.
		for tag, prev := range pt.Scores {
			if _, observed := obs[tag]; observed {
				continue
			}
			if prev <= 0 {
				continue
			}
			decayed := clamp((1 - alpha) * prev)
			if decayed != prev {
				pt.Scores[tag] = decayed
				changed = true
			}
		}
	}

	if !changed {
		return false
	}

	// Apply no_emojis overrides emojis_ok.
	if pt.Scores["no_emojis"] >= activateThreshold {
		pt.Scores["emojis_ok"] = 0
	}

	// Enforce mutual exclusion: keep the higher score.
	for _, pair := range mutuallyExclusivePairs {
		a, b := pair[0], pair[1]
		sa, sb := pt.Scores[a], pt.Scores[b]
		if sa >= activateThreshold && sb >= activateThreshold {
			if sa >= sb {
				pt.Scores[b] = deactivateThresh - 0.01
			} else {
				pt.Scores[a] = deactivateThresh - 0.01
			}
		}
	}

	// Rebuild active tags from scores using hysteresis.
	activeSet := make(map[string]bool)
	for _, t := range pt.Tags {
		activeSet[t] = true
	}

	for tag, score := range pt.Scores {
		if score >= activateThreshold {
			activeSet[tag] = true
		} else if score <= deactivateThresh {
			delete(activeSet, tag)
		}
		// Between thresholds: keep current state (hysteresis).
	}

	// no_emojis overrides emojis_ok in active set as well.
	if activeSet["no_emojis"] {
		delete(activeSet, "emojis_ok")
	}

	// Rebuild tags slice.
	newTags := make([]string, 0, len(activeSet))
	for t := range activeSet {
		newTags = append(newTags, t)
	}
	pt.Tags = newTags
	pt.LastUpdatedAt = now
	pt.UpdateSource = proposal.Source
	pt.Version = 1

	return true
}

// BuildToneGuide produces a compact instruction snippet for injection into LLM system prompts.
// It returns an empty string when there are no active tags.
func BuildToneGuide(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	set := make(map[string]bool, len(tags))
	for _, t := range tags {
		set[t] = true
	}

	var b strings.Builder
	b.WriteString("\n<TONE POLICY>\nAdapt your responses to the user's communication style:\n")

	// Style rules.
	if set["concise"] {
		b.WriteString("- Be concise: short sentences, minimal filler.\n")
	}
	if set["detailed"] {
		b.WriteString("- Be detailed: provide slightly more explanation, but avoid rambling.\n")
	}
	if set["formal"] {
		b.WriteString("- Use formal diction and professional register.\n")
	}
	if set["casual"] {
		b.WriteString("- Use casual, friendly language.\n")
	}
	if set["no_emojis"] {
		b.WriteString("- Do NOT use emojis.\n")
	} else if set["emojis_ok"] {
		b.WriteString("- Emojis are welcome where appropriate.\n")
	}
	if set["bullet_points"] {
		b.WriteString("- Prefer bullet points when listing items.\n")
	}
	if set["one_question_at_a_time"] {
		b.WriteString("- Ask only one question at a time.\n")
	}

	// Stance rules.
	hasStance := false
	if set["warm_supportive"] {
		b.WriteString("- Adopt a warm, supportive stance. Encourage the user.\n")
		hasStance = true
	}
	if set["neutral_professional"] {
		b.WriteString("- Keep a neutral, professional stance.\n")
		hasStance = true
	}
	if set["direct_coach"] {
		b.WriteString("- Be a direct coach: clear, action-oriented feedback.\n")
		hasStance = true
	}
	if set["gentle_coach"] {
		b.WriteString("- Be a gentle coach: patient, encouraging guidance.\n")
		hasStance = true
	}
	if !hasStance {
		// Default stance.
		b.WriteString("- Keep a neutral, professional stance.\n")
	}

	// Interaction rules.
	if set["confirm_before_acting"] {
		b.WriteString("- Confirm with the user before taking actions.\n")
	}
	if set["default_actionable"] {
		b.WriteString("- Provide actionable next steps by default.\n")
	}
	if set["high_autonomy"] {
		b.WriteString("- Act with high autonomy; minimize confirmations.\n")
	}

	b.WriteString("- NEVER mirror hostility, sarcasm, insults, or unsafe language.\n")
	b.WriteString("</TONE POLICY>\n")

	return b.String()
}

// ---- helpers ----

func clamp(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	// Round to 4 decimal places to avoid floating point drift.
	return float32(math.Round(float64(v)*10000) / 10000)
}
