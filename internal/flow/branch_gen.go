// filepath: internal/flow/branch.go
package flow

import (
	"context"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// init ensures any shared branch message utilities are properly initialized
func init() {
	// Currently no global branch structures to initialize,
	// but this provides a place for future shared functionality
}

// BranchGenerator formats branch-type prompts into a selectable list.
type BranchGenerator struct{}

// Generate returns the body combined with branch options and user instructions.
func (b *BranchGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	if len(p.BranchOptions) == 0 {
		return p.Body, nil
	}

	// Create a Branch struct from the prompt data
	branch := models.Branch{
		Body:    p.Body,
		Options: p.BranchOptions,
	}

	// Use the structured branch generation
	return branch.Generate(), nil
}
