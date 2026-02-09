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

**Legacy tables** (present in migrations but unused by current code):

- `intervention_participants`
- `intervention_responses`

## Flow state data

`flow_states.state_data` stores JSON (SQLite) or JSONB (Postgres) key/value pairs for conversation flow. Keys are defined in `internal/models/flow_types.go` (e.g., `conversationHistory`, `userProfile`, `scheduleRegistry`).

## Connection settings (SQLite)

SQLite connections are configured with:

- max open connections: 1
- max idle connections: 1
- max lifetime: 30 minutes

This avoids database lock contention.
