// Package store provides the OutboxRepo interface and model for restart-safe outgoing sends.
package store

import (
	"time"
)

// OutboxStatus represents the lifecycle state of an outbox message.
type OutboxStatus string

const (
	OutboxStatusQueued   OutboxStatus = "queued"
	OutboxStatusSending  OutboxStatus = "sending"
	OutboxStatusSent     OutboxStatus = "sent"
	OutboxStatusFailed   OutboxStatus = "failed"
	OutboxStatusCanceled OutboxStatus = "canceled"
)

// OutboxMessage represents a durable outgoing message record.
type OutboxMessage struct {
	ID            string       `json:"id"`
	ParticipantID string       `json:"participant_id"`
	Kind          string       `json:"kind"`
	PayloadJSON   string       `json:"payload_json"`
	Status        OutboxStatus `json:"status"`
	Attempts      int          `json:"attempts"`
	NextAttemptAt *time.Time   `json:"next_attempt_at"`
	DedupeKey     string       `json:"dedupe_key"`
	LockedAt      *time.Time   `json:"locked_at"`
	LastError     string       `json:"last_error"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// OutboxRepo defines the interface for durable outbox message persistence.
type OutboxRepo interface {
	// EnqueueOutboxMessage inserts a new outbox message. If dedupeKey is non-empty
	// and a non-terminal message with that key exists, returns the existing ID.
	EnqueueOutboxMessage(participantID, kind, payloadJSON, dedupeKey string) (string, error)

	// ClaimDueOutboxMessages marks up to limit queued messages whose
	// next_attempt_at <= now (or is NULL) as sending and returns them.
	ClaimDueOutboxMessages(now time.Time, limit int) ([]OutboxMessage, error)

	// MarkOutboxMessageSent marks a message as successfully sent.
	MarkOutboxMessageSent(id string) error

	// FailOutboxMessage records a send failure and schedules a retry at nextAttemptAt.
	FailOutboxMessage(id string, errMsg string, nextAttemptAt time.Time) error

	// RequeueStaleSendingMessages resets messages stuck in sending since before
	// staleBefore back to queued (crash recovery).
	RequeueStaleSendingMessages(staleBefore time.Time) (int, error)
}
