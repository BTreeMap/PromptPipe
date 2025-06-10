package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// State constants for micro health intervention flow.
const (
	StateOrientation      = "ORIENTATION"
	StateCommitmentPrompt = "COMMITMENT_PROMPT"
	StateFeelingPrompt    = "FEELING_PROMPT"
	StateRandomAssignment = "RANDOM_ASSIGNMENT"
	StateHabitReminder    = "HABIT_REMINDER"
	StateFollowUp         = "FOLLOW_UP"
	StateComplete         = "COMPLETE"
)

// Message templates for micro health intervention flow.
const (
	MsgOrientation      = "Hi! 🌱 Welcome to our Healthy Habits study!\nHere's how it works: You will receive messages on a schedule, or type 'Ready' anytime to get a prompt. Your input is important."
	MsgCommitment       = "You committed to a quick habit today—ready to go?\n1. 🚀 Let's do it!\n2. ⏳ Not yet\n(Reply with '1' or '2')"
	MsgFeeling          = "How do you feel about this first step?\n1. 😊 Excited\n2. 🤔 Curious\n3. 😃 Motivated\n4. 📖 Need info\n5. ⚖️ Not sure\n(Reply with '1'–'5')"
	MsgRandomAssignment = "Based on your profile, we're assigning you to a personalized track. Please wait for your next message."
	MsgHabitReminder    = "⏰ Reminder: It's time for your healthy habit! How did it go?\n1. ✅ Completed\n2. ⏳ Will do later\n3. ❌ Skipped today\n(Reply with '1', '2', or '3')"
	MsgFollowUp         = "Great progress! 📈 How are you feeling about your habit journey?\n1. 😊 Going well\n2. 🤔 Mixed feelings\n3. 😓 Struggling\n(Reply with '1', '2', or '3')"
	MsgComplete         = "🎉 Congratulations! You've completed the micro health intervention. Thank you for participating!"
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
		// Orientation: send welcome message
		return MsgOrientation, nil
	case StateCommitmentPrompt:
		slog.Debug("MicroHealthIntervention state commitment prompt", "to", p.To)
		// Commitment poll
		return MsgCommitment, nil
	case StateFeelingPrompt:
		slog.Debug("MicroHealthIntervention state feeling prompt", "to", p.To)
		// Feeling poll
		return MsgFeeling, nil
	case StateRandomAssignment:
		slog.Debug("MicroHealthIntervention state random assignment", "to", p.To)
		// Random assignment message
		return MsgRandomAssignment, nil
	case StateHabitReminder:
		slog.Debug("MicroHealthIntervention state habit reminder", "to", p.To)
		// Habit reminder message
		return MsgHabitReminder, nil
	case StateFollowUp:
		slog.Debug("MicroHealthIntervention state follow up", "to", p.To)
		// Follow up message
		return MsgFollowUp, nil
	case StateComplete:
		slog.Debug("MicroHealthIntervention state complete", "to", p.To)
		// Completion message
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
