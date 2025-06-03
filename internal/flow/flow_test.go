package flow

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestRegisterAndGet(t *testing.T) {
	// Register a dummy generator
	var dummy Generator = &StaticGenerator{}
	Register("DUMMY", dummy)

	gen, ok := Get("DUMMY")
	if !ok {
		t.Errorf("expected generator for DUMMY, got none")
	}
	if gen != dummy {
		t.Errorf("expected returned generator to match registered one")
	}
}

func TestGenerateUnregistered(t *testing.T) {
	p := models.Prompt{Type: "UNKNOWN"}
	_, err := Generate(context.Background(), p)
	if err == nil {
		t.Errorf("expected error for unregistered prompt type, got nil")
	}
}

func TestGenerateStaticAndBranch(t *testing.T) {
	ctx := context.Background()
	// Static
	p1 := models.Prompt{Type: models.PromptTypeStatic, Body: "hello"}
	out, err := Generate(ctx, p1)
	if err != nil {
		t.Fatalf("static generate error: %v", err)
	}
	if out != "hello" {
		t.Errorf("static: expected 'hello', got %q", out)
	}

	// Branch
	p2 := models.Prompt{
		Type:          models.PromptTypeBranch,
		Body:          "choose:",
		BranchOptions: []models.BranchOption{{Label: "A", Body: "opt1"}, {Label: "B", Body: "opt2"}},
	}
	out2, err := Generate(ctx, p2)
	if err != nil {
		t.Fatalf("branch generate error: %v", err)
	}
	if out2 == p2.Body {
		t.Errorf("branch: expected extended body, got only base")
	}
	if len(out2) <= len(p2.Body) {
		t.Errorf("branch: output too short: %q", out2)
	}
}
