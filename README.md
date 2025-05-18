# PromptPipe

## Description

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a clean API for scheduling messages, sending dynamic content, and tracking delivery/read receipts, all configured via environment variables for easy integration with your intervention logic.

> **Detailed documentation is available in [docs.md](docs.md).**

## Features

* **Go Core**: Written in Go for performance and concurrency.
* **Whatsmeow Integration**: Uses the official Whatsmeow client for WhatsApp messaging.
* **Scheduling**: Schedule prompts at specific times or intervals.
* **Dynamic Payloads**: Send text, media, and template messages with custom variables.
* **Receipt Tracking**: Capture sent, delivered, and read events.
* **Modular Design**: Integrates with any adaptive-intervention framework with minimal boilerplate.
* **Clear API**: RESTful endpoints for easy integration with your application.
* **GenAI-Enhanced Content**: Use OpenAI to generate message content dynamically based on system and user prompts.

## Installation

```bash
# Clone the repository
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe

# Build the binary
make build
```

*Or use **`go build`** directly:*

```bash
go build -o PromptPipe cmd/PromptPipe/main.go
```

## Configuration

Create a `.env` file or export the following environment variables:

```bash
# Whatsmeow DB driver (e.g., postgres)
WHATSAPP_DB_DRIVER=postgres

# Whatsmeow DB DSN for SQL store
WHATSAPP_DB_DSN="postgres://postgres:postgres@localhost:5432/whatsapp?sslmode=disable"

# (Optional) Scheduling default cron expression
DEFAULT_SCHEDULE="0 9 * * *"  # cron format for 9 AM daily

# (Optional) Database for receipts (Postgres)
DATABASE_URL="postgres://user:pass@host:port/dbname?sslmode=disable"

# OpenAI API key for GenAI operations
OPENAI_API_KEY="your_openai_api_key"
```

## Usage

```bash
# Start the service (reads .env automatically)
./PromptPipe
```

### API Endpoints

| Endpoint    | Method | Description                          |
| ----------- | ------ | ------------------------------------ |
| `/schedule` | POST   | Schedule a new prompt. Supports optional `system_prompt` and `user_prompt` fields for GenAI content. |
| `/send`     | POST   | Send a prompt immediately. Supports optional `system_prompt` and `user_prompt` fields to generate dynamic content. |
| `/receipts` | GET    | Fetch delivery/read receipt events   |

#### Example `schedule` payload

```json
{
  "to": "+15551234567",
  "cron": "0 8 * * *",
  "body": "Good morning feature!",
  "system_prompt": "You are a friendly reminder bot.",
  "user_prompt": "Please send a motivational quote for today."
}
```

## Example

```bash
# Immediately send a test prompt
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{"to":"+15551234567","body":"Test from PromptPipe!"}'
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
