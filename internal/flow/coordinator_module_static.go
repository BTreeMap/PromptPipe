// Package flow provides a static, rule-based coordinator implementation.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// StaticCoordinatorModule is a rule-based, non-LLM coordinator that deterministically
// routes the conversation between COORDINATOR, INTAKE, and FEEDBACK, and calls tools
// directly without relying on model reasoning.
type StaticCoordinatorModule struct {
	stateManager StateManager
	msgService   MessagingService

	// Tools reused from other modules
	schedulerTool       *SchedulerTool
	promptGeneratorTool *PromptGeneratorTool
	stateTransitionTool *StateTransitionTool
	profileSaveTool     *ProfileSaveTool
}

// NewStaticCoordinatorModule creates a new static coordinator instance.
func NewStaticCoordinatorModule(stateManager StateManager, msgService MessagingService, schedulerTool *SchedulerTool, promptGeneratorTool *PromptGeneratorTool, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool) *StaticCoordinatorModule {
	slog.Debug("StaticCoordinatorModule.NewStaticCoordinatorModule: creating static coordinator",
		"hasStateManager", stateManager != nil,
		"hasMessaging", msgService != nil,
		"hasScheduler", schedulerTool != nil,
		"hasPromptGenerator", promptGeneratorTool != nil,
		"hasStateTransition", stateTransitionTool != nil,
		"hasProfileSave", profileSaveTool != nil)
	return &StaticCoordinatorModule{
		stateManager:        stateManager,
		msgService:          msgService,
		schedulerTool:       schedulerTool,
		promptGeneratorTool: promptGeneratorTool,
		stateTransitionTool: stateTransitionTool,
		profileSaveTool:     profileSaveTool,
	}
}

// Ensure StaticCoordinatorModule implements Coordinator
var _ Coordinator = (*StaticCoordinatorModule)(nil)

// LoadSystemPrompt is a no-op for the static coordinator (kept for interface parity).
func (sc *StaticCoordinatorModule) LoadSystemPrompt() error { return nil }

// ProcessMessageWithHistory implements a deterministic state machine:
// - If profile is incomplete -> transition to INTAKE and prompt for missing fields
// - If profile is complete -> generate a prompt and transition to FEEDBACK
func (sc *StaticCoordinatorModule) ProcessMessageWithHistory(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
	if sc.stateManager == nil {
		return "", fmt.Errorf("state manager not initialized")
	}

	// Append user message to conversation history for persistence parity
	if conversationHistory != nil {
		conversationHistory.Messages = append(conversationHistory.Messages, ConversationMessage{Role: "user", Content: userMessage, Timestamp: time.Now()})
	}

	// Check profile completeness to drive the deterministic flow
	complete, missingFields := sc.isProfileComplete(ctx, participantID)

	if !complete {
		// Transition to INTAKE and prompt for required info
		if sc.stateTransitionTool != nil {
			_, _ = sc.stateTransitionTool.ExecuteStateTransition(ctx, participantID, map[string]interface{}{"target_state": "INTAKE", "reason": "collect profile (static flow)"})
		} else {
			_ = sc.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateIntake))
		}

		// Deterministic intake kick-off message
		need := "habit domain, motivation, preferred time, and a natural anchor"
		if len(missingFields) > 0 {
			need = strings.Join(missingFields, ", ")
		}
		return fmt.Sprintf("Let's set up your profile. Please share your %s.", need), nil
	}

	// Profile is complete -> generate a prompt now and transition to FEEDBACK
	var prompt string
	var err error
	if sc.promptGeneratorTool != nil {
		// Prefer history-aware generation if chatHistory provided
		if len(chatHistory) > 0 {
			prompt, err = sc.promptGeneratorTool.ExecutePromptGeneratorWithHistory(ctx, participantID, map[string]interface{}{"delivery_mode": "immediate"}, chatHistory)
		} else {
			prompt, err = sc.promptGeneratorTool.ExecutePromptGenerator(ctx, participantID, map[string]interface{}{"delivery_mode": "immediate"})
		}
	}

	if err != nil || prompt == "" {
		prompt = "After your next coffee, try a 1-minute stretch — it helps you reset and feel better."
	}

	// Transition to FEEDBACK to await the user's outcome
	if sc.stateTransitionTool != nil {
		_, _ = sc.stateTransitionTool.ExecuteStateTransition(ctx, participantID, map[string]interface{}{"target_state": "FEEDBACK", "reason": "await feedback (static flow)"})
	} else {
		_ = sc.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateFeedback))
	}

	return prompt, nil
}

// getProfileStatus mirrors the coordinator's profile status helper (simplified here)
func (sc *StaticCoordinatorModule) getProfileStatus(ctx context.Context, participantID string) string {
	profileJSON, err := sc.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil || profileJSON == "" {
		return "PROFILE STATUS: User has no profile."
	}
	var p UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return "PROFILE STATUS: User profile parse error."
	}
	missing := []string{}
	if p.HabitDomain == "" {
		missing = append(missing, "habit domain")
	}
	if p.MotivationalFrame == "" {
		missing = append(missing, "motivation")
	}
	if p.PreferredTime == "" {
		missing = append(missing, "preferred time")
	}
	if p.PromptAnchor == "" {
		missing = append(missing, "habit anchor")
	}
	if len(missing) > 0 {
		return "PROFILE STATUS: User profile is incomplete, missing: " + strings.Join(missing, ", ")
	}
	return "PROFILE STATUS: User profile is complete."
}

func (sc *StaticCoordinatorModule) defaultCoordinatorReply(ctx context.Context, participantID string, profileComplete bool) (string, error) {
	// Basic guidance with deterministic next steps
	if !profileComplete {
		return "Welcome! To personalize your habit prompts, tell me your goal area, your motivation, a preferred time, and a natural anchor (like after coffee). I can then set you up.", nil
	}
	return "You're all set! Ask me to schedule daily reminders (e.g., 'schedule daily at 09:00') or say 'give me a prompt' to get one now.", nil
}

// Helpers: naive parsers for time and schedule ID
func extractTimeLike(s string) string {
	// very small heuristic: look for hh:mm pattern
	for i := 0; i+5 <= len(s); i++ {
		seg := s[i : i+5]
		if seg[2] == ':' {
			// quick digit check
			if seg[0] >= '0' && seg[0] <= '2' && seg[1] >= '0' && seg[1] <= '9' && seg[3] >= '0' && seg[3] <= '5' && seg[4] >= '0' && seg[4] <= '9' {
				return seg
			}
		}
	}
	return ""
}

func extractScheduleID(s string) string {
	// find token starting with sched_
	parts := strings.Fields(s)
	for _, p := range parts {
		if strings.HasPrefix(p, "sched_") {
			return p
		}
	}
	return ""
}

// isProfileComplete checks required fields and returns completeness and missing list in user-friendly order
func (sc *StaticCoordinatorModule) isProfileComplete(ctx context.Context, participantID string) (bool, []string) {
	profileJSON, err := sc.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyUserProfile)
	if err != nil || profileJSON == "" {
		return false, []string{"habit domain", "motivation", "preferred time", "habit anchor"}
	}
	var p UserProfile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return false, []string{"habit domain", "motivation", "preferred time", "habit anchor"}
	}
	missing := []string{}
	if p.HabitDomain == "" {
		missing = append(missing, "habit domain")
	}
	if p.MotivationalFrame == "" {
		missing = append(missing, "motivation")
	}
	if p.PreferredTime == "" {
		missing = append(missing, "preferred time")
	}
	if p.PromptAnchor == "" {
		missing = append(missing, "habit anchor")
	}
	return len(missing) == 0, missing
}
