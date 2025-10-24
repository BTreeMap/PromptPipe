# PromptPipe Onboarding & Conversation Flow Guide

Welcome! This guide gets you productive quickly: environment setup, core architecture, conversation flow (dynamic 3‑bot and new static state-path), coding conventions, and a first contribution checklist.

---

## 1. Prerequisites

Install locally:

- Go >= 1.24.3 (`go version`)
- make
- Git
- (Optional) Docker (for Postgres)
- WhatsApp test number + device to scan QR (for live messaging)

Accounts / Keys:

- OpenAI API key (if using GenAI flows) – export `OPENAI_API_KEY`

---

## 2. Clone & Build

```bash
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe
make build   # builds ./build/promptpipe
```

Run tests:

```bash
go test ./...
```

Binary quick run (no external DB → SQLite in state dir):

```bash
./build/promptpipe -state-dir ./local-state
```

---

## 3. Environment Configuration

Create `.env` (values can be minimal for local dev):

```bash
PROMPTPIPE_STATE_DIR=./local-state
API_ADDR=:8080
OPENAI_API_KEY=sk-xxx            # optional for GenAI
# Optional Postgres
# DATABASE_DSN=postgres://user:pass@localhost:5432/promptpipe?sslmode=disable
```

Load automatically (Makefile / main uses env). Override via flags if needed (`-api-addr`, etc.).

Key directories (dev mental map):

```
internal/api          REST handlers
internal/flow         Conversation engine (state, modules, tools)
internal/messaging    WhatsApp integration & response handling
internal/store        Persistence (SQLite/Postgres/in-memory)
internal/genai        OpenAI abstraction
prompts/              System prompt templates
```

---

## 4. Conversation Flow Overview

PromptPipe supports two coordinator strategies now:

1. LLM Coordinator (`CoordinatorModule`) – dynamic tool-driven reasoning.
2. Static Coordinator (`StaticCoordinatorModule`) – deterministic state diagram.

### 4.1 Core Entities

- ConversationFlow (`conversation_flow.go`): top-level router, loads history and delegates.
- Sub-state (DataKeyConversationState): `COORDINATOR`, `INTAKE`, `FEEDBACK`.
- Tools:
  - `StateTransitionTool` – writes sub-state (immediate or delayed)
  - `ProfileSaveTool` – builds & updates `userProfile`
  - `PromptGeneratorTool` – creates habit prompt (needs complete profile)
  - `SchedulerTool` – schedules daily reminder (currently silent in static coordinator path)
  - `FeedbackModule` – processes user outcome & updates profile statistics

### 4.2 Dynamic (LLM) Path

LLM coordinator chooses when to call tools (profile filling, prompt generation, scheduling) via OpenAI function calling loop. Good for exploratory behavior, but can hallucinate transitions.

### 4.3 Static Deterministic Path

State Machine (simplified):

```
START/ANY → (profile incomplete) → INTAKE → (profile complete) → FEEDBACK (after immediate prompt generation) → back to COORDINATOR or next cycle
```

Static coordinator logic:

- If profile incomplete: transition to `INTAKE`, prompt user for missing fields.
- When profile complete: immediately generate a habit prompt, transition to `FEEDBACK` to watch for success/failure messages.
- Feedback processing increments counters, optionally tweaks profile, then returns control (future refinement: explicit transition back to COORDINATOR after reply).
- Scheduler operations are executed silently (no user-facing text) and may be triggered within Intake/Feedback contexts.

### 4.4 Data Keys (from `models/flow_types.go`)

```
conversationHistory
userProfile
lastHabitPrompt
conversationState (sub-state)

feedbackTimerID / feedbackFollowupTimerID
stateTransitionTimerID
```

### 4.5 Persistence & Recovery

- All state stored via `StateManager` in store backend.
- Timers & schedules have IDs persisted; on restart recovery logic rehydrates timers (see `timer.go`, scheduler & feedback modules).

---

## 5. Making Your First Change

1. Pick a small enhancement (e.g., add a missing log field, refine a static coordinator message).
2. Create a feature branch: `git checkout -b feat/improve-intake-msg`.
3. Run tests before edit: `go test ./...`.
4. Make change.
5. `go fmt` is enforced; run `gofmt -s -w .` or `make fmt` if defined.
6. Run tests again; ensure no regressions.
7. Submit PR referencing issue or improvement rationale.

Definition of Done (small change):

- Tests pass (`go test ./...`).
- No new data races / lints (use `go vet ./...` optionally).
- Clear commit message (imperative mood) & concise PR description.

---

## 6. Adding / Switching Coordinator Implementations

- Interface: `internal/flow/coordinator_interface.go` (`Coordinator`).
- LLM: built automatically in `NewConversationFlowWithAllTools`.
- Static: create via `NewStaticCoordinatorModule(...)` and assign to `ConversationFlow.coordinatorModule` manually or add a selector.

Example (pseudo-code):

```go
flow := NewConversationFlowWithAllTools(...)
flow.SetStaticCoordinator(NewStaticCoordinatorModule(...)) // helper you may add
```

(You can add a helper method if swapping becomes frequent.)

Testing static path quickly: ensure a user without profile triggers intake prompt; then inject a complete `userProfile` state record and send another message to see prompt generation.

---

## 7. Coding Conventions

- Logging: Use `slog` with structured keys (`"participantID"`, not `"participant_id"`).
- Error wrapping: `fmt.Errorf("context: %w", err)`.
- Avoid panics outside `main`; return errors upward.
- Keep tool & module public methods minimal; internal helpers unexported.
- Tests: co-locate `_test.go` with implementation; use table-driven style.

---

## 8. Debug Mode

Debug messages are routed through `SendDebugMessageIfEnabled` and contextual phone number. Enable via flow’s debug mode setter when manually diagnosing tool loops.

---

## 9. Common Troubleshooting

| Symptom | Likely Cause | Fix |
|--------|--------------|-----|
| No reply to inbound message | Missing state initialization | Check `ProcessResponse` logs; ensure `CONVERSATION_ACTIVE` set |
| Repeated intake questions | Profile not persisting | Inspect `userProfile` state key; verify `ProfileSaveTool` calls |
| Scheduler doesn’t fire | Timer not persisted / timezone parse | Check `scheduleRegistry` & timer logs |
| Prompt generation error | Incomplete profile | Fill required fields (habit domain, motivation, preferred time, anchor) |
| Feedback not transitioning | Missing transition_state call | Inspect FeedbackModule logs for tool call results |

---

## 10. Suggested Next Improvements (Good Starter Tasks)

- Add tests for `StaticCoordinatorModule.isProfileComplete` edge cases.
- Implement automatic transition back to COORDINATOR after feedback reply.
- CLI flag to force static vs LLM coordinator.
- Lightweight metrics (counts per sub-state) for observability.
- Add linter config (golangci-lint) & GitHub Actions workflow.

---

## 11. Security & Privacy Notes

- Do not log full user messages at INFO level in production—consider redaction strategy.
- Secrets only via environment variables; never commit keys.
- WhatsApp identifiers treated as PII—mask when exporting logs.

---

## 12. Glossary

| Term | Definition |
|------|------------|
| Participant | End-user conversing over WhatsApp |
| Sub-state | Conversation sub-mode (Coordinator, Intake, Feedback) |
| Tool | Function-call capability exposed to LLM (scheduler, prompt generator, etc.) |
| Timer | Deferred execution primitive for scheduling & delayed transitions |
| Profile | Structured habit context captured during intake |

---

Welcome aboard. Ask for a first issue if unsure—small, fast iterations beat large rewrites.
