// filepath: internal/flow/genai.go
package flow

import (
    "context"

    "github.com/BTreeMap/PromptPipe/internal/genai"
    "github.com/BTreeMap/PromptPipe/internal/models"
)

// GenAIGenerator uses a GenAI client to generate prompt bodies.
type GenAIGenerator struct {
    Client *genai.Client
}

// Generate generates the prompt body using GenAI.
func (g *GenAIGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
    return g.Client.GeneratePrompt(p.SystemPrompt, p.UserPrompt)
}
