# Persistence Audit – State Ledger

This document inventories all state that influences user-visible behavior and
must survive a `docker compose down && docker compose up -d` cycle.

## State Items

| # | State Item | Owner (package) | Must Survive Restart? | Current Storage | Proposed Durable Representation | Idempotency / Dedup Strategy |
|---|---|---|---|---|---|---|
| 1 | **In-memory timers** (`SimpleTimer.timers` map) | `flow/timer.go` | Yes | In-memory map of `*timerEntry` keyed by timer ID | `jobs` table (`kind` + `run_at` + `payload_json`) | `dedupe_key` unique constraint; on restart, requeue stale running jobs |
| 2 | **Recurring schedule timers** (daily prompt schedule) | `flow/timer.go` | Yes | In-memory via `time.AfterFunc` with self-rescheduling | `jobs` table with `kind=recurring_prompt`; after execution, enqueue next occurrence | `dedupe_key` per participant + schedule; only one active job per schedule |
| 3 | **Daily prompt pending state** (`dailyPromptPendingState`) | `flow/scheduler_tool.go` | Yes | Persisted in `flow_states.state_data` as JSON (key `daily_prompt_pending`) | Keep in `flow_states`; reminder becomes a `jobs` row instead of `time.AfterFunc` | Check pending state before sending reminder; clear after send |
| 4 | **Pending state transition timer** (delayed `transition_state`) | `flow/state_transition_tool.go` | Yes | Timer ID stored in `flow_states.state_data`, actual timer in memory | `jobs` table with `kind=state_transition` | `dedupe_key` per participant + flow type; cancel = mark job canceled |
| 5 | **Feedback follow-up timer** | `flow/feedback_module.go` | Yes | In-memory timer via `SimpleTimer` | `jobs` table with `kind=feedback_followup` | `dedupe_key` per participant; skip if participant state changed |
| 6 | **Feedback initial timeout timer** | `flow/feedback_module.go` | Yes | In-memory timer via `SimpleTimer` | `jobs` table with `kind=feedback_timeout` | `dedupe_key` per participant; skip if already responded |
| 7 | **LID-by-phone cache** (`lidByPhone` map) | `messaging/whatsapp_service.go` | No | In-memory map | Remains in-memory; repopulated on first message from each contact | N/A – cache miss just means first send uses phone number directly |
| 8 | **Outgoing WhatsApp messages** (sent via `SendMessage`) | `messaging/whatsapp_service.go` | Yes | Fire-and-forget; receipt stored in `receipts` table | `outbox_messages` table; flow enqueues, sender worker delivers | `dedupe_key` prevents duplicate sends on restart |
| 9 | **Inbound WhatsApp messages** (received via event handler) | `messaging/whatsapp_service.go` | Yes (dedup) | Processed immediately; response stored in `responses` table | `inbound_dedup` table keyed by WhatsApp message ID | Insert-or-skip on message ID; prevents reprocessing after restart |
| 10 | **Flow state** (`flow_states` table) | `store/`, `flow/state_manager.go` | Yes | SQLite/Postgres `flow_states` table | Already persisted | N/A – already durable |
| 11 | **Conversation participants** | `store/` | Yes | SQLite/Postgres `conversation_participants` table | Already persisted | N/A – already durable |
| 12 | **Registered hooks** | `store/` | Yes | SQLite/Postgres `registered_hooks` table | Already persisted | N/A – already durable |
| 13 | **Receipts / Responses** | `store/` | Yes | SQLite/Postgres tables | Already persisted | N/A – already durable |

## New Tables

### `jobs`

Replaces all in-memory `time.AfterFunc` timers with durable, restart-safe job records.

| Column | Type | Description |
|---|---|---|
| `id` | TEXT (UUID) | Primary key |
| `kind` | TEXT | Job type (e.g., `recurring_prompt`, `state_transition`, `feedback_followup`, `daily_prompt_reminder`) |
| `run_at` | TIMESTAMP | When the job should execute |
| `payload_json` | TEXT/JSON | Job-specific parameters |
| `status` | TEXT | `queued`, `running`, `done`, `failed`, `canceled` |
| `attempt` | INTEGER | Current attempt number |
| `max_attempts` | INTEGER | Maximum retry attempts |
| `last_error` | TEXT | Last error message (nullable) |
| `locked_at` | TIMESTAMP | When a worker claimed this job (nullable) |
| `dedupe_key` | TEXT | Unique constraint for preventing duplicates (nullable) |
| `created_at` | TIMESTAMP | Row creation time |
| `updated_at` | TIMESTAMP | Last update time |

### `outbox_messages`

Ensures outgoing WhatsApp sends are restart-safe and idempotent.

| Column | Type | Description |
|---|---|---|
| `id` | TEXT (UUID) | Primary key |
| `participant_id` | TEXT | Target participant |
| `kind` | TEXT | Message type (e.g., `prompt`, `reminder`, `feedback_followup`) |
| `payload_json` | TEXT/JSON | Message content and metadata |
| `status` | TEXT | `queued`, `sending`, `sent`, `failed` |
| `attempts` | INTEGER | Send attempt count |
| `next_attempt_at` | TIMESTAMP | When to retry (nullable) |
| `dedupe_key` | TEXT | Unique constraint for preventing duplicate sends |
| `locked_at` | TIMESTAMP | When a worker claimed this message (nullable) |
| `last_error` | TEXT | Last error message (nullable) |
| `created_at` | TIMESTAMP | Row creation time |
| `updated_at` | TIMESTAMP | Last update time |

### `inbound_dedup`

Prevents reprocessing of inbound messages after restart.

| Column | Type | Description |
|---|---|---|
| `message_id` | TEXT | Primary key – stable WhatsApp message ID |
| `participant_id` | TEXT | Sender identifier |
| `received_at` | TIMESTAMP | When the message was first seen |
| `processed_at` | TIMESTAMP | When processing completed (nullable) |

## Invariants

1. Any state transition that implies future work must, in ONE DB transaction:
   - Update `flow_states`
   - Enqueue `jobs` and/or `outbox_messages`
2. Timers are never used for durable behavior – they become jobs.
3. `dedupe_key` constraints ensure restarts and retries do not duplicate work.
4. On startup: requeue stale `running` jobs/outbox (locked_at older than threshold).
