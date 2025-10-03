# Conversation Flow

Up‑to‑date description (2025-08) of the production conversation flow architecture and API as implemented in `internal/flow` and `internal/api`.

## Goals

Provide a persistent, tool‑augmented AI conversation with each enrolled participant, supporting structured intake, habit prompt generation & scheduling, and feedback collection. The production flow uses Intake and Feedback modules directly; the prompt generator and scheduler remain available as tools that those modules can call when they need to produce messaging or state changes.

## High‑Level Behavior

1. Client enrolls a participant via `POST /conversation/participants` (phone is unique & canonicalized).
2. Enrollment initializes flow state to `CONVERSATION_ACTIVE` and registers a persistent hook so every inbound message routes through the conversation flow.
3. The server immediately generates and sends the first AI message using `ConversationFlow.ProcessResponse()` with a simulated user hint so the reply is a natural greeting.
4. Each inbound user message is appended to stored history. The flow reads `conversationState` (defaults to `INTAKE`) to determine whether the Intake or Feedback module should handle the turn. The active module may invoke shared tools (scheduler, prompt generator, state transition, profile save) before returning a user-visible reply.
5. History is trimmed (keep last 50) to avoid unbounded growth; up to 30 recent messages are supplied to the LLM for context (adaptive limiting logic in code).

## Key Components (Code References)

| Component | Purpose | File(s) |
|-----------|---------|---------|
| `ConversationFlow` | Orchestrates state retrieval, history persistence, routing to modules | `internal/flow/conversation_flow.go` |
| `CoordinatorModule` | Legacy optional router (not wired by default in production flow) | `internal/flow/coordinator_module.go` |
| `IntakeModule` | Collects structured user profile (habit domain, motivation, preferred time, anchor) | `internal/flow/intake_module.go` |
| `FeedbackModule` | Manages feedback after prompts; schedules follow‑ups | `internal/flow/feedback_module.go` |
| `PromptGeneratorTool` | Generates personalized habit prompt text | `internal/flow/prompt_generator_tool.go` |
| `SchedulerTool` | Schedules daily prompt delivery, mechanical reminders, and auto-feedback timers | `internal/flow/scheduler_tool.go` |
| `StateTransitionTool` | Updates the active module (`INTAKE` ↔ `FEEDBACK`) with optional delays | `internal/flow/state_transition_tool.go` |
| `ProfileSaveTool` | Persists structured profile fields | `internal/flow/profile_save_tool.go` |
| `StoreBasedStateManager` | Persists state & data blobs | `internal/flow/state_manager.go` |
| `MessagingService` | Sends outbound WhatsApp/SMS messages | `internal/messaging/` |
| GenAI client | OpenAI (tool calling) wrapper | `internal/genai/genai.go` |

## Conversation States

State constants (in `internal/models/flow_types.go`):

* `CONVERSATION_ACTIVE` – Top‑level active status (always set once enrolled).
* `INTAKE` – Default sub‑state; collects or repairs profile fields and can trigger scheduling.
* `FEEDBACK` – Feedback tracker gathering adherence & outcomes after prompts are delivered.

The flow keeps two notions:

* Current flow state (`CONVERSATION_ACTIVE`) stored as the persistent state record.
* Current conversation sub‑state stored separately under data key `conversationState` (defaults to `INTAKE` if unset).

> **Note:** The legacy `COORDINATOR` state remains in the repository for backward compatibility but is no longer wired into `NewConversationFlowWithAllTools`. Modules transition directly between `INTAKE` and `FEEDBACK` via the `StateTransitionTool`.

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
| `conversationState` | Current sub‑state (`INTAKE` or `FEEDBACK`; defaults to `INTAKE`) |
| `stateTransitionTimerID` | Pending delayed state transition timer |
| `lastPromptSentAt` | Timestamp of the most recent scheduled habit prompt (RFC3339) |
| `autoFeedbackTimerID` | Timer ID for post-prompt auto-feedback enforcement |
| `dailyPromptPending` | JSON blob tracking the outstanding daily prompt awaiting reply |
| `dailyPromptReminderTimerID` | Timer ID for the mechanical daily prompt reminder |
| `dailyPromptReminderSentAt` | Timestamp when the reminder was actually sent |
| `dailyPromptRespondedAt` | Timestamp when the participant replied after a prompt |

## System Prompts

* `conversation_system_3bot.txt` – Base conversation instructions (legacy coordinator prompt; still loaded for compatibility)
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

1. Append inbound user text to history and notify the scheduler so any pending daily prompt reminder can be cancelled (`SchedulerTool.handleDailyPromptReply`).
1. Determine current conversation sub‑state (`conversationState`, defaults to `INTAKE`).
1. Route to the active module:

* **Intake (`INTAKE`)** – Runs `handleIntakeToolLoop`, which may call `transition_state`, `save_user_profile`, `scheduler`, or `generate_habit_prompt` tool functions until a user-facing reply is produced.
* **Feedback (`FEEDBACK`)** – Runs `handleFeedbackToolLoop`, which may call `transition_state`, `save_user_profile`, or `scheduler` while collecting outcome data.
* Unknown or missing state defaults back to `INTAKE` and is persisted.

1. Append the assistant response to history; trim if >50 and persist the updated conversation record.
1. If debug mode is enabled, send a separate developer-facing debug message summarizing the current state, profile counters, and latest tool actions.

## Tool / Function Calling

* **IntakeModule** exposes `transition_state`, `save_user_profile`, `scheduler`, and `generate_habit_prompt` tool definitions to the LLM. Calls are executed inside `handleIntakeToolLoop`, which logs the tool name and a truncated JSON argument preview before dispatching.
* **FeedbackModule** exposes `transition_state`, `save_user_profile`, and `scheduler` through `handleFeedbackToolLoop`, following the same logging and execution pattern.
* **SchedulerTool** may in turn invoke the prompt generator when delivering scheduled prompts and manages the daily reminder / auto-feedback timers.
* **Legacy CoordinatorModule** can still be wired manually for experimentation; if used, it shares the same tool set as Intake plus any custom additions.

## History & Context Limits

* Persisted history trimmed to the last 50 messages.
* Intake and Feedback modules both respect `CHAT_HISTORY_LIMIT` when constructing context for the LLM (default intake window is 30 user/assistant turns).
* `SetChatHistoryLimit()` can override per‑tool history exposure (0 disables, -1 unlimited, >0 cap).

## Daily Prompt Reminder Workflow

1. When a scheduled habit prompt is delivered, `SchedulerTool.executeScheduledPrompt` records `lastPromptSentAt`, stores a `dailyPromptPending` payload, and schedules a reminder timer using `dailyPromptReminderDelay` (default 5 hours).
1. If the participant replies before the reminder fires, `ConversationFlow.processConversationMessage` calls `SchedulerTool.handleDailyPromptReply`, which cancels the timer, clears pending state, and records `dailyPromptRespondedAt`.
1. If the timer fires first, `SchedulerTool.sendDailyPromptReminder` sends a mechanical follow-up message, records `dailyPromptReminderSentAt`, and clears pending state to prevent duplicates.
1. Reminder delays can be overridden per-instance via `SchedulerTool.SetDailyPromptReminderDelay`; passing a non-positive duration disables the follow-up entirely.

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
4. Update `processConversationMessage` routing and, if tool callable, ensure the new tool definition is exposed by the relevant module.
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
