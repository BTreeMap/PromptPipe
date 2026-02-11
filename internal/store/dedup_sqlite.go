package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Compile-time check that SQLiteStore implements DedupRepo.
var _ DedupRepo = (*SQLiteStore)(nil)

func (s *SQLiteStore) IsDuplicate(messageID string) (bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT message_id FROM inbound_dedup WHERE message_id = ?`, messageID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dedup check failed: %w", err)
	}
	return true, nil
}

func (s *SQLiteStore) RecordInbound(messageID, participantID string) (bool, error) {
	// First check if it already exists
	exists, err := s.IsDuplicate(messageID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	now := time.Now()
	_, err = s.db.Exec(
		`INSERT OR IGNORE INTO inbound_dedup (message_id, participant_id, received_at) VALUES (?, ?, ?)`,
		messageID, participantID, now,
	)
	if err != nil {
		return false, fmt.Errorf("record inbound failed: %w", err)
	}
	return true, nil
}

func (s *SQLiteStore) MarkProcessed(messageID string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE inbound_dedup SET processed_at = ? WHERE message_id = ?`,
		now, messageID,
	)
	if err != nil {
		return fmt.Errorf("mark processed failed: %w", err)
	}
	return nil
}
