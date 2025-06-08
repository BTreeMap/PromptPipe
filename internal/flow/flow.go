// filepath: internal/flow/flow.go
package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Generator defines how to create a message body from a Prompt.
type Generator interface {
	Generate(ctx context.Context, p models.Prompt) (string, error)
}

var registry = make(map[models.PromptType]Generator)

// Register associates a PromptType with a Generator implementation.
func Register(pt models.PromptType, gen Generator) {
	registry[pt] = gen
}

// Get retrieves the Generator for a given PromptType.
func Get(pt models.PromptType) (Generator, bool) {
	gen, ok := registry[pt]
	return gen, ok
}

// Generate finds and runs the Generator for the prompt's type.
func Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("Flow Generate invoked", "type", p.Type, "to", p.To)
	if gen, ok := Get(p.Type); ok {
		result, err := gen.Generate(ctx, p)
		if err != nil {
			slog.Error("Flow generator error", "type", p.Type, "to", p.To, "error", err)
		} else {
			slog.Debug("Flow Generate succeeded", "type", p.Type, "to", p.To)
		}
		return result, err
	}
	slog.Error("No generator registered for prompt type", "type", p.Type, "to", p.To)
	return "", fmt.Errorf("no generator registered for prompt type %s", p.Type)
}

// Register default generators
func init() {
	Register(models.PromptTypeStatic, &StaticGenerator{})
	Register(models.PromptTypeBranch, &BranchGenerator{})
}
