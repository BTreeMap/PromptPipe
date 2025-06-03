package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestBranchGenerator_Generate(t *testing.T) {
	b := &BranchGenerator{}
	ctx := context.Background()
	p := models.Prompt{
		Body: "Please choose:",
		BranchOptions: []models.BranchOption{
			{Label: "Opt1", Body: "Choice 1"},
			{Label: "Opt2", Body: "Choice 2"},
		},
	}
	out, err := b.Generate(ctx, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain base body
	if !strings.HasPrefix(out, p.Body) {
		t.Errorf("output should start with base body; got %q", out)
	}
	// Should list options numbered
	if !strings.Contains(out, "1. Opt1: Choice 1") {
		t.Errorf("option 1 not formatted; got %q", out)
	}
	if !strings.Contains(out, "2. Opt2: Choice 2") {
		t.Errorf("option 2 not formatted; got %q", out)
	}
}
