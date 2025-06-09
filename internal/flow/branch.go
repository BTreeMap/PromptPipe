// filepath: internal/flow/branch.go
package flow

import (
	"context"
	"fmt"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Branch formatting constants
const (
	// BranchOptionFormat is the format string for branch option display
	BranchOptionFormat = "\n%d. %s: %s"
)

// BranchGenerator formats branch-type prompts into a selectable list.
type BranchGenerator struct{}

// Generate returns the body combined with branch options.
func (b *BranchGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	sb := p.Body
	for i, opt := range p.BranchOptions {
		sb += fmt.Sprintf(BranchOptionFormat, i+1, opt.Label, opt.Body)
	}
	return sb, nil
}
