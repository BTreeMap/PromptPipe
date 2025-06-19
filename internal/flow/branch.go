// filepath: internal/flow/branch.go
package flow

import (
	"context"
	"fmt"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// BranchGenerator formats branch-type prompts into a selectable list.
type BranchGenerator struct{}

// Generate returns the body combined with branch options and user instructions.
func (b *BranchGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	if len(p.BranchOptions) == 0 {
		return p.Body, nil
	}

	sb := p.Body

	// Add a separator line if body doesn't end with newline
	if len(sb) > 0 && sb[len(sb)-1] != '\n' {
		sb += "\n"
	}

	// Add options with better formatting
	sb += "\n"
	for i, opt := range p.BranchOptions {
		sb += fmt.Sprintf("%d. %s: %s\n", i+1, opt.Label, opt.Body)
	}

	// Add user instruction with proper grammar
	sb += "\n"
	if len(p.BranchOptions) == 2 {
		sb += "(Reply with '1' or '2')"
	} else {
		// Generate the range dynamically for multiple options
		sb += "(Reply with '1'"
		for i := 2; i < len(p.BranchOptions); i++ {
			sb += fmt.Sprintf(", '%d'", i)
		}
		sb += fmt.Sprintf(", or '%d')", len(p.BranchOptions))
	}

	return sb, nil
}
