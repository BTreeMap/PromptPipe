// filepath: internal/flow/flow.go
package flow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Error message constants
const (
	ErrMsgNoGeneratorRegistered = "no generator registered for prompt type %s"
)

// Generator defines how to create a message body from a Prompt.
type Generator interface {
	Generate(ctx context.Context, p models.Prompt) (string, error)
}

// Registry manages Generator implementations for different prompt types.
type Registry struct {
	generators map[models.PromptType]Generator
}

// NewRegistry creates a new Generator registry.
func NewRegistry() *Registry {
	return &Registry{
		generators: make(map[models.PromptType]Generator),
	}
}

// Register associates a PromptType with a Generator implementation.
func (r *Registry) Register(pt models.PromptType, gen Generator) {
	r.generators[pt] = gen
}

// Get retrieves the Generator for a given PromptType.
func (r *Registry) Get(pt models.PromptType) (Generator, bool) {
	gen, ok := r.generators[pt]
	return gen, ok
}

// Generate finds and runs the Generator for the prompt's type.
func (r *Registry) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("Flow Generate invoked", "type", p.Type, "to", p.To)
	if gen, ok := r.Get(p.Type); ok {
		result, err := gen.Generate(ctx, p)
		if err != nil {
			slog.Error("Flow generator error", "type", p.Type, "to", p.To, "error", err)
		} else {
			slog.Debug("Flow Generate succeeded", "type", p.Type, "to", p.To)
		}
		return result, err
	}
	slog.Error("No generator registered for prompt type", "type", p.Type, "to", p.To)
	return "", fmt.Errorf(ErrMsgNoGeneratorRegistered, p.Type)
}

// Default registry instance for backward compatibility
var defaultRegistry = NewRegistry()

// Register associates a PromptType with a Generator implementation in the default registry.
func Register(pt models.PromptType, gen Generator) {
	defaultRegistry.Register(pt, gen)
}

// Get retrieves the Generator for a given PromptType from the default registry.
func Get(pt models.PromptType) (Generator, bool) {
	return defaultRegistry.Get(pt)
}

// Generate finds and runs the Generator for the prompt's type using the default registry.
func Generate(ctx context.Context, p models.Prompt) (string, error) {
	return defaultRegistry.Generate(ctx, p)
}

// Register default generators
func init() {
	Register(models.PromptTypeStatic, &StaticGenerator{})
	Register(models.PromptTypeBranch, &BranchGenerator{})
}
