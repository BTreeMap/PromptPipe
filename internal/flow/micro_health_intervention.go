package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// State constants for micro health intervention flow.
const (
	StateOrientation                  = "ORIENTATION"
	StateCommitmentPrompt             = "COMMITMENT_PROMPT"
	StateFeelingPrompt                = "FEELING_PROMPT"
	StateRandomAssignment             = "RANDOM_ASSIGNMENT"
	StateSendInterventionImmediate    = "SEND_INTERVENTION_IMMEDIATE"
	StateSendInterventionReflective   = "SEND_INTERVENTION_REFLECTIVE"
	StateReinforcementFollowup        = "REINFORCEMENT_FOLLOWUP"
	StateDidYouGetAChance             = "DID_YOU_GET_A_CHANCE"
	StateContextQuestion              = "CONTEXT_QUESTION"
	StateMoodQuestion                 = "MOOD_QUESTION"
	StateBarrierCheckAfterContextMood = "BARRIER_CHECK_AFTER_CONTEXT_MOOD"
	StateBarrierReasonNoChance        = "BARRIER_REASON_NO_CHANCE"
	StateIgnoredPath                  = "IGNORED_PATH"
	StateEndOfDay                     = "END_OF_DAY"
	StateWeeklySummary                = "WEEKLY_SUMMARY"
	StateComplete                     = "COMPLETE"

	// Legacy states for backward compatibility
	StateHabitReminder = "HABIT_REMINDER"
	StateFollowUp      = "FOLLOW_UP"
)

// Message templates for micro health intervention flow.
const (
	MsgOrientation            = "Hi! 🌱 Welcome to our Healthy Habits study!\nHere's how it works: You will receive messages on a schedule, or type 'Ready' anytime to get a prompt. Your input is important."
	MsgCommitment             = "You committed to a quick habit today—ready to go?\n1. 🚀 Let's do it!\n2. ⏳ Not yet\n(Reply with '1' or '2')"
	MsgFeeling                = "How do you feel about this first step?\n1. 😊 Excited\n2. 🤔 Curious\n3. 😃 Motivated\n4. 📖 Need info\n5. ⚖️ Not sure\n(Reply with '1'–'5')"
	MsgRandomAssignment       = "Based on your profile, we're assigning you to a personalized track. Please wait for your next message."
	MsgInterventionImmediate  = "Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you're done, reply 'Done.'"
	MsgInterventionReflective = "Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply 'Done.'"
	MsgReinforcement          = "Great job! 🎉 You just completed your habit in under one minute—keep it up!"
	MsgDidYouGetAChance       = "Did you get a chance to try it?\n1. Yes\n2. No\n(Reply with '1' or '2')"
	MsgContextQuestion        = "You did it! What was happening around you?\n1. Alone & focused\n2. With others around\n3. In a distracting place\n4. Busy & stressed\n(Reply with '1'–'4')"
	MsgMoodQuestion           = "What best describes your mood before doing this?\n1. 🙂 Relaxed\n2. 😐 Neutral\n3. 😫 Stressed\n(Reply with '1', '2', or '3')"
	MsgBarrierCheck           = "Did something make this easier or harder today? What was it?"
	MsgBarrierReason          = "Could you let me know why you couldn't do it this time?\n1. I didn't have enough time\n2. I didn't understand the task\n3. I didn't feel motivated to do it\n4. Other (please specify)\n(Reply with '1', '2', '3', or '4')"
	MsgIgnoredPath1           = "What kept you from doing it today? Reply with one word, a quick audio, or a short video!"
	MsgIgnoredPath2           = "Building awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day."
	MsgWeeklySummary          = "Great job this week! 🎉 You completed your habit %d times in the past 7 days! 🙌 Keep up the momentum—small actions add up!"
	MsgComplete               = "🎉 Congratulations! You've completed the micro health intervention. Thank you for participating!"

	// Legacy messages for backward compatibility
	MsgHabitReminder = "⏰ Reminder: It's time for your healthy habit! How did it go?\n1. ✅ Completed\n2. ⏳ Will do later\n3. ❌ Skipped today\n(Reply with '1', '2', or '3')"
	MsgFollowUp      = "Great progress! 📈 How are you feeling about your habit journey?\n1. 😊 Going well\n2. 🤔 Mixed feelings\n3. 😓 Struggling\n(Reply with '1', '2', or '3')"
)

// MicroHealthInterventionGenerator implements a custom, stateful micro health intervention flow.
type MicroHealthInterventionGenerator struct {
	stateManager StateManager
	timer        Timer
}

// NewMicroHealthInterventionGenerator creates a new generator with dependencies.
func NewMicroHealthInterventionGenerator(stateManager StateManager, timer Timer) *MicroHealthInterventionGenerator {
	slog.Debug("Creating MicroHealthInterventionGenerator with dependencies")
	return &MicroHealthInterventionGenerator{
		stateManager: stateManager,
		timer:        timer,
	}
}

// SetDependencies injects dependencies into the generator.
func (g *MicroHealthInterventionGenerator) SetDependencies(deps Dependencies) {
	slog.Debug("MicroHealthInterventionGenerator SetDependencies called")
	g.stateManager = deps.StateManager
	g.timer = deps.Timer
}

// Generate selects the next message based on the current state in p.State.
func (g *MicroHealthInterventionGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("MicroHealthIntervention Generate invoked", "state", p.State, "to", p.To)

	// For simple message generation, dependencies are not required
	// Dependencies are only needed for stateful operations like state transitions and timers
	switch p.State {
	case "", StateOrientation:
		slog.Debug("MicroHealthIntervention state orientation", "to", p.To)
		return MsgOrientation, nil
	case StateCommitmentPrompt:
		slog.Debug("MicroHealthIntervention state commitment prompt", "to", p.To)
		return MsgCommitment, nil
	case StateFeelingPrompt:
		slog.Debug("MicroHealthIntervention state feeling prompt", "to", p.To)
		return MsgFeeling, nil
	case StateRandomAssignment:
		slog.Debug("MicroHealthIntervention state random assignment", "to", p.To)
		return MsgRandomAssignment, nil
	case StateSendInterventionImmediate:
		slog.Debug("MicroHealthIntervention state intervention immediate", "to", p.To)
		return MsgInterventionImmediate, nil
	case StateSendInterventionReflective:
		slog.Debug("MicroHealthIntervention state intervention reflective", "to", p.To)
		return MsgInterventionReflective, nil
	case StateReinforcementFollowup:
		slog.Debug("MicroHealthIntervention state reinforcement followup", "to", p.To)
		return MsgReinforcement, nil
	case StateDidYouGetAChance:
		slog.Debug("MicroHealthIntervention state did you get a chance", "to", p.To)
		return MsgDidYouGetAChance, nil
	case StateContextQuestion:
		slog.Debug("MicroHealthIntervention state context question", "to", p.To)
		return MsgContextQuestion, nil
	case StateMoodQuestion:
		slog.Debug("MicroHealthIntervention state mood question", "to", p.To)
		return MsgMoodQuestion, nil
	case StateBarrierCheckAfterContextMood:
		slog.Debug("MicroHealthIntervention state barrier check", "to", p.To)
		return MsgBarrierCheck, nil
	case StateBarrierReasonNoChance:
		slog.Debug("MicroHealthIntervention state barrier reason", "to", p.To)
		return MsgBarrierReason, nil
	case StateIgnoredPath:
		slog.Debug("MicroHealthIntervention state ignored path", "to", p.To)
		// For ignored path, we send both messages
		return MsgIgnoredPath1 + "\n\n" + MsgIgnoredPath2, nil
	case StateWeeklySummary:
		slog.Debug("MicroHealthIntervention state weekly summary", "to", p.To)
		// Weekly summary needs completion count - this will be handled by the API layer
		return MsgWeeklySummary, nil
	case StateEndOfDay:
		slog.Debug("MicroHealthIntervention state end of day", "to", p.To)
		// End of day doesn't send a message, it's a terminal state
		return "", nil
	case StateComplete:
		slog.Debug("MicroHealthIntervention state complete", "to", p.To)
		return MsgComplete, nil

	// Legacy states for backward compatibility
	case StateHabitReminder:
		slog.Debug("MicroHealthIntervention state habit reminder", "to", p.To)
		return MsgHabitReminder, nil
	case StateFollowUp:
		slog.Debug("MicroHealthIntervention state follow up", "to", p.To)
		return MsgFollowUp, nil
	default:
		slog.Error("MicroHealthIntervention unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf("unsupported micro health intervention state '%s'", p.State)
	}
}

// ProcessResponse handles participant responses and manages state transitions.
// This method requires dependencies to be properly initialized.
func (g *MicroHealthInterventionGenerator) ProcessResponse(ctx context.Context, participantID, response string) error {
	// Validate dependencies for stateful operations
	if g.stateManager == nil || g.timer == nil {
		slog.Error("MicroHealthIntervention dependencies not initialized for state operations")
		return fmt.Errorf("generator dependencies not properly initialized for state operations")
	}

	// Implementation would handle response processing and state transitions
	// This is placeholder for future implementation
	slog.Debug("MicroHealthIntervention ProcessResponse", "participantID", participantID, "response", response)
	return nil
}

// Note: Removed unsafe global registration - custom generators should be registered
// manually with proper dependency injection in the application startup code.
