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

// ProcessMessageWithHistory implements deterministic routing.
func (sc *StaticCoordinatorModule) ProcessMessageWithHistory(ctx context.Context, participantID, userMessage string, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
	if sc.stateManager == nil {
		return "", fmt.Errorf("state manager not initialized")
	}

	// Append user message to conversation history for persistence parity
	if conversationHistory != nil {
		conversationHistory.Messages = append(conversationHistory.Messages, ConversationMessage{Role: "user", Content: userMessage, Timestamp: time.Now()})
	}

	// Decide next action via rules
	stateStr, err := sc.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState)
	if err != nil {
		slog.Warn("StaticCoordinator: failed to get conversation state, defaulting to COORDINATOR", "error", err, "participantID", participantID)
		stateStr = string(models.StateCoordinator)
	}
	current := models.StateType(stateStr)
	if current == "" {
		current = models.StateCoordinator
	}

	// High-level rules:
	// 1) If user mentions profile info keywords and profile incomplete -> transition to INTAKE.
	// 2) If user asks for schedule/list/delete and scheduler is available -> call scheduler.
	// 3) If profile exists and complete and user asks for a prompt -> generate prompt.
	// 4) If responding to a prompt (feedback-like words) -> transition to FEEDBACK.
	// 5) Otherwise, provide a deterministic coordinator reply and keep state.

	// Rule helpers
	lower := strings.ToLower(userMessage)
	has := func(kw ...string) bool {
		for _, k := range kw {
			if strings.Contains(lower, k) {
				return true
			}
		}
		return false
	}

	// Profile completeness check
	profileStatus := sc.getProfileStatus(ctx, participantID)
	profileComplete := strings.Contains(profileStatus, "profile is complete")

	// 1) Intake routing if user is supplying profile-like data and profile incomplete
	if !profileComplete && has("my goal", "i want", "habit", "domain", "time", "anchor", "motivation", "prefer") {
		if sc.stateTransitionTool != nil {
			_, _ = sc.stateTransitionTool.ExecuteStateTransition(ctx, participantID, map[string]interface{}{"target_state": "INTAKE", "reason": "collect profile"})
		} else {
			_ = sc.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateIntake))
		}
		return "Got it! I'll connect you with our intake specialist to set up your profile.", nil
	}

	// 2) Scheduler intents
	if sc.schedulerTool != nil {
		// create schedule
		if has("schedule", "reminder") && (has("daily", "every day") || has("at ")) {
			// naive parse: look for time like HH:MM within message
			timeStr := extractTimeLike(lower)
			if timeStr != "" {
				params := models.SchedulerToolParams{Action: models.SchedulerActionCreate, Type: models.SchedulerTypeFixed, FixedTime: timeStr}
				res, _ := sc.schedulerTool.ExecuteScheduler(ctx, participantID, params)
				if res != nil && res.Message != "" {
					return res.Message, nil
				}
				return "I tried to set up your daily reminder.", nil
			}
		}
		// list
		if has("list", "my schedules", "what's scheduled") {
			params := models.SchedulerToolParams{Action: models.SchedulerActionList}
			res, _ := sc.schedulerTool.ExecuteScheduler(ctx, participantID, params)
			if res != nil && res.Message != "" {
				return res.Message, nil
			}
		}
		// delete by id
		if has("delete", "remove") && has("sched_") {
			id := extractScheduleID(lower)
			if id != "" {
				params := models.SchedulerToolParams{Action: models.SchedulerActionDelete, ScheduleID: id}
				res, _ := sc.schedulerTool.ExecuteScheduler(ctx, participantID, params)
				if res != nil && res.Message != "" {
					return res.Message, nil
				}
			}
		}
	}

	// 3) Prompt generation when profile complete and user asks for a prompt
	if profileComplete && sc.promptGeneratorTool != nil && has("prompt", "habit idea", "give me a", "what should i do") {
		msg, err := sc.promptGeneratorTool.ExecutePromptGenerator(ctx, participantID, map[string]interface{}{"delivery_mode": "immediate"})
		if err == nil && msg != "" {
			return msg, nil
		}
		return "Here's a simple habit to try today: take a 2-minute mindful breathing break after your coffee.", nil
	}

	// 4) Feedback-like responses -> transition to FEEDBACK
	if has("i did it", "done", "completed", "couldn't", "barrier", "did not", "tweak") {
		if sc.stateTransitionTool != nil {
			_, _ = sc.stateTransitionTool.ExecuteStateTransition(ctx, participantID, map[string]interface{}{"target_state": "FEEDBACK", "reason": "collect feedback"})
		} else {
			_ = sc.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationState, string(models.StateFeedback))
		}
		return "Thanks! Let's log your feedback.", nil
	}

	// Default coordinator reply
	return sc.defaultCoordinatorReply(ctx, participantID, profileComplete)
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
