# Conversation Flow

Up‑to‑date description (2025-08) of the production conversation flow architecture and API as implemented in `internal/flow` and `internal/api`.

## Goals

Provide a persistent, tool‑augmented AI conversation with each enrolled participant, supporting: free chat, structured intake, habit prompt generation & scheduling, and feedback collection with stateful routing (the "3‑bot" architecture: Intake Bot, Prompt Generator, Feedback Tracker) coordinated by a central Coordinator.

## High‑Level Behavior

1. Client enrolls a participant via `POST /conversation/participants` (phone is unique & canonicalized).
2. Enrollment initializes flow state to `CONVERSATION_ACTIVE` and registers a persistent hook so every inbound message routes through the conversation flow.
3. The server immediately generates and sends the first AI message using `ConversationFlow.ProcessResponse()` with a simulated user hint so the reply is a natural greeting.
4. Each inbound user message is processed, appended to stored history, routed according to the current conversation sub‑state (Coordinator / Intake / Feedback), tools may be invoked, then the assistant response is stored and returned/sent.
5. History is trimmed (keep last 50) to avoid unbounded growth; up to 30 recent messages are supplied to the LLM for context (adaptive limiting logic in code).

## Key Components (Code References)

| Component | Purpose | File(s) |
|-----------|---------|---------|
| `ConversationFlow` | Orchestrates state retrieval, history persistence, routing to modules | `internal/flow/conversation_flow.go` |
| `CoordinatorModule` | Default router; decides tool usage & state transitions | `internal/flow/coordinator_module.go` |
| `IntakeModule` | Collects structured user profile (habit domain, motivation, preferred time, anchor) | `internal/flow/intake_module.go` |
| `FeedbackModule` | Manages feedback after prompts; schedules follow‑ups | `internal/flow/feedback_module.go` |
| `PromptGeneratorTool` | Generates personalized habit prompt text | `internal/flow/prompt_generator_tool.go` |
| `SchedulerTool` | Schedules daily prompt delivery & manages schedule registry | `internal/flow/scheduler_tool.go` |
| `StateTransitionTool` | LLM‑invokable function to change conversation sub‑state | `internal/flow/state_transition_tool.go` |
| `ProfileSaveTool` | Persists structured profile fields | `internal/flow/profile_save_tool.go` |
| `StoreBasedStateManager` | Persists state & data blobs | `internal/flow/state_manager.go` |
| `MessagingService` | Sends outbound WhatsApp/SMS messages | `internal/messaging/` |
| GenAI client | OpenAI (tool calling) wrapper | `internal/genai/genai.go` |

## Conversation States

State constants (in `internal/models/flow_types.go`):

* `CONVERSATION_ACTIVE` – Top‑level active status (always set once enrolled).
* `COORDINATOR` – Default sub‑state; routes & invokes tools.
* `INTAKE` – Intake bot collecting/repairing missing profile fields.
* `FEEDBACK` – Feedback tracker gathering adherence & outcomes.

The flow keeps two notions:

* Current flow state (`CONVERSATION_ACTIVE`) stored as current state.
* Current conversation sub‑state stored separately under data key `conversationState` (defaults to `COORDINATOR`).

## Data Keys (models.DataKey*)

| Key | Meaning |
|-----|---------|
| `conversationHistory` | JSON array of messages `{role, content, timestamp}` (trimmed to 50) |
| `systemPrompt` | Loaded base system prompt (if stored) |
| `participantBackground` | Preformatted background from enrollment (Name/Gender/Ethnicity/Background lines) |
| `userProfile` | Structured profile JSON (habit domain, motivation, preferred time, anchor, counters) |
| `lastHabitPrompt` | Last generated habit prompt text |
| `feedbackState` | Internal tracker for feedback collection progress |
| `feedbackTimerID`, `feedbackFollowupTimerID` | Timer IDs for scheduled feedback follow‑ups |
| `scheduleRegistry` | Active schedule metadata (prompt delivery scheduling) |
| `conversationState` | Current sub‑state: `COORDINATOR` / `INTAKE` / `FEEDBACK` |
| `stateTransitionTimerID` | Pending delayed state transition timer |

## System Prompts

* `conversation_system_3bot.txt` – Coordinator system prompt (passed to CoordinatorModule + background + profile status)
* `intake_bot_system.txt` – Intake bot instructions
* `prompt_generator_system.txt` – Habit prompt generator
* `feedback_tracker_system.txt` – Feedback tracker

Loading is lazy; default fallback strings are used if files are missing. Background and dynamic profile status strings are appended as additional system messages (not concatenated into a single file) during message construction.

## Enrollment API

Endpoint: `POST /conversation/participants`

Request body fields (`ConversationEnrollmentRequest`):

```jsonc
{
  "phone_number": "+1234567890",            // REQUIRED, validated & canonicalized
  "name": "Alice Smith",                    // optional
  "gender": "female",                       // optional
  "ethnicity": "Hispanic",                  // optional
  "background": "College student...",       // optional
  "timezone": "America/New_York"            // optional IANA TZ
}
```

Important behaviors:

* Rejects duplicate phone numbers (HTTP 409).
* Persists participant; initializes current state to `CONVERSATION_ACTIVE`.
* Stores formatted `participantBackground` if any contextual fields provided.
* Registers persistent hook so inbound messages route automatically.
* Generates & sends first assistant message via flow (simulated user hint).

Response (201):

```json
{
  "status": "ok",
  "message": "Conversation participant enrolled successfully",
  "result": {
    "id": "conv_xxxxx",
    "phone_number": "+1234567890",
    "name": "Alice Smith",
    "gender": "female",
    "ethnicity": "Hispanic",
    "background": "College student...",
    "timezone": "America/New_York",
    "status": "active",
    "enrolled_at": "2025-08-24T12:34:56Z",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

Other endpoints:

* `GET  /conversation/participants` – List
* `GET  /conversation/participants/{id}` – Retrieve
* `PUT  /conversation/participants/{id}` – Partial update (name/gender/ethnicity/background/timezone/status)
* `DELETE /conversation/participants/{id}` – Unregister (sends notification, removes hook, clears state)

## Message Processing Flow

1. Append inbound user text to history.
2. Determine current conversation sub‑state (`conversationState`, default `COORDINATOR`).
3. Route to appropriate module:
   * Coordinator → builds messages (system prompt + background + profile status + trimmed history), may call tools (state_transition, profile_save, generate_habit_prompt, schedule_prompt, etc.).
   * Intake → runs structured question logic; uses previous chat history (configurable limit) and updates profile via tools.
   * Feedback → analyzes user reply, updates feedback state, may cancel timers.
4. Append assistant response to history; trim if >50.
5. (Optional) If debug mode enabled, send a separate debug info message (state, profile summary, tool actions).

## Tool / Function Calling

The Coordinator exposes a set of tools to the LLM (only those initialized):

* `transition_state` (StateTransitionTool) – switch between COORDINATOR/INTAKE/FEEDBACK.
* `save_user_profile` (ProfileSaveTool) – persist structured fields.
* `generate_habit_prompt` (PromptGeneratorTool) – craft habit prompt.
* `schedule_prompt` (SchedulerTool) – schedule recurring delivery referencing generated prompts.

Intake & Feedback modules call their logic directly (not via tool calls from coordinator) while still receiving limited conversation context.

## History & Context Limits

* Persisted history trimmed to last 50 messages.
* When constructing OpenAI context for Coordinator: keep up to last 30 messages (user/assistant only) plus system & background messages.
* `SetChatHistoryLimit()` can override per‑tool history exposure (0 disables, -1 unlimited, >0 cap).

## Profile Status Injection

Dynamic system message describes profile completeness so the AI knows whether to transition to Intake or proceed with prompt generation:

* Missing fields → instructs AI to `transition_state` to `INTAKE` (and NOT manually ask intake questions outside the module).
* Complete profile → indicates prompt generation is allowed.

## Debug Mode

`ConversationFlow.SetDebugMode(true)` enables user‑visible messages prefixed with "🐛 DEBUG:" containing:

* Current sub‑state
* Profile summary and success counters
* Tool execution summaries (select points)

Phone number must be in context for debug delivery (`GetPhoneNumberFromContext`).

## Error Handling & Fallbacks

* Missing prompt files → default system prompt strings.
* Failure to save history does not block response delivery (logged as warning/error).
* Enrollment side effects (state init, hook registration, first message send) are best‑effort; enrollment still returns 201 unless participant save itself fails.

## Extensibility Notes

To add a new specialized module:

1. Implement `<NewModule>` with `LoadSystemPrompt` and `Process...` / `Execute...` functions.
2. Inject in `NewConversationFlowWithAllTools...` similar to other modules.
3. Add new state constant & data keys (if needed) in `internal/models/flow_types.go`.
4. Update coordinator tool set (if tool callable) or routing logic in `processConversationMessage`.
5. Document prompt file in `prompts/` directory.

## Operational Guidelines

* Keep prompt files concise & version them via changesets.
* Use debug mode sparingly for participant accounts (opt‑in) to avoid noise.
* Monitor trimming: if specialized analytics need older context, export `conversationHistory` before trimming policy changes.

## Future Enhancements (Backlog Ideas)

* Endpoint to fetch a participant's trimmed conversation history (`/conversation/participants/{id}/history`).
* Admin action to force state transition (manual override) without LLM tool call.
* Metrics surface for tool usage frequencies & state transition patterns.
* Multi‑language expansion (system prompts + localization of intake questions).

---

This document reflects the current implementation; verify against code if introducing structural changes.
