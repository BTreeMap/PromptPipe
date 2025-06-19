package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestMicroHealthInterventionGenerator_Generate(t *testing.T) {
	g := &MicroHealthInterventionGenerator{}
	ctx := context.Background()

	// Orientation state
	out, err := g.Generate(ctx, models.Prompt{State: ""})
	if err != nil {
		t.Fatalf("orientation: unexpected error: %v", err)
	}
	if !strings.Contains(out, "Welcome") {
		t.Errorf("orientation: expected welcome message, got %q", out)
	}

	// Commitment prompt state
	out, err = g.Generate(ctx, models.Prompt{State: models.StateCommitmentPrompt})
	if err != nil {
		t.Fatalf("commitment: unexpected error: %v", err)
	}
	if !strings.Contains(out, "ready to go") {
		t.Errorf("commitment: expected commitment prompt, got %q", out)
	}

	// Feeling prompt state
	out, err = g.Generate(ctx, models.Prompt{State: models.StateFeelingPrompt})
	if err != nil {
		t.Fatalf("feeling: unexpected error: %v", err)
	}
	if !strings.Contains(out, "How do you feel") {
		t.Errorf("feeling: expected feeling prompt, got %q", out)
	}

	// Unknown state
	_, err = g.Generate(ctx, models.Prompt{State: "UNKNOWN"})
	if err == nil {
		t.Error("expected error for unknown state, got nil")
	}
}
