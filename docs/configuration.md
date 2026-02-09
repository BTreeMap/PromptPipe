# Configuration

PromptPipe loads configuration from environment variables and command-line flags. It also attempts to load `.env` files from the following locations (first match wins):

1. `./.env`
2. `../.env`
3. `../../.env`

## Precedence

1. **CLI flags**
2. **Environment variables**
3. **Defaults**

`DATABASE_URL` is only used when `DATABASE_DSN` is unset.

## State directory

`PROMPTPIPE_STATE_DIR` controls where PromptPipe stores SQLite files and debug logs. The server acquires an exclusive lock at:

```
{STATE_DIR}/promptpipe.lock
```

Default: `/var/lib/promptpipe`

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `PROMPTPIPE_STATE_DIR` | `/var/lib/promptpipe` | Base state directory. |
| `WHATSAPP_DB_DSN` | `file:{STATE_DIR}/whatsmeow.db?_foreign_keys=on` | WhatsApp/whatsmeow database DSN. |
| `DATABASE_DSN` | `file:{STATE_DIR}/state.db?_foreign_keys=on` | Application database DSN. |
| `DATABASE_URL` | (empty) | Legacy fallback for `DATABASE_DSN`. |
| `API_ADDR` | `:8080` | HTTP server listen address. |
| `DEFAULT_SCHEDULE` | (empty) | Default cron string for `/schedule` when no schedule is provided. |
| `OPENAI_API_KEY` | (empty) | Enables GenAI features. |
| `GENAI_MODEL` | `gpt-4o-mini` | OpenAI model name. |
| `GENAI_TEMPERATURE` | `0.1` | OpenAI temperature (0.0–1.0). |
| `INTAKE_BOT_PROMPT_FILE` | `prompts/intake_bot_system.txt` | Intake bot system prompt. |
| `PROMPT_GENERATOR_PROMPT_FILE` | `prompts/prompt_generator_system.txt` | Prompt generator system prompt. |
| `FEEDBACK_TRACKER_PROMPT_FILE` | `prompts/feedback_tracker_system.txt` | Feedback tracker system prompt. |
| `CHAT_HISTORY_LIMIT` | `-1` | History messages sent to tools (-1 unlimited, 0 none). |
| `FEEDBACK_INITIAL_TIMEOUT` | `15m` | Initial feedback timeout. |
| `FEEDBACK_FOLLOWUP_DELAY` | `3h` | Follow-up feedback delay. |
| `SCHEDULER_PREP_TIME_MINUTES` | `10` | Minutes before prompt time for preparation messages. |
| `AUTO_FEEDBACK_AFTER_PROMPT_ENABLED` | `true` | Auto-enter feedback after scheduled prompt inactivity. |
| `AUTO_ENROLL_NEW_USERS` | `false` | Auto-enroll unknown phone numbers on first message. |
| `PROMPTPIPE_DEBUG` | `false` | Enables debug messages and GenAI call logging. |

GenAI-dependent features (`type=genai` prompts and the conversation flow) require `OPENAI_API_KEY`. Without it, `/schedule` rejects GenAI prompts and conversation replies cannot be generated.

### `DEFAULT_SCHEDULE` format

`DEFAULT_SCHEDULE` uses a **five-field** cron-like format:

```
minute hour day month weekday
```

Only simple numeric fields are supported. Ranges are accepted but only the first value is used (e.g., `1-5` is treated as `1`). If parsing fails, the default schedule is ignored and `/schedule` will require an explicit schedule object.

## Command-line flags

| Flag | Purpose |
| --- | --- |
| `--api-addr` | Override `API_ADDR`. |
| `--qr-output` | Path to write WhatsApp login QR code. |
| `--numeric-code` | Use numeric login code instead of QR. |
| `--state-dir` | Override `PROMPTPIPE_STATE_DIR`. |
| `--whatsapp-db-dsn` | Override `WHATSAPP_DB_DSN`. |
| `--app-db-dsn` | Override `DATABASE_DSN` / `DATABASE_URL`. |
| `--openai-api-key` | Override `OPENAI_API_KEY`. |
| `--default-cron` | Override `DEFAULT_SCHEDULE`. |
| `--debug` | Override `PROMPTPIPE_DEBUG`. |
| `--intake-bot-prompt-file` | Override `INTAKE_BOT_PROMPT_FILE`. |
| `--prompt-generator-prompt-file` | Override `PROMPT_GENERATOR_PROMPT_FILE`. |
| `--feedback-tracker-prompt-file` | Override `FEEDBACK_TRACKER_PROMPT_FILE`. |
| `--chat-history-limit` | Override `CHAT_HISTORY_LIMIT`. |
| `--feedback-initial-timeout` | Override `FEEDBACK_INITIAL_TIMEOUT`. |
| `--feedback-followup-delay` | Override `FEEDBACK_FOLLOWUP_DELAY`. |
| `--genai-temperature` | Override `GENAI_TEMPERATURE`. |
| `--genai-model` | Override `GENAI_MODEL`. |
| `--scheduler-prep-time-minutes` | Override `SCHEDULER_PREP_TIME_MINUTES`. |
| `--auto-feedback-after-prompt-enabled` | Override `AUTO_FEEDBACK_AFTER_PROMPT_ENABLED`. |
| `--auto-enroll-new-users` | Override `AUTO_ENROLL_NEW_USERS`. |

## Examples

```bash
# SQLite in a custom state dir
./build/promptpipe --state-dir /srv/promptpipe

# PostgreSQL application DB, SQLite whatsmeow DB
export DATABASE_DSN="postgres://user:pass@host/db?sslmode=disable"
./build/promptpipe

# Enable GenAI features
export OPENAI_API_KEY="..."
export GENAI_MODEL="gpt-4o-mini"
```
