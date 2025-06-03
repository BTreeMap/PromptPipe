// filepath: internal/flow/flow.go
package flow

import (
	"context"
	"fmt"

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
	if gen, ok := Get(p.Type); ok {
		return gen.Generate(ctx, p)
	}
	return "", fmt.Errorf("no generator registered for prompt type %s", p.Type)
}

// Register default generators
func init() {
	Register(models.PromptTypeStatic, &StaticGenerator{})
	Register(models.PromptTypeBranch, &BranchGenerator{})
}
