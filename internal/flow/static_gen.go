// filepath: internal/flow/static.go
package flow

import (
	"context"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// StaticGenerator returns the prompt body as-is.
type StaticGenerator struct{}

// Generate returns the static body of the prompt.
func (s *StaticGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("StaticGenerator Generate invoked", "type", p.Type, "to", p.To, "body_length", len(p.Body))
	return p.Body, nil
}
