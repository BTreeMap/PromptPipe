package flow

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestStaticGenerator_Generate(t *testing.T) {
	s := &StaticGenerator{}
	ctx := context.Background()
	p := models.Prompt{Body: "static content"}
	out, err := s.Generate(ctx, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != p.Body {
		t.Errorf("expected %q, got %q", p.Body, out)
	}
}
