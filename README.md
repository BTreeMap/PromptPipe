# PromptPipe

## Description

PromptPipe is a Go-based messaging service that delivers adaptive-intervention prompts over WhatsApp using the [whatsmeow](https://github.com/tulir/whatsmeow) library. It provides a clean API for scheduling messages, sending dynamic content, and tracking delivery/read receipts, all configured via environment variables for easy integration with your intervention logic.

## Features

* **Go Core**: Written in Go for performance and concurrency.
* **Whatsmeow Integration**: Uses the official whatsmeow client for WhatsApp messaging.
* **Scheduling**: Schedule prompts at specific times or intervals.
* **Dynamic Payloads**: Send text, media, and template messages with custom variables.
* **Receipt Tracking**: Capture sent, delivered, and read events.
* **Modular Design**: Plug into any adaptive-intervention framework with minimal boilerplate.
* **Clear API**: RESTful endpoints for easy integration with your application.

## Installation

```bash
# Clone the repository
git clone https://github.com/yourorg/PromptPipe.git
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
# Whatsmeow QR store directory
WHATSAPP_STORE_PATH=/path/to/whatsapp/store

# Environment for Whatsmeow client
GO_ENV=production

# (Optional) Scheduling defaults
default_schedule="0 9 * * *"  # cron format for 9 AM daily
```

## Usage

```bash
# Start the service (reads .env automatically)
./PromptPipe serve
```

### API Endpoints

| Endpoint    | Method | Description                        |
| ----------- | ------ | ---------------------------------- |
| `/schedule` | POST   | Schedule a new prompt              |
| `/send`     | POST   | Send a prompt immediately          |
| `/receipts` | GET    | Fetch delivery/read receipt events |

#### Example `schedule` payload

```json
{
  "to": "+15551234567",
  "cron": "0 8 * * *",
  "body": "Good morning! Don't forget your mindfulness exercise today."
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
