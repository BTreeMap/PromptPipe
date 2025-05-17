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
    // The actual scheduling logic resides in the scheduler package
    _, err := s.scheduler.AddJob(req.Cron, func() {
        // This function will be executed based on the cron schedule
        // You would typically call your WhatsApp sending logic here
        // For example: s.whatsapp.SendMessage(req.To, req.Body)
        log.Printf("Executing scheduled job for %s: %s", req.To, req.Body)
    })
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
    // The actual message sending logic resides in the whatsapp package
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
    // The actual logic for fetching receipts resides in the store package
    receipts, err := s.store.GetReceipts() // Assuming GetReceipts can return an error
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
package scheduler

import (
    "log"
    "time"

    "github.com/robfig/cron/v3"
)

// Scheduler holds the cron instance and manages scheduled jobs.
// It uses the robfig/cron library for robust cron expression parsing and scheduling.
type Scheduler struct {
    cron *cron.Cron
}

// NewScheduler creates and starts a new cron scheduler.
func NewScheduler() *Scheduler {
    c := cron.New(cron.WithSeconds()) // Optional: use WithSeconds() if you need second-level precision
    c.Start()
    log.Println("Cron scheduler started")
    return &Scheduler{cron: c}
}

// AddJob schedules a new task based on the cron expression.
// The task is a function that will be executed according to the schedule.
func (s *Scheduler) AddJob(cronExpr string, task func()) (cron.EntryID, error) {
    id, err := s.cron.AddFunc(cronExpr, task)
    if err != nil {
        log.Printf("Error adding job with cron expression '%s': %v", cronExpr, err)
        return 0, err
    }
    log.Printf("Scheduled job with ID %d for cron expression: %s", id, cronExpr)
    return id, nil
}

// RemoveJob stops and removes a scheduled job by its ID.
func (s *Scheduler) RemoveJob(id cron.EntryID) {
    s.cron.Remove(id)
    log.Printf("Removed job with ID %d", id)
}

// Stop gracefully shuts down the cron scheduler.
func (s *Scheduler) Stop() {
    s.cron.Stop() // Stops the scheduler from running further jobs
    log.Println("Cron scheduler stopped")
}

// Example usage (typically in your main application setup):
// scheduler := scheduler.NewScheduler()
// defer scheduler.Stop() // Ensure scheduler is stopped gracefully on application exit
//
// // Schedule a task to run every minute
// scheduler.AddJob("* * * * *", func() {
//     log.Println("Executing a scheduled task every minute!")
// })
//
// // Schedule a task to run at a specific time (e.g., 9 AM every day)
// scheduler.AddJob("0 9 * * *", func() {
//     log.Println("Executing a scheduled task at 9 AM daily!")
// })
```

## Receipt Tracking

- The `/receipts` endpoint returns sent, delivered, and read events for each message.
- Receipts are stored in the database and can be queried for analytics or monitoring.
- The `store` package handles the persistence and retrieval of these receipts.

**Go Example:**

```go
// internal/store/store.go
package store

import (
    "sync"

    "github.com/BTreeMap/PromptPipe/internal/models"
    // "database/sql" // Uncomment if using a SQL database like PostgreSQL
    // _ "github.com/lib/pq" // PostgreSQL driver
)

// Store defines the interface for storage operations related to receipts.
// This allows for different storage implementations (e.g., in-memory, PostgreSQL).
type Store interface {
    AddReceipt(r models.Receipt) error
    GetReceipts() ([]models.Receipt, error)
}

// InMemoryStore is a simple in-memory store for receipts, primarily for testing or simple deployments.
// It uses a mutex to handle concurrent access safely.
type InMemoryStore struct {
    mu       sync.RWMutex
    receipts []models.Receipt
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
    return &InMemoryStore{
        receipts: make([]models.Receipt, 0),
    }
}

// AddReceipt adds a new receipt to the in-memory store.
func (s *InMemoryStore) AddReceipt(r models.Receipt) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.receipts = append(s.receipts, r)
    return nil
}

// GetReceipts retrieves all receipts from the in-memory store.
func (s *InMemoryStore) GetReceipts() ([]models.Receipt, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    // Return a copy to prevent external modification of the internal slice
    copiedReceipts := make([]models.Receipt, len(s.receipts))
    copy(copiedReceipts, s.receipts)
    return copiedReceipts, nil
}

// TODO: Implement PostgreSQLStore that implements the Store interface
// type PostgreSQLStore struct {
//     db *sql.DB
// }
//
// func NewPostgreSQLStore(dataSourceName string) (*PostgreSQLStore, error) {
//     db, err := sql.Open("postgres", dataSourceName)
//     if err != nil {
//         return nil, err
//     }
//     if err = db.Ping(); err != nil {
//         return nil, err
//     }
//     return &PostgreSQLStore{db: db}, nil
// }
//
// func (s *PostgreSQLStore) AddReceipt(r models.Receipt) error {
//     // SQL INSERT statement for receipts
//     return nil
// }
//
// func (s *PostgreSQLStore) GetReceipts() ([]models.Receipt, error) {
//     // SQL SELECT statement for receipts
//     return nil, nil
// }
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
