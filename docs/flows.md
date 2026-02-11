# Flow Generators

PromptPipe routes prompts through a generator registry (`internal/flow/flow.go`). Each `PromptType` maps to a generator:

```go
type Generator interface {
    Generate(ctx context.Context, p models.Prompt) (string, error)
}
```

## Built-in generators

### `static`

Uses `Prompt.body` verbatim. A response hook is only auto-registered if the body includes “reply”, “respond”, “answer”, or a `?`.

Validation: `body` must be non-empty and ≤ 4096 characters.

### `genai`

Uses OpenAI to generate content:

- Requires `system_prompt` and `user_prompt`.
- Requires `OPENAI_API_KEY` to be configured.

GenAI prompts automatically register a response hook that acknowledges user replies.

### `branch`

Renders a multi-option prompt with numbered choices, then uses a response hook to interpret numeric replies (`1`, `2`, …).

Validation:

- 2–10 `branch_options`
- `label` ≤ 100 characters
- `body` ≤ 1000 characters

### `conversation`

The conversation generator is stateful and primarily used by the messaging response handler (not by the `/send` API). Direct `/send` calls return a generic “ready to chat” response. See [Conversation Flow](conversation.md).

### `custom`

Reserved for custom generators registered by developers. If no generator is registered for `custom`, `/send` and `/schedule` will return a 500 error when generation fails.

## Registering custom generators

```go
type MyGenerator struct{}

func (g *MyGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
    return "custom message", nil
}

func init() {
    flow.Register(models.PromptTypeCustom, &MyGenerator{})
}
```
