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

### POST /schedule

Schedule a new prompt to be sent at a specified time or interval using cron syntax.

**Request Body:**

```json
{
  "to": "+15551234567",
  "cron": "0 8 * * *",
  "body": "Good morning! Don't forget your mindfulness exercise today."
}
```

**Go Implementation Example:**

```go
// internal/api/api.go
func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
    var req models.ScheduleRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }
    err := s.scheduler.SchedulePrompt(req.To, req.Cron, req.Body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusCreated)
}
```

### POST /send

Send a prompt immediately to a WhatsApp user.

**Request Body:**

```json
{
  "to": "+15551234567",
  "body": "Test from PromptPipe!"
}
```

**Go Implementation Example:**

```go
// internal/api/api.go
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
    var req models.SendRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }
    err := s.whatsapp.SendMessage(req.To, req.Body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
}
```

### GET /receipts

Fetch delivery and read receipt events for sent messages.

**Go Implementation Example:**

```go
// internal/api/api.go
func (s *Server) handleReceipts(w http.ResponseWriter, r *http.Request) {
    receipts, err := s.store.GetReceipts()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(receipts)
}
```

## Scheduling Prompts

- Use the `/schedule` endpoint to schedule prompts using cron syntax.
- Supports dynamic payloads (text, media, templates).
- Prompts can be scheduled for specific times or intervals.
- The scheduler module parses cron expressions and triggers message sending at the correct time.

**Go Example:**

```go
// internal/scheduler/scheduler.go
func (s *Scheduler) SchedulePrompt(to, cronExpr, body string) error {
    // Parse cron, store job, and register callback
    // ...implementation...
}
```

## Receipt Tracking

- The `/receipts` endpoint returns sent, delivered, and read events for each message.
- Receipts are stored in the database and can be queried for analytics or monitoring.

**Go Example:**

```go
// internal/store/store.go
func (s *Store) GetReceipts() ([]models.Receipt, error) {
    // Query database for receipts
    // ...implementation...
}
```

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
