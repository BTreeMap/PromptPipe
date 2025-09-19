# PromptPipe

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [API Reference](#api-reference)
  - [POST /schedule](#post-schedule)
  - [POST /send](#post-send)
  - [GET /receipts](#get-receipts)
  - [POST /response](#post-response)
  - [GET /responses](#get-responses)
  - [GET /stats](#get-stats)
  - [GET /health](#get-health)
  - [Timers API](#timers-api)
  - [Conversation API](#conversation-api)
- [Data Models](#data-models)
  - [Prompt](#prompt)
  - [Receipt](#receipt)
  - [Response](#response)
- [Scheduling Prompts](#scheduling-prompts)
- [Receipt Tracking](#receipt-tracking)
- [Storage](#storage)
- [Environment Variables](#environment-variables)
- [Custom Flows](#custom-flows)
- [Development](#development)
- [License](#license)

## Overview

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a RESTful API for scheduling messages, sending dynamic or GenAI-generated content, and tracking delivery/read receipts. It also includes a stateful conversation flow (3‑bot architecture: Coordinator, Intake, Feedback). The service is highly configurable via environment variables and supports SQLite and PostgreSQL for the application database.

## Architecture

- **Go Core**: High-performance, concurrent backend written in Go.
- **Whatsmeow Integration**: Programmable WhatsApp client for messaging.
- **API Layer**: RESTful endpoints for scheduling, sending, and tracking prompts (`internal/api`).
- **Timer & Scheduler Tool**: Scheduling for recurring or one-time prompts via a timer and scheduler tool (`internal/flow/timer.go`, `internal/flow/scheduler_tool.go`).
- **Store**: Persists receipts, responses, conversation state, schedules, and hooks (`internal/store`).
- **Message Flow**: Pluggable flow generators produce message bodies (`internal/flow`).
  - **Flow Generators**: `static`, `branch`, `genai`, and the stateful `conversation` flow; you can register custom generators.
- **WhatsApp Client**: Handles WhatsApp network communication (`internal/whatsapp`).
- **GenAI**: Optional OpenAI integration for dynamic content (`internal/genai`).
- **Models**: Shared data structures (`internal/models`).

## Features

- **Schedule prompts** at specific times or intervals (cron syntax).
- **Messaging abstraction**: WhatsApp supported today via whatsmeow; interface allows adding other providers later.
- **Send dynamic payloads**: text, media, and template messages with custom variables.
- **GenAI-enhanced content**: Use OpenAI to generate message content dynamically.
- **Structured reasoning**: Agent modules always request JSON `{thinking, content}`; thinking is surfaced only in debug mode for developers (no toggle to avoid schema drift).
- **Branch flows**: Present selectable branch options to participants.
- **Custom flows**: Plug in your own `Generator` implementations for fully customized message-generation logic.
- **Receipt tracking**: Capture sent, delivered, and read events.
- **Modular design**: Integrates with any adaptive-intervention framework.
- **Clear REST API**: Easy integration with your application.
- **Customizable Message Flows**: Define and register custom flow generators for handling different prompt types.
- **Stateful conversation**: 3‑bot architecture with enrollment endpoints and recovery.

## Installation

```bash
# Clone the repository
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe

# Build the binary
make build
# Or use Go directly:
go build -o build/promptpipe cmd/PromptPipe/main.go
```

## Additional Configuration Details

PromptPipe uses two separate databases to clearly separate concerns:

1. **WhatsApp Database**: Used by the whatsmeow library for WhatsApp session data (we don't control this schema)
2. **Application Database**: Used for receipts, responses, and flow state (controlled by PromptPipe)

### Environment Variables

Create a `.env` file or export the following environment variables:

```bash
# WhatsApp/Whatsmeow Database Configuration
WHATSAPP_DB_DSN="file:/var/lib/promptpipe/whatsmeow.db?_foreign_keys=on"

# Application Database Configuration
DATABASE_DSN="postgres://user:pass@host:port/dbname?sslmode=disable"

# Legacy Support (DATABASE_URL will be used for application database if DATABASE_DSN is not set)
DATABASE_URL="postgres://user:pass@host:port/dbname?sslmode=disable"

# Other Configuration
PROMPTPIPE_STATE_DIR="/var/lib/promptpipe"    # Directory for file-based storage
DEFAULT_SCHEDULE="0 9 * * *"                  # Default cron schedule (9 AM daily)
API_ADDR=":8080"                              # API server address
OPENAI_API_KEY="your_openai_api_key"          # OpenAI API key for GenAI operations
# Optional GenAI settings
GENAI_MODEL="gpt-4o-mini"
GENAI_TEMPERATURE="0.1"

# Conversation Flow prompts and limits
INTAKE_BOT_PROMPT_FILE="prompts/intake_bot_system.txt"
PROMPT_GENERATOR_PROMPT_FILE="prompts/prompt_generator_system.txt"
FEEDBACK_TRACKER_PROMPT_FILE="prompts/feedback_tracker_system.txt"
CHAT_HISTORY_LIMIT="-1"                         # -1=unlimited, 0=none, N=last N messages to tools
FEEDBACK_INITIAL_TIMEOUT="15m"
FEEDBACK_FOLLOWUP_DELAY="3h"
SCHEDULER_PREP_TIME_MINUTES="10"
AUTO_FEEDBACK_AFTER_PROMPT_ENABLED="true"
PROMPTPIPE_DEBUG="false"
```

### Database Configuration Examples

#### Default Configuration (No Environment Variables)

If no database configuration is provided, both databases will use SQLite files:

- WhatsApp database: `{STATE_DIR}/whatsmeow.db` (with foreign keys enabled)
- Application database: `{STATE_DIR}/state.db`

#### PostgreSQL for Both Databases

```bash
WHATSAPP_DB_DSN="postgres://user:pass@host:port/whatsapp_db?sslmode=disable"
DATABASE_DSN="postgres://user:pass@host:port/app_db?sslmode=disable"
```

#### Mixed Configuration (PostgreSQL for App, SQLite for WhatsApp)

```bash
DATABASE_DSN="postgres://user:pass@host:port/app_db?sslmode=disable"
# WHATSAPP_DB_DSN not set - will default to SQLite with foreign keys
```

#### Mixed Configuration (PostgreSQL for WhatsApp, SQLite for App)

```bash
WHATSAPP_DB_DSN="postgres://user:pass@host:port/whatsapp_db?sslmode=disable"
# DATABASE_DSN not set - will default to SQLite
```

### SQLite Foreign Keys

**Important**: The whatsmeow library strongly recommends enabling foreign keys for SQLite databases to ensure data integrity. PromptPipe will automatically enable foreign keys in default SQLite configurations for the WhatsApp database.

If you provide a custom SQLite DSN for the WhatsApp database without foreign keys enabled, PromptPipe will log a warning message recommending you add `?_foreign_keys=on` to your connection string.

Example SQLite DSN with foreign keys: `file:/path/to/database.db?_foreign_keys=on`

## Usage

```bash
# Start the service (reads .env automatically)
./build/promptpipe [flags]
```

## Command Line Flags

- `-api-addr` API server address (overrides $API_ADDR)
- `-qr-output` path to write login QR code
- `-numeric-code` use numeric login code instead of QR code
- `-state-dir` state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)
- `-whatsapp-db-dsn` WhatsApp/whatsmeow DB connection string (overrides $WHATSAPP_DB_DSN)
- `-app-db-dsn` application DB connection string (overrides $DATABASE_DSN or $DATABASE_URL)
- `-openai-api-key` OpenAI API key (overrides $OPENAI_API_KEY)
- `-default-cron` default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)
- `-intake-bot-prompt-file` path to intake bot system prompt file
- `-prompt-generator-prompt-file` path to prompt generator system prompt file
- `-feedback-tracker-prompt-file` path to feedback tracker system prompt file
- `-chat-history-limit` limit of history messages exposed to tools (-1=no limit)
- `-feedback-initial-timeout` e.g., 15m
- `-feedback-followup-delay` e.g., 3h
- `-genai-temperature` 0.0–1.0
- `-genai-model` OpenAI model name
- `-scheduler-prep-time-minutes` default 10
- `-auto-feedback-after-prompt-enabled` default true
- `-debug` enable debug logging + structured thinking surfacing

## API Reference

All API endpoints expect and return JSON.

### POST /schedule

Schedules a new prompt to be sent according to a cron expression.

**Request Body:** `Prompt` object (see [Data Models](#prompt)). Supports optional `system_prompt` and `user_prompt` for GenAI. Provide a `schedule` object to define timing.

**Response Body:** `{"status":"ok"}`

**Responses:**

- `201 Created`: Prompt successfully scheduled.
- `400 Bad Request`: Invalid request payload.
- `500 Internal Server Error`: Error scheduling the prompt.

### POST /send

Sends a prompt immediately.

**Request Body:** `Prompt` object (see [Data Models](#prompt); any `schedule` is ignored). Supports optional `system_prompt` and `user_prompt` to generate dynamic content.

**Response Body:** `{"status":"ok"}`

**Responses:**

- `200 OK`: Prompt successfully sent.
- `400 Bad Request`: Invalid request payload.
- `500 Internal Server Error`: Error sending the prompt.

### GET /receipts

Fetches all stored delivery and read receipt events.

**Response Body:** Array of `Receipt` objects (see [Data Models](#receipt))

**Responses:**

- `200 OK`: Successfully retrieved receipts.
- `500 Internal Server Error`: Error fetching receipts.

### POST /response

Collects a participant's response message.

**Request Body:** `Response` object (see [Data Models](#response)).

**Response Body:** `{"status":"ok"}`

**Responses:**

- `201 Created`: Response successfully recorded.
- `400 Bad Request`: Invalid request payload.
- `500 Internal Server Error`: Error recording response.

### GET /responses

Retrieves all collected participant responses.

**Response Body:** Array of `Response` objects.

**Responses:**

- `200 OK`: Successfully retrieved responses.
- `500 Internal Server Error`: Error fetching responses.

### GET /stats

Provides statistics over collected responses (total count, per sender counts, average response length).

**Response Body:** JSON object with fields:

- `total_responses`: integer
- `responses_per_sender`: map of sender to count
- `avg_response_length`: float

**Responses:**

- `200 OK`: Successfully retrieved statistics.
- `500 Internal Server Error`: Error computing statistics.

### GET /health

Basic health/status endpoint.

Responses:

- `200 OK` healthy
- `503 Service Unavailable` degraded, with error info

### Timers API

- `GET /timers` – list active timers
- `GET /timers/{id}` – get info for a timer
- `DELETE /timers/{id}` – cancel a timer

### Conversation API

- `POST /conversation/participants` – enroll a participant
- `GET /conversation/participants` – list participants
- `GET /conversation/participants/{id}` – get participant
- `PUT /conversation/participants/{id}` – update participant
- `DELETE /conversation/participants/{id}` – remove participant

## Data Models

### Prompt

Represents a message to be sent, supporting multiple flow types (static, genai, branch, conversation, or custom).

```json
{
  "to": "+15551234567",
  "schedule": {
    "minute": 0,
    "hour": 9,
    "weekday": 1,
    "timezone": "America/Toronto"
  },
  "type": "static | genai | branch | conversation | custom",
  "state": "string (custom flows only)",
  "body": "string (for static)",
  "system_prompt": "string (for genai/conversation)",
  "user_prompt": "string (for genai/conversation)",
  "branch_options": [ { "label": "string", "body": "string" } ]
}
```

- `to`: The recipient's WhatsApp phone number in E.164 format (e.g., `+15551234567`).
- `schedule`: Object for minute/hour/day/month/weekday + optional timezone. Required for `/schedule` if no default schedule is configured.
- `type`: The type of flow to use for generating the message (e.g., "static", "genai", "branch", "custom").
- `state`: Optional current state for custom flows.
- `body`: The text content of the message or prompt template.
- `system_prompt`: Optional system prompt for generating dynamic content using GenAI.
- `user_prompt`: Optional user prompt for generating dynamic content using GenAI.
- `branch_options`: Optional list of branch options for "branch" type flows.

### Receipt

Represents a delivery or read receipt for a sent message.

```json
{
  "to": "string (E.164 phone number)",
  "status": "string (e.g., \"sent\", \"delivered\", \"read\", \"failed\", \"error\", \"scheduled\", \"cancelled\")",
  "time": "int64 (Unix timestamp)"
}
```

- `to`: The recipient's WhatsApp phone number.
- `status`: The status of the message (e.g., "sent", "delivered", "read", "failed", "error", "scheduled", "cancelled").
- `time`: Unix timestamp of when the receipt event occurred.

### Response

Represents an incoming response from a participant.

```json
{
  "from": "string (E.164 phone number)",
  "body": "string (message content)",
  "time": "int64 (Unix timestamp)"
}
```

- `from`: The sender's WhatsApp phone number in E.164 format.
- `body`: The text content of the response message.
- `time`: Unix timestamp of when the response was received.

## Scheduling Prompts

The `/schedule` endpoint allows you to define messages that will be sent out based on a structured schedule object (minute/hour/day/month/weekday, optional timezone). A simple cron-like conversion is supported internally for timers.

## Receipt Tracking

The system tracks message events (sent, delivered, read) and stores them. These can be retrieved via the `/receipts` endpoint. The `internal/store` package handles the persistence of these receipts.

## Storage

PromptPipe uses two separate databases to separate concerns:

- WhatsApp database (managed by whatsmeow) – configured via `WHATSAPP_DB_DSN` or `-whatsapp-db-dsn` (SQLite by default at `{STATE_DIR}/whatsmeow.db?_foreign_keys=on`).
- Application database (managed by PromptPipe) – configured via `DATABASE_DSN` or `-app-db-dsn` (SQLite by default at `{STATE_DIR}/state.db?_foreign_keys=on`).

PostgreSQL is supported for the application database by providing a `postgres://...` DSN. The project auto-detects DSN type. An in-memory store exists primarily for tests.

## Configuration

### State Directory

PromptPipe uses a state directory to store SQLite databases and other persistent data:

- **Default**: `/var/lib/promptpipe`
- **Environment Variable**: `PROMPTPIPE_STATE_DIR`
- **Command Line Flag**: `-state-dir`

The SQLite database file is automatically placed at `{state-dir}/promptpipe.db` unless a specific database DSN is provided.

### Database Configuration

The database can be configured in several ways (in order of precedence):

1. **Command Line Flags**: `-whatsapp-db-dsn`, `-app-db-dsn`
2. **Environment Variables**: `WHATSAPP_DB_DSN`, `DATABASE_DSN` (or legacy `DATABASE_URL`)
3. **Default**: SQLite databases under `{state-dir}` with foreign keys enabled

Examples:

```bash
# Use SQLite in custom location
./build/promptpipe -app-db-dsn "file:/path/to/state.db?_foreign_keys=on"

# Use PostgreSQL
./build/promptpipe -app-db-dsn "postgres://user:password@localhost/promptpipe?sslmode=disable"

# Use custom state directory (SQLite will be at /custom/path/promptpipe.db)
./build/promptpipe -state-dir /custom/path

# Environment variable configuration
export DATABASE_DSN="postgres://user:password@localhost/promptpipe?sslmode=disable"
export PROMPTPIPE_STATE_DIR="/custom/state/dir"
./build/promptpipe
```

## Additional Environment Variables

| Variable                         | Description |
|----------------------------------|-------------|
| PROMPTPIPE_STATE_DIR             | State directory for PromptPipe data |
| WHATSAPP_DB_DSN                  | WhatsApp/whatsmeow DB DSN |
| DATABASE_DSN                     | Application DB DSN (or legacy DATABASE_URL) |
| DEFAULT_SCHEDULE                 | Default cron schedule for /schedule when none provided |
| API_ADDR                         | API server address |
| OPENAI_API_KEY                   | API key for OpenAI GenAI operations |
| GENAI_MODEL                      | OpenAI model (default gpt-4o-mini) |
| GENAI_TEMPERATURE                | OpenAI temperature (default 0.1) |
| INTAKE_BOT_PROMPT_FILE           | Path to intake system prompt |
| PROMPT_GENERATOR_PROMPT_FILE     | Path to prompt-generator system prompt |
| FEEDBACK_TRACKER_PROMPT_FILE     | Path to feedback tracker system prompt |
| CHAT_HISTORY_LIMIT               | History window exposed to tools (-1/0/N) |
| FEEDBACK_INITIAL_TIMEOUT         | Initial feedback timeout (e.g., 15m) |
| FEEDBACK_FOLLOWUP_DELAY          | Follow-up feedback delay (e.g., 3h) |
| SCHEDULER_PREP_TIME_MINUTES      | Minutes before target time for prep notifications |
| AUTO_FEEDBACK_AFTER_PROMPT_ENABLED | Auto-enforce feedback session after scheduled prompt inactivity |
| PROMPTPIPE_DEBUG                 | Enable debug mode and write API call logs under {STATE_DIR}/debug |

## Custom Flows

You can define your own message-generation flows by implementing the `flow.Generator` interface:

```go
 type Generator interface {
     Generate(ctx context.Context, p models.Prompt) (string, error)
 }
```

Then register your generator with a `PromptType` in an `init()` function:

```go
 func init() {
     flow.Register(models.PromptTypeCustom, &MyCustomGenerator{})
 }
```

Set `type: "custom"` in your `Prompt` JSON; the API will dispatch to your generator.

## Development

- Code is organized in the `internal/` directory by module (API, scheduler, store, WhatsApp integration, GenAI, models).
- Tests are provided for each module.
- To run tests:

```bash
go test ./...
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
