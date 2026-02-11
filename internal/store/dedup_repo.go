// Package store provides the DedupRepo interface for inbound message deduplication.
package store

import (
	"time"
)

// DedupRecord represents an inbound message deduplication record.
type DedupRecord struct {
	MessageID     string     `json:"message_id"`
	ParticipantID string     `json:"participant_id"`
	ReceivedAt    time.Time  `json:"received_at"`
	ProcessedAt   *time.Time `json:"processed_at"`
}

// DedupRepo defines the interface for inbound message deduplication.
type DedupRepo interface {
	// IsDuplicate checks if a message ID has already been processed.
	// Returns true if the message was already seen.
	IsDuplicate(messageID string) (bool, error)

	// RecordInbound inserts a new inbound message record. Returns false if the
	// message was already recorded (duplicate).
	RecordInbound(messageID, participantID string) (bool, error)

	// MarkProcessed sets the processed_at timestamp for a message.
	MarkProcessed(messageID string) error
}
