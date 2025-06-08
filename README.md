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
- [Data Models](#data-models)
  - [Prompt](#prompt)
  - [Receipt](#receipt)
  - [Response](#response)
- [Scheduling Prompts](#scheduling-prompts)
- [Receipt Tracking](#receipt-tracking)
- [Storage Backends](#storage-backends)
- [Environment Variables](#environment-variables)
- [Custom Flows](#custom-flows)
- [Development](#development)
- [License](#license)

## Overview

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a RESTful API for scheduling messages, sending dynamic or GenAI-generated content, and tracking delivery/read receipts. The service is highly configurable via environment variables and supports both in-memory and PostgreSQL storage backends.

## Architecture

- **Go Core**: High-performance, concurrent backend written in Go.
- **Whatsmeow Integration**: Programmable WhatsApp client for messaging.
- **API Layer**: RESTful endpoints for scheduling, sending, and tracking prompts (`internal/api`).
- **Scheduler**: Cron-based scheduling for recurring or one-time prompts (`internal/scheduler`).
- **Store**: Persists scheduled prompts and receipt events; supports in-memory and PostgreSQL (`internal/store`).
- **Message Flow**: Pluggable flow generators produce message bodies (`internal/flow`).
  - **Flow Generators**: Pluggable modules under `internal/flow` that transform a `Prompt` into a message string. By default, `static` and `branch` are registered; you can register your own generators (e.g. `genai` or `custom`).
- **WhatsApp Client**: Handles WhatsApp network communication (`internal/whatsapp`).
- **GenAI**: Optional OpenAI integration for dynamic content (`internal/genai`).
- **Models**: Shared data structures (`internal/models`).

## Features

- **Schedule prompts** at specific times or intervals (cron syntax).
- **Pluggable messaging drivers**: Easily switch between WhatsApp, Twilio, or other providers without changing core logic.
- **Send dynamic payloads**: text, media, and template messages with custom variables.
- **GenAI-enhanced content**: Use OpenAI to generate message content dynamically.
- **Branch flows**: Present selectable branch options to participants.
- **Custom flows**: Plug in your own `Generator` implementations for fully customized message-generation logic.
- **Receipt tracking**: Capture sent, delivered, and read events.
- **Modular design**: Integrates with any adaptive-intervention framework.
- **Clear REST API**: Easy integration with your application.
- **Customizable Message Flows**: Define and register custom flow generators for handling different prompt types.

## Installation

```bash
# Clone the repository
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe

# Build the binary
make build
# Or use Go directly:
go build -o PromptPipe cmd/PromptPipe/main.go
```

## Example Configuration

Create a `.env` file or export the following environment variables:

```bash
# Whatsmeow DB driver (e.g., postgres)
WHATSAPP_DB_DRIVER=postgres
# Whatsmeow DB DSN for SQL store
WHATSAPP_DB_DSN="postgres://postgres:postgres@localhost:5432/whatsapp?sslmode=disable"
# (Optional) State directory for PromptPipe data
PROMPTPIPE_STATE_DIR="/var/lib/promptpipe"
# (Optional) Default cron schedule for prompts
DEFAULT_SCHEDULE="0 9 * * *"  # 9 AM daily
# (Optional) PostgreSQL connection string for receipts
DATABASE_URL="postgres://user:pass@host:port/dbname?sslmode=disable"
# (Optional) API server address
API_ADDR=":8080"
# (Optional) OpenAI API key for GenAI operations
OPENAI_API_KEY="your_openai_api_key"
```

## Usage

```bash
# Start the service (reads .env automatically)
./PromptPipe [flags]
```

## Flags

- `-api-addr string` : API server address (overrides $API_ADDR)
- `-qr-output string` : path to write login QR code (default: stdout)
- `-numeric-code`    : use numeric login code instead of QR code
- `-state-dir string`: state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)
- `-db-driver string`: database driver for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DRIVER / $DATABASE_URL)
- `-db-dsn string`   : database DSN for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DSN / $DATABASE_URL)
- `-openai-api-key string`: OpenAI API key (overrides $OPENAI_API_KEY)
- `-default-cron string`: default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)

## API Reference

All API endpoints expect and return JSON.

### POST /schedule

Schedules a new prompt to be sent according to a cron expression.

**Request Body:** `Prompt` object (see [Data Models](#prompt)). Supports optional `system_prompt` and `user_prompt` fields for GenAI content.

**Response Body:** `{"status":"scheduled"}`

**Responses:**

- `201 Created`: Prompt successfully scheduled.
- `400 Bad Request`: Invalid request payload.
- `500 Internal Server Error`: Error scheduling the prompt.

### POST /send

Sends a prompt immediately.

**Request Body:** `Prompt` object (see [Data Models](#prompt), `cron` field is ignored). Supports optional `system_prompt` and `user_prompt` fields to generate dynamic content.

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

**Response Body:** `{"status":"recorded"}`

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

## Data Models

### Prompt

Represents a message to be sent, supporting multiple flow types (static, GenAI, branch, or custom).

```json
{
  "to": "string (E.164 phone number)",
  "cron": "string (cron expression, optional for /send)",
  "type": "string (one of \"static\", \"genai\", \"branch\", \"custom\"; defaults to \"static\")",
  "state": "string (current state for custom flows, optional)",
  "body": "string (message content or prompt template)",
  "system_prompt": "string (optional system prompt for GenAI)",
  "user_prompt": "string (optional user prompt for GenAI)",
  "branch_options": [ { "label": "string", "body": "string" } ],
  // custom flow modules may define additional fields here
}
```

- `to`: The recipient's WhatsApp phone number in E.164 format (e.g., `+15551234567`).
- `cron`: A standard cron expression (e.g., `0 9 * * *` for 9 AM daily). Required for `/schedule`.
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

The `/schedule` endpoint allows you to define messages that will be sent out based on a cron schedule. The `cron` field in the `Prompt` model uses standard cron syntax. The scheduler service (`internal/scheduler`) is responsible for managing these jobs.

## Receipt Tracking

The system tracks message events (sent, delivered, read) and stores them. These can be retrieved via the `/receipts` endpoint. The `internal/store` package handles the persistence of these receipts, with options for in-memory or PostgreSQL storage.

## Storage Backends

PromptPipe supports a unified storage interface for message receipts and responses, with three implementations:

- **SQLite Store**: Default storage backend. Stores data in a SQLite database file. By default, uses `/var/lib/promptpipe/promptpipe.db`. Configured via the `-db-dsn` flag or `DATABASE_URL` environment variable with a file path.
- **PostgreSQL Store**: Enterprise storage backend. Provide a PostgreSQL DSN via the `-db-dsn` flag or `DATABASE_URL` environment variable to enable this backend. The system auto-detects PostgreSQL DSNs (containing `postgres://`, `host=`, or multiple key=value pairs).
- **In-Memory Store**: Used for testing and development. Fast, but not persistent. Used when no database configuration is provided.

All backends support the same schema with the following tables:

- `receipts` (`id SERIAL PRIMARY KEY`, `recipient TEXT NOT NULL`, `status TEXT NOT NULL`, `time BIGINT NOT NULL`)
- `responses` (`id SERIAL PRIMARY KEY`, `sender TEXT NOT NULL`, `body TEXT NOT NULL`, `time BIGINT NOT NULL`)

## Configuration

### State Directory

PromptPipe uses a state directory to store SQLite databases and other persistent data:

- **Default**: `/var/lib/promptpipe`
- **Environment Variable**: `PROMPTPIPE_STATE_DIR`
- **Command Line Flag**: `-state-dir`

The SQLite database file is automatically placed at `{state-dir}/promptpipe.db` unless a specific database DSN is provided.

### Database Configuration

The database can be configured in several ways (in order of precedence):

1. **Command Line Flag**: `-db-dsn <connection-string-or-file-path>`
2. **Environment Variables**: `WHATSAPP_DB_DSN` or `DATABASE_URL`
3. **Default**: SQLite database at `{state-dir}/promptpipe.db`

Examples:

```bash
# Use SQLite in custom location
./promptpipe -db-dsn /path/to/my/database.db

# Use PostgreSQL
./promptpipe -db-dsn "postgres://user:password@localhost/promptpipe"

# Use custom state directory (SQLite will be at /custom/path/promptpipe.db)
./promptpipe -state-dir /custom/path

# Environment variable configuration
export DATABASE_URL="postgres://user:password@localhost/promptpipe"
export PROMPTPIPE_STATE_DIR="/custom/state/dir"
./promptpipe
```

## Environment Variables

| Variable               | Description                                 |
|------------------------|---------------------------------------------|
| PROMPTPIPE_STATE_DIR   | State directory for PromptPipe data        |
| WHATSAPP_DB_DRIVER     | Database driver for Whatsmeow storage      |
| WHATSAPP_DB_DSN        | Data source name for Whatsmeow DB          |
| DATABASE_URL           | Database connection string (PostgreSQL/SQLite) |
| DEFAULT_SCHEDULE       | Default cron schedule for prompts          |
| API_ADDR               | API server address                          |
| OPENAI_API_KEY         | API key for OpenAI GenAI operations        |

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
