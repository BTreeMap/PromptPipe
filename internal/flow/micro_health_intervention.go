package flow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Simple message constants for micro health intervention flow.
const (
	MsgOrientation      = "Hi! 🌱 Welcome to our Healthy Habits study!\nHere's how it works: You will receive messages on a schedule, or type 'Ready' anytime to get a prompt. Your input is important."
	MsgRandomAssignment = "Based on your profile, we're assigning you to a personalized track. Please wait for your next message."
	MsgComplete         = "🎉 Congratulations! You've completed the micro health intervention. Thank you for participating!"

	// Other intervention messages
	MsgBarrierDetail           = "Did something make this easier or harder today? What was it?"
	MsgImmediateIntervention   = "Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you're done, reply 'Done.'"
	MsgReflectiveIntervention  = "Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply 'Done.'"
	MsgReinforcement           = "Great job! 🎉 You just completed your habit in under one minute—keep it up!"
	MsgIgnoredPath             = "What kept you from doing it today? Reply with one word, a quick audio, or a short video!\n\nBuilding awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day."
	MsgEndOfDay                = "That's okay! We'll check back with you tomorrow. Take care! 🌙"
	MsgInvalidCommitmentChoice = "Please reply with '1' for 'Let's do it!' or '2' for 'Not yet'."
	MsgInvalidFeelingChoice    = "Please reply with '1' through '5' to indicate your feeling."
	MsgInvalidDidYouGetAChance = "Please reply with '1' for 'Yes' or '2' for 'No'."
	MsgInvalidContextChoice    = "Please reply with '1' through '4' to describe your context."
	MsgInvalidMoodChoice       = "Please reply with '1' through '3' to describe your mood."
	MsgInvalidBarrierChoice    = "Please reply with '1' through '4' to indicate the barrier reason."
)

// Timeout duration constants based on micro health intervention documentation
const (
	CommitmentTimeout       = 12 * time.Hour   // COMMITMENT_PROMPT timeout
	FeelingTimeout          = 15 * time.Minute // FEELING_PROMPT timeout
	CompletionTimeout       = 30 * time.Minute // SEND_INTERVENTION_* timeout
	DidYouGetAChanceTimeout = 15 * time.Minute // DID_YOU_GET_A_CHANCE timeout
	ContextTimeout          = 15 * time.Minute // CONTEXT_QUESTION timeout
	MoodTimeout             = 15 * time.Minute // MOOD_QUESTION timeout
	BarrierCheckTimeout     = 15 * time.Minute // BARRIER_CHECK_AFTER_CONTEXT_MOOD timeout
	BarrierReasonTimeout    = 15 * time.Minute // BARRIER_REASON_NO_CHANCE timeout
)

// Pre-generated message strings from structured data - initialized in init()
var (
	MsgCommitment       string
	MsgFeeling          string
	MsgHabitReminder    string
	MsgFollowUp         string
	MsgDidYouGetAChance string
	MsgContext          string
	MsgMood             string
	MsgBarrierReason    string
)

// Global structured branch messages - initialized in init()
var (
	CommitmentMessage       *models.Branch
	FeelingMessage          *models.Branch
	HabitReminderMessage    *models.Branch
	FollowUpMessage         *models.Branch
	DidYouGetAChanceMessage *models.Branch
	ContextMessage          *models.Branch
	MoodMessage             *models.Branch
	BarrierReasonMessage    *models.Branch
)

// init initializes all structured branch messages and generates string versions
func init() {
	CommitmentMessage = &models.Branch{
		Body: "You committed to trying a quick habit today—ready to go?",
		Options: []models.BranchOption{
			{Label: "🚀 Let's do it!", Body: "Continue"},
			{Label: "⏳ Not yet", Body: "Let's try again tomorrow"},
		},
	}

	FeelingMessage = &models.Branch{
		Body: "How do you feel about this first step?",
		Options: []models.BranchOption{
			{Label: "😊 Excited", Body: "Great energy!"},
			{Label: "🤔 Curious", Body: "Perfect mindset!"},
			{Label: "😃 Motivated", Body: "Let's channel that motivation!"},
			{Label: "📖 Need info", Body: "We'll guide you through it!"},
			{Label: "⚖️ Not sure", Body: "That's completely normal!"},
		},
	}

	HabitReminderMessage = &models.Branch{
		Body: "⏰ Reminder: It's time for your healthy habit! How did it go?",
		Options: []models.BranchOption{
			{Label: "✅ Completed", Body: "Excellent work!"},
			{Label: "⏳ Will do later", Body: "We'll check back with you!"},
			{Label: "❌ Skipped today", Body: "No worries, tomorrow is a new day!"},
		},
	}

	FollowUpMessage = &models.Branch{
		Body: "Great progress! 📈 How are you feeling about your habit journey?",
		Options: []models.BranchOption{
			{Label: "😊 Going well", Body: "Keep up the great work!"},
			{Label: "🤔 Mixed feelings", Body: "That's normal - progress isn't always linear!"},
			{Label: "😓 Struggling", Body: "We're here to support you!"},
		},
	}

	DidYouGetAChanceMessage = &models.Branch{
		Body: "Did you get a chance to try it?",
		Options: []models.BranchOption{
			{Label: "Yes", Body: "Great! Let's explore more."},
			{Label: "No", Body: "Let's understand what happened."},
		},
	}

	ContextMessage = &models.Branch{
		Body: "You did it! What was happening around you?",
		Options: []models.BranchOption{
			{Label: "Alone & focused", Body: "Perfect environment!"},
			{Label: "With others around", Body: "Great despite distractions!"},
			{Label: "In a distracting place", Body: "Impressive focus!"},
			{Label: "Busy & stressed", Body: "Amazing that you made time!"},
		},
	}

	MoodMessage = &models.Branch{
		Body: "What best describes your mood before doing this?",
		Options: []models.BranchOption{
			{Label: "🙂 Relaxed", Body: "Perfect state for building habits!"},
			{Label: "😐 Neutral", Body: "A calm approach works well!"},
			{Label: "😫 Stressed", Body: "Great that you prioritized self-care!"},
		},
	}

	BarrierReasonMessage = &models.Branch{
		Body: "Could you let me know why you couldn't do it this time?",
		Options: []models.BranchOption{
			{Label: "I didn't have enough time", Body: "Time management is key - let's work on that!"},
			{Label: "I didn't understand the task", Body: "Let's clarify the instructions!"},
			{Label: "I didn't feel motivated to do it", Body: "Motivation fluctuates - that's normal!"},
			{Label: "Other (please specify)", Body: "Please share more details."},
		},
	}

	// Generate string versions from structured data
	MsgCommitment = CommitmentMessage.Generate()
	MsgFeeling = FeelingMessage.Generate()
	MsgHabitReminder = HabitReminderMessage.Generate()
	MsgFollowUp = FollowUpMessage.Generate()
	MsgDidYouGetAChance = DidYouGetAChanceMessage.Generate()
	MsgContext = ContextMessage.Generate()
	MsgMood = MoodMessage.Generate()
	MsgBarrierReason = BarrierReasonMessage.Generate()
}

// MicroHealthInterventionGenerator implements a custom, stateful micro health intervention flow.
type MicroHealthInterventionGenerator struct {
	stateManager StateManager
	timer        models.Timer
}

// NewMicroHealthInterventionGenerator creates a new generator with dependencies.
func NewMicroHealthInterventionGenerator(stateManager StateManager, timer models.Timer) *MicroHealthInterventionGenerator {
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
	case "", models.StateOrientation:
		slog.Debug("MicroHealthIntervention state orientation", "to", p.To)
		return MsgOrientation, nil
	case models.StateCommitmentPrompt:
		slog.Debug("MicroHealthIntervention state commitment prompt", "to", p.To)
		return MsgCommitment, nil
	case models.StateFeelingPrompt:
		slog.Debug("MicroHealthIntervention state feeling prompt", "to", p.To)
		return MsgFeeling, nil
	case models.StateRandomAssignment:
		slog.Debug("MicroHealthIntervention state random assignment", "to", p.To)
		return MsgRandomAssignment, nil
	case models.StateHabitReminder:
		slog.Debug("MicroHealthIntervention state habit reminder", "to", p.To)
		return MsgHabitReminder, nil
	case models.StateFollowUp:
		slog.Debug("MicroHealthIntervention state follow up", "to", p.To)
		return MsgFollowUp, nil
	case models.StateComplete:
		slog.Debug("MicroHealthIntervention state complete", "to", p.To)
		return MsgComplete, nil
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
