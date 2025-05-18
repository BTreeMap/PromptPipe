# PromptPipe Documentation

Welcome to the PromptPipe documentation! This guide provides a comprehensive overview of the architecture, configuration, API, and usage of the PromptPipe messaging service.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [API Reference](#api-reference)
  - [POST /schedule](#post-schedule)
  - [POST /send](#post-send)
  - [GET /receipts](#get-receipts)
- [Data Models](#data-models)
  - [Prompt](#prompt)
  - [Receipt](#receipt)
- [Scheduling Prompts](#scheduling-prompts)
- [Receipt Tracking](#receipt-tracking)
- [Storage Backends](#storage-backends)
- [Environment Variables](#environment-variables)
- [Development](#development)
- [License](#license)

---

## Overview

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a RESTful API for scheduling messages, sending dynamic content, and tracking delivery/read receipts. The service is designed for easy integration with intervention logic and is highly configurable via environment variables.

## Architecture

- **Go Core**: Built with Go for high performance and concurrency.
- **Whatsmeow Integration**: Uses the official Whatsmeow client for WhatsApp messaging.
- **API Layer**: Exposes RESTful endpoints for scheduling, sending, and tracking prompts. (`internal/api`)
- **Scheduler**: Supports cron-based scheduling for recurring or one-time prompts. (`internal/scheduler`)
- **Store**: Persists scheduled prompts and receipt events. Supports in-memory and PostgreSQL. (`internal/store`)
- **WhatsApp Client**: Handles communication with the WhatsApp network. (`internal/whatsapp`)
- **Models**: Defines shared data structures. (`internal/models`)

## Installation

Clone the repository and build the binary:

```bash
git clone https://github.com/yourorg/PromptPipe.git
cd PromptPipe
make build
```

Or build directly with Go:

```bash
go build -o PromptPipe cmd/PromptPipe/main.go
```

## Configuration

Create a `.env` file or export the following environment variables:

- `WHATSAPP_DB_DRIVER`: Database driver for Whatsmeow storage (default: `postgres`)
- `WHATSAPP_DB_DSN`: Data source name for Whatsmeow DB (default: `postgres://postgres:postgres@localhost:5432/whatsapp?sslmode=disable`)
- `DATABASE_URL`: (Optional) PostgreSQL connection string for PromptPipe receipt storage
- `DEFAULT_SCHEDULE`: (Optional) Default cron schedule for prompts (e.g., `0 9 * * *` for 9 AM daily)
- `OPENAI_API_KEY`: (Optional) API key for OpenAI GenAI operations

## Usage

Start the service (reads `.env` automatically):

```bash
./PromptPipe serve
```

## API Reference

All API endpoints expect and return JSON.

### POST /schedule

Schedules a new prompt to be sent according to a cron expression.

**Request Body:** `Prompt` object (see [Data Models](#prompt)). Supports optional `system_prompt` and `user_prompt` fields for GenAI content.

**Responses:**

- `201 Created`: Prompt successfully scheduled.
- `400 Bad Request`: Invalid request payload.
- `500 Internal Server Error`: Error scheduling the prompt.

### POST /send

Sends a prompt immediately.

**Request Body:** `Prompt` object (see [Data Models](#prompt), `cron` field is ignored). Supports optional `system_prompt` and `user_prompt` fields to generate dynamic content.

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

## Data Models

### Prompt

Represents a message to be sent.

```json
{
  "to": "string (E.164 phone number)",
  "cron": "string (cron expression, optional for /send)",
  "body": "string (message content)",
  "system_prompt": "string (optional system prompt for GenAI)",
  "user_prompt": "string (optional user prompt for GenAI)"
}
```

- `to`: The recipient's WhatsApp phone number in E.164 format (e.g., `+15551234567`).
- `cron`: A standard cron expression (e.g., `0 9 * * *` for 9 AM daily). Required for `/schedule`.
- `body`: The text content of the message.
- `system_prompt`: Optional system prompt for generating dynamic content using GenAI.
- `user_prompt`: Optional user prompt for generating dynamic content using GenAI.

### Receipt

Represents a delivery or read receipt for a sent message.

```json
{
  "to": "string (E.164 phone number)",
  "status": "string (e.g., \"sent\", \"delivered\", \"read\")",
  "time": "int64 (Unix timestamp)"
}
```

- `to`: The recipient's WhatsApp phone number.
- `status`: The status of the message (e.g., "sent", "delivered", "read").
- `time`: Unix timestamp of when the receipt event occurred.

## Scheduling Prompts

The `/schedule` endpoint allows you to define messages that will be sent out based on a cron schedule. The `cron` field in the `Prompt` model uses standard cron syntax. The scheduler service (`internal/scheduler`) is responsible for managing these jobs.

## Receipt Tracking

The system tracks message events (sent, delivered, read) and stores them. These can be retrieved via the `/receipts` endpoint. The `internal/store` package handles the persistence of these receipts, with options for in-memory or PostgreSQL storage.

## Storage Backends

PromptPipe supports a unified storage interface for message receipts, with two implementations:

- **In-Memory Store**: Used for testing and development. Fast, but not persistent.
- **PostgreSQL Store**: Used in production. Set the `DATABASE_URL` environment variable to enable this backend. The database must have a `receipts` table with columns: `recipient TEXT`, `status TEXT`, `time BIGINT`.

The system will use the PostgreSQL store if `DATABASE_URL` is set, otherwise it defaults to in-memory storage.

## Environment Variables

| Variable             | Description                                 |
|----------------------|---------------------------------------------|
| WHATSAPP_DB_DRIVER   | Database driver for Whatsmeow storage       |
| WHATSAPP_DB_DSN      | Data source name for Whatsmeow DB           |
| DEFAULT_SCHEDULE     | Default cron schedule for prompts           |
| DATABASE_URL         | PostgreSQL connection string (optional)     |
| OPENAI_API_KEY       | API key for OpenAI GenAI operations         |

## Development

- Code is organized in the `internal/` directory by module (API, scheduler, store, WhatsApp integration).
- Tests are provided for each module.
- To run tests:

```bash
go test ./...
```

## License

This project is licensed under the MIT License. See the [LICENSE](../LICENSE) file for details.
