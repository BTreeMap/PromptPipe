# Storage and Databases

PromptPipe uses **two** databases:

1. **WhatsApp/whatsmeow database** (managed by whatsmeow)
2. **Application database** (managed by PromptPipe)

## WhatsApp database (whatsmeow)

Configured via `WHATSAPP_DB_DSN` or `--whatsapp-db-dsn`.

Default:

```
file:{STATE_DIR}/whatsmeow.db?_foreign_keys=on
```

The WhatsApp client auto-detects SQLite vs PostgreSQL from the DSN. For SQLite, it logs a warning if foreign keys are not enabled.

## Application database

Configured via `DATABASE_DSN` or `--app-db-dsn`. If unset, `DATABASE_URL` is used as a legacy fallback.

Default:

```
file:{STATE_DIR}/state.db?_foreign_keys=on
```

The store auto-detects DSN type:

- `postgres://...` or `host=...` → PostgreSQL
- file paths / `.db` → SQLite

When no store options are provided (primarily in tests), the API falls back to an in-memory store.

## Application schema

Migrations are embedded in:

- `internal/store/migrations_sqlite.sql`
- `internal/store/migrations_postgres.sql`

Tables used by the current Go service:

| Table | Purpose |
| --- | --- |
| `receipts` | Delivery/read receipts |
| `responses` | Incoming responses |
| `flow_states` | Flow state + JSON state data |
| `conversation_participants` | Enrolled conversation participants |
| `registered_hooks` | Persisted response hooks |
| `jobs` | Durable job queue (replaces in-memory timers) |
| `outbox_messages` | Restart-safe outgoing message queue |
| `inbound_dedup` | Inbound message deduplication |

**Legacy tables** (present in migrations but unused by current code):

- `intervention_participants`
- `intervention_responses`

## Durable jobs (`jobs` table)

All time-based behavior (scheduled prompts, feedback timeouts, state transitions) is represented as persisted job records instead of in-memory timers. Each job has:

- `kind` – handler type (e.g., `recurring_prompt`, `state_transition`, `feedback_followup`)
- `run_at` – when the job should execute
- `payload_json` – job-specific parameters
- `status` – lifecycle state (`queued` → `running` → `done`/`failed`/`canceled`)
- `dedupe_key` – prevents duplicate jobs for the same intent

On startup, stale `running` jobs (locked before a configurable threshold) are requeued.

## Outbox messages (`outbox_messages` table)

Outgoing WhatsApp messages are first persisted to the outbox, then sent by a background worker. This ensures messages are not lost on crash. Each message has a `dedupe_key` to prevent duplicate sends.

## Inbound dedup (`inbound_dedup` table)

Inbound WhatsApp messages are deduplicated by message ID to prevent reprocessing after restart.

See `docs/persistence-audit.md` for a full state ledger.

## Flow state data

`flow_states.state_data` stores JSON (SQLite) or JSONB (Postgres) key/value pairs for conversation flow. Keys are defined in `internal/models/flow_types.go` (e.g., `conversationHistory`, `userProfile`, `scheduleRegistry`).

## Connection settings (SQLite)

SQLite connections are configured with:

- max open connections: 1
- max idle connections: 1
- max lifetime: 30 minutes

This avoids database lock contention.
