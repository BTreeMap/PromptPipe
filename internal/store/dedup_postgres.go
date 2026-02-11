package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Compile-time check that PostgresStore implements DedupRepo.
var _ DedupRepo = (*PostgresStore)(nil)

func (s *PostgresStore) IsDuplicate(messageID string) (bool, error) {
	var id string
	err := s.db.QueryRow(`SELECT message_id FROM inbound_dedup WHERE message_id = $1`, messageID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dedup check failed: %w", err)
	}
	return true, nil
}

func (s *PostgresStore) RecordInbound(messageID, participantID string) (bool, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO inbound_dedup (message_id, participant_id, received_at) VALUES ($1, $2, $3) ON CONFLICT (message_id) DO NOTHING`,
		messageID, participantID, now,
	)
	if err != nil {
		return false, fmt.Errorf("record inbound failed: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("dedup rows affected check failed: %w", err)
	}
	return n > 0, nil
}

func (s *PostgresStore) MarkProcessed(messageID string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE inbound_dedup SET processed_at = $1 WHERE message_id = $2`,
		now, messageID,
	)
	if err != nil {
		return fmt.Errorf("mark processed failed: %w", err)
	}
	return nil
}
