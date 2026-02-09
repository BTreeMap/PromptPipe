# Tone Feature

This document describes the implicit user tone adaptation feature, which allows the system to infer and adapt to each user's communication style.

## Overview

The system infers a user's preferred communication style from interactions and persists it safely in their profile. Bot responses are then adapted to match the user's tone. The design defends against prompt injection and tool-steering by storing only a fixed, hard-coded vocabulary of tags—never free-form text.

## Whitelist Vocabulary (15 tags)

### Style
| Tag | Description |
|-----|-------------|
| `concise` | Short sentences, minimal filler |
| `detailed` | Slightly more explanation, avoid rambling |
| `formal` | Professional register and diction |
| `casual` | Friendly, conversational language |
| `no_emojis` | Never use emojis |
| `emojis_ok` | Emojis are welcome where appropriate |
| `bullet_points` | Prefer bullet points when listing items |
| `one_question_at_a_time` | Ask only one question per turn |

### Stance
| Tag | Description |
|-----|-------------|
| `warm_supportive` | Warm, encouraging stance |
| `neutral_professional` | Neutral, professional stance (default) |
| `direct_coach` | Clear, action-oriented feedback |
| `gentle_coach` | Patient, encouraging guidance |

### Interaction
| Tag | Description |
|-----|-------------|
| `confirm_before_acting` | Confirm with user before taking actions |
| `default_actionable` | Provide actionable next steps by default |
| `high_autonomy` | Act with high autonomy; minimize confirmations |

## Mutual Exclusion and Override Rules

- **concise XOR detailed**: At most one may be active. Higher score wins.
- **formal XOR casual**: At most one may be active. Higher score wins.
- **direct_coach XOR gentle_coach**: At most one may be active. Higher score wins.
- **no_emojis overrides emojis_ok**: When no_emojis is active, emojis_ok is forced to 0.
- **Default stance**: If no stance tag is active, the system defaults to `neutral_professional` in the tone guide.

## Data Model

Tone fields are stored inside the `UserProfile` struct (serialized as JSON in flow state):

```go
type ProfileTone struct {
    Tags            []string           `json:"tone_tags,omitempty"`
    Scores          map[string]float32 `json:"tone_scores,omitempty"`
    Version         int                `json:"tone_version"`
    LastUpdatedAt   time.Time          `json:"tone_last_updated_at,omitempty"`
    UpdateSource    UpdateSource       `json:"tone_update_source,omitempty"`
    OverrideUntil   *time.Time         `json:"tone_override_until,omitempty"`
}
```

- **Tags**: Active subset of whitelist tags.
- **Scores**: EMA-smoothed scores (0–1) for each observed tag.
- **Version**: Schema version (currently 1).
- **LastUpdatedAt**: Timestamp of last tone update.
- **UpdateSource**: `"explicit"` (user requested) or `"implicit"` (inferred).
- **OverrideUntil**: Optional timestamp for session-level tone override (reserved for future use).

### Backward Compatibility

Existing profiles without tone fields load with defaults: empty tags, nil scores, version 1. No database migration is needed because profiles are stored as JSON blobs in the `flow_states` table.

## Inference Method

### Update Sources

- **Explicit** (`tone_update_source: "explicit"`): User directly requests a communication style change (e.g., "please be more concise"). Applied immediately with full weight.
- **Implicit** (`tone_update_source: "implicit"`): System infers tone from conversation patterns. Applied via EMA smoothing with hysteresis.

### Which Bots Propose Tone Updates

- **Intake bot**: May propose tone updates when explicit request or high-confidence repeated pattern.
- **Feedback bot** (track_feedback): Primary tone learner. Infers tone from ongoing user language.
- **Coordinator**: Matches tone in responses but **cannot** write to user profile.
- **Generator**: Minimal tone usage; preserves output format.

## Smoothing and Gating

### EMA (Exponential Moving Average)

For implicit updates:
```
score[tag] = (1 - α) × score[tag] + α × observation
```
- α = 0.15
- Observation is 1.0 when tag is proposed, 0.0 when absent (decay).

### Hysteresis Thresholds

- **Activate**: Score ≥ 0.7 → tag becomes active.
- **Deactivate**: Score ≤ 0.4 → tag becomes inactive.
- **Between**: Tag retains its current state (prevents oscillation).

### Decay

On implicit updates, non-observed tags decay toward 0 via EMA. This allows tags to deactivate naturally when the user's style changes.

### Rate Limiting

Implicit updates are limited to at most one per 3 minutes per user. Explicit updates bypass this limit.

### Server-Side Validation

Even when the LLM proposes tone tags, the server always:
1. Strips unknown tags (not in whitelist).
2. Clamps scores to [0, 1].
3. Applies mutual exclusion rules.
4. Enforces rate limits for implicit updates.
5. Applies EMA smoothing and hysteresis.

## How Prompts Use Tone

When building system prompts for any bot module, the `tone.BuildToneGuide(tags)` function generates a compact `<TONE POLICY>` section appended to the system messages. This section instructs the LLM to:

- Match brevity and formality per active tags.
- Apply the appropriate stance.
- Never mirror hostility, sarcasm, insults, or unsafe language.

The guide is empty when no tone tags are active, adding zero overhead for new users.

## How to Add New Tags in Future

1. Add the tag string to `AllTags` in `internal/tone/tone.go`.
2. If the tag has a mutually exclusive partner, add it to `mutuallyExclusivePairs`.
3. Add a corresponding rule in `BuildToneGuide`.
4. Update the `TestAllTags_Count` test to match the new count.
5. Update the whitelist description in the bot prompt files.
6. Update this document.
