# Conversation Flow

PromptPipe includes a stateful conversation flow with intake and feedback modules. Participants are enrolled via the API or automatically (when `AUTO_ENROLL_NEW_USERS=true`) and all incoming messages for enrolled participants are routed through the flow.

> **Requirement:** The conversation flow requires an OpenAI client (`OPENAI_API_KEY`). Without it, conversation responses cannot be generated.

For a deep dive, see [Conversation Flow (detailed)](conversation-flow.md).

## States

Conversation flow stores two related states:

- **Flow state**: `CONVERSATION_ACTIVE` (stored in `flow_states.current_state`)
- **Conversation sub-state**: `INTAKE` or `FEEDBACK` (stored in `flow_states.state_data["conversationState"]`)

If no sub-state is present, the flow defaults to `INTAKE`.

## Modules and tools

The active flow uses the Intake and Feedback modules directly. The Coordinator module is present in the repository but **not wired** into `NewConversationFlowWithAllTools`.

Shared tools:

- `scheduler` (daily habit scheduling, preparation reminders, auto-feedback timers)
- `generate_habit_prompt`
- `save_user_profile`
- `transition_state`

## Enrollment behavior

`POST /conversation/participants`:

- Creates a participant
- Initializes `CONVERSATION_ACTIVE`
- Registers a persistent response hook
- Sends the first AI reply by simulating a greeting

Auto-enrollment (if enabled) performs the same setup when an unknown phone number sends a message.

## Scheduler behavior (conversation tool)

The LLM-facing scheduler tool creates daily reminders:

- Fixed schedules default to timezone `America/Toronto` when none is supplied.
- Random schedules default to `UTC`.
- A preparation message is scheduled **SCHEDULER_PREP_TIME_MINUTES** before the target habit time.
- Auto-feedback enforcement (when enabled) schedules a 5-minute timer that switches the user to `FEEDBACK` if no feedback arrives. This transition does **not** send a message because the phone number is not persisted in state yet.

## Daily reminders and intensity

After a scheduled prompt is sent:

- A follow-up reminder is scheduled (default delay: 5 hours, not configurable via env).
- An intensity adjustment poll (“How’s the intensity?”) is sent at most once per day.
- Poll responses adjust the `UserProfile.Intensity` field.

## Debug mode

When `PROMPTPIPE_DEBUG=true` or `--debug` is set:

- The conversation flow sends extra messages prefixed with `🐛 DEBUG:` to enrolled participants.
- GenAI calls are logged to `{STATE_DIR}/debug/` (see [Debug Mode](debug-mode.md)).
