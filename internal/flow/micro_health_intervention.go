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
	// TODO: add other states as needed
)

// Message templates for micro health intervention flow.
const (
	MsgOrientation = "Hi! 🌱 Welcome to our Healthy Habits study!\nHere's how it works: You will receive messages on a schedule, or type 'Ready' anytime to get a prompt. Your input is important."
	MsgCommitment  = "You committed to a quick habit today—ready to go?\n1. 🚀 Let's do it!\n2. ⏳ Not yet\n(Reply with '1' or '2')"
	MsgFeeling     = "How do you feel about this first step?\n1. 😊 Excited\n2. 🤔 Curious\n3. 😃 Motivated\n4. 📖 Need info\n5. ⚖️ Not sure\n(Reply with '1'–'5')"
)

// Error message constants
const (
	ErrMsgUnsupportedState = "unsupported micro health intervention state '%s'"
)

// MicroHealthInterventionGenerator implements a custom, stateful micro health intervention flow.
type MicroHealthInterventionGenerator struct {
	// TODO: inject dependencies (e.g., state store, timers)
}

// Generate selects the next message based on the current state in p.State.
func (g *MicroHealthInterventionGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("MicroHealthIntervention Generate invoked", "state", p.State, "to", p.To)
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
	default:
		slog.Error("MicroHealthIntervention unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf(ErrMsgUnsupportedState, p.State)
	}
}

func init() {
	Register(models.PromptTypeCustom, &MicroHealthInterventionGenerator{})
}
