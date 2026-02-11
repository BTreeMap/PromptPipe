# AGENTS.md

Guide for autonomous coding agents working in the PromptPipe repository. Read this before touching the code.

## 1. Project Architecture

PromptPipe is a Go service that delivers personalized micro-habit prompts via WhatsApp, using an AI-driven conversation flow with tool calling.

### Component Map

```
cmd/PromptPipe/main.go          Entry point, flag/env parsing, wiring
internal/api/                    HTTP API endpoints (REST)
internal/flow/                   Conversation state machine, bot modules, tools
internal/genai/                  OpenAI client wrapper (tool calling, thinking)
internal/messaging/              WhatsApp messaging service
internal/models/                 Data models, state types, flow types
internal/store/                  Storage backends (SQLite, Postgres, in-memory)
internal/tone/                   Tone adaptation (whitelist, EMA, validation)
internal/util/                   Env parsing, helpers
internal/lockfile/               Single-instance lock
internal/recovery/               Panic recovery
internal/whatsapp/               WhatsApp client integration
internal/testutil/               Shared test helpers
prompts/                         System prompt text files for each bot module
docs/                            Documentation
```

### Conversation State Machine

The conversation flow dispatches between two primary modules:

- **IntakeModule** (`INTAKE` state): Collects user profile (habit domain, motivation, timing, anchor). Has access to save_user_profile, scheduler, generate_habit_prompt, and transition_state tools.
- **FeedbackModule** (`FEEDBACK` state): Tracks habit completion, barriers, and tweaks. Has access to save_user_profile, scheduler, and transition_state tools.

Modules transition via `StateTransitionTool`. The `CoordinatorModule` exists as a legacy router but is not wired by default in production flow.

Profile data is stored as JSON blobs in the `flow_states` table via `StoreBasedStateManager`.

### Tool Calling Pattern

Each module builds system messages + chat history, then calls the LLM with tool definitions. When the LLM returns tool calls, the module executes them server-side in a loop (max 10 rounds) until a user-facing text response is produced.

## 2. Engineering Workflow

- **Read code first.** Code is the source of truth. Docs may lag behind.
- **Small, reviewable commits** with clear messages (e.g., `feat(tone): ...`, `refactor: ...`, `test: ...`, `docs: ...`).
- **Always run before finishing:**
  ```sh
  go test ./...
  go vet ./...
  gofmt -w <modified files>
  ```
- **Update docs** when behavior or configuration changes.
- **Prefer incremental refactors** with tests over big rewrites.

## 3. Tool and Schema Boundaries

### Profile Write Permissions

| Component | Can write profile? | Notes |
|-----------|-------------------|-------|
| IntakeModule | ✅ Yes | Primary profile builder |
| FeedbackModule | ✅ Yes | Updates feedback fields and tone |
| CoordinatorModule | ❌ No | Must not call save_user_profile |
| PromptGeneratorTool | ❌ No | Read-only access to profile |

### Schema Evolution

- Tool schemas (OpenAI function definitions) should be evolved with backward-compatible, optional fields.
- Validation happens at the Go server boundary, not in the LLM.
- New tool parameters should default to safe values when absent.

### Security

- **No free-form user text** stored as instructions or tone data. Only whitelist tags.
- **Whitelist enforcement** for all tone fields (see `internal/tone/tone.go`).
- **Server-side gating**: LLM proposes, server decides. Even if the LLM sends invalid data, the server strips/rejects it.
- Prompt injection mitigation: tone guide uses structured `<TONE POLICY>` tags, not user-controlled text.

## 4. Configuration

Environment variables and CLI flags are defined in `cmd/PromptPipe/main.go`. See [docs/configuration.md](docs/configuration.md) for the full reference.

**Precedence:** CLI flags > Environment variables > Defaults.

**Key vars:**
- `DATABASE_DSN` / `DATABASE_URL`: SQLite or Postgres connection string.
- `OPENAI_API_KEY`: Required for GenAI features.
- `PROMPTPIPE_STATE_DIR`: Base directory for SQLite files and locks (default: `/var/lib/promptpipe`).
- `API_ADDR`: HTTP listen address (default: `:8080`).

**Databases:** SQLite (default) or PostgreSQL. Migrations are `CREATE TABLE IF NOT EXISTS` in `internal/store/migrations_*.sql`. Profile data is stored as JSON in `flow_states.state_data`.

## 5. Testing Strategy

### Where Tests Live

- Unit tests: `*_test.go` files alongside source in each package.
- Test helpers: `internal/testutil/testutil.go` (HTTP test server, assertions).
- Flow test mocks: `internal/flow/test_helpers.go` (MockStateManager, MockGenAIClient, MockMessagingService).

### How to Run

```sh
go test ./...           # All tests
go test ./internal/flow/ -v -run TestName  # Specific test
```

### What Requires New Tests

- Schema changes to tool definitions or profile structs.
- New validation logic (e.g., tone whitelist, mutual exclusion).
- Store round-trips for new data fields.
- Prompt assembly logic changes.
- State transition changes.

## 6. Code Style and Patterns

### Error Handling

- Return `error` from functions, wrap with `fmt.Errorf("context: %w", err)`.
- Log errors at the call site with `slog.Error(...)`.
- Use `slog.Debug(...)` for tracing, `slog.Info(...)` for significant events.

### Logging

- All logging uses `log/slog` with structured key-value pairs.
- Log function name prefix: `"PackageName.FunctionName: message"`.
- Include `participantID` in flow-related logs.

### Package Boundaries

- `internal/tone`: Self-contained module with no dependencies on `flow` or `store`. Keep it that way.
- `internal/flow`: Contains all conversation logic. May import `tone`, `models`, `genai`, `store`.
- `internal/models`: Shared type definitions. No business logic.
- `internal/store`: Storage only. No flow logic.

### Adding New Features

Follow the `internal/tone` pattern:
1. Create a focused package under `internal/`.
2. Keep coupling minimal (import models, not flow).
3. Add unit tests in the same package.
4. Integration tests go in the consuming package (e.g., `internal/flow/`).
5. Update docs and AGENTS.md if the feature affects architecture or boundaries.
