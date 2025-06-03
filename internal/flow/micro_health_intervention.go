package flow

import (
	"context"
	"fmt"

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

// MicroHealthInterventionGenerator implements a custom, stateful micro health intervention flow.
type MicroHealthInterventionGenerator struct {
	// TODO: inject dependencies (e.g., state store, timers)
}

// Generate selects the next message based on the current state in p.State.
func (g *MicroHealthInterventionGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	switch p.State {
	case "", StateOrientation:
		// Orientation: send welcome message
		return "Hi! 🌱 Welcome to our Healthy Habits study!\nHere’s how it works: You will receive messages on a schedule, or type ‘Ready’ anytime to get a prompt. Your input is important.", nil
	case StateCommitmentPrompt:
		// Commitment poll
		return "You committed to a quick habit today—ready to go?\n1. 🚀 Let’s do it!\n2. ⏳ Not yet\n(Reply with ‘1’ or ‘2’)", nil
	case StateFeelingPrompt:
		// Feeling poll
		return "How do you feel about this first step?\n1. 😊 Excited\n2. 🤔 Curious\n3. 😃 Motivated\n4. 📖 Need info\n5. ⚖️ Not sure\n(Reply with ‘1’–‘5’)", nil
	default:
		return "", fmt.Errorf("unsupported micro health intervention state '%s'", p.State)
	}
}

func init() {
	Register(models.PromptTypeCustom, &MicroHealthInterventionGenerator{})
}
