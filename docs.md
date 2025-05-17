# PromptPipe Documentation

Welcome to the PromptPipe documentation! This guide provides a comprehensive overview of the architecture, configuration, API, and usage of the PromptPipe messaging service.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [API Reference](#api-reference)
- [Scheduling Prompts](#scheduling-prompts)
- [Receipt Tracking](#receipt-tracking)
- [Environment Variables](#environment-variables)
- [Development](#development)
- [License](#license)

---

## Overview

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a RESTful API for scheduling messages, sending dynamic content, and tracking delivery/read receipts. The service is designed for easy integration with intervention logic and is highly configurable via environment variables.

## Architecture

- **Go Core**: Built with Go for high performance and concurrency.
- **Whatsmeow Integration**: Uses the official Whatsmeow client for WhatsApp messaging.
- **API Layer**: Exposes RESTful endpoints for scheduling, sending, and tracking prompts.
- **Scheduler**: Supports cron-based scheduling for recurring or one-time prompts.
- **Store**: Persists scheduled prompts and receipt events (supports PostgreSQL).
- **Receipt Tracking**: Captures sent, delivered, and read events for each message.

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

- `WHATSAPP_STORE_PATH`: Path to WhatsApp QR store directory (required)
- `GO_ENV`: Environment for Whatsmeow client (default: `production`)
- `default_schedule`: (Optional) Default cron schedule (e.g., `0 9 * * *` for 9 AM daily)
- `DATABASE_URL`: (Optional) PostgreSQL connection string for persistent storage

## Usage

Start the service (reads `.env` automatically):

```bash
./PromptPipe serve
```

## API Reference

### Endpoints

| Endpoint    | Method | Description                        |
| ----------- | ------ | ---------------------------------- |
| `/schedule` | POST   | Schedule a new prompt              |
| `/send`     | POST   | Send a prompt immediately          |
| `/receipts` | GET    | Fetch delivery/read receipt events |

### Example `schedule` Payload

```json
{
  "to": "+15551234567",
  "cron": "0 8 * * *",
  "body": "Good morning! Don't forget your mindfulness exercise today."
}
```

### Example: Send a Test Prompt

```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{"to":"+15551234567","body":"Test from PromptPipe!"}'
```

## Scheduling Prompts

- Use the `/schedule` endpoint to schedule prompts using cron syntax.
- Supports dynamic payloads (text, media, templates).
- Prompts can be scheduled for specific times or intervals.

## Receipt Tracking

- The `/receipts` endpoint returns sent, delivered, and read events for each message.
- Useful for monitoring engagement and delivery status.

## Environment Variables

| Variable             | Description                                 |
|----------------------|---------------------------------------------|
| WHATSAPP_STORE_PATH  | Path to WhatsApp QR store directory         |
| GO_ENV               | Whatsmeow client environment                |
| default_schedule     | Default cron schedule for prompts           |
| DATABASE_URL         | PostgreSQL connection string (optional)     |

## Development

- Code is organized in the `internal/` directory by module (API, scheduler, store, WhatsApp integration).
- Tests are provided for each module.
- To run tests:

```bash
go test ./...
```

## License

This project is licensed under the MIT License. See the [LICENSE](../LICENSE) file for details.
