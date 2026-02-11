package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/util"
)

// Compile-time check that SQLiteStore implements OutboxRepo.
var _ OutboxRepo = (*SQLiteStore)(nil)

func (s *SQLiteStore) EnqueueOutboxMessage(participantID, kind, payloadJSON, dedupeKey string) (string, error) {
	id := util.GenerateRandomID("outbox_", 32)
	now := time.Now()

	if dedupeKey != "" {
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM outbox_messages WHERE dedupe_key = ? AND status NOT IN ('sent', 'canceled')`,
			dedupeKey,
		).Scan(&existingID)
		if err == nil {
			slog.Debug("SQLiteStore.EnqueueOutboxMessage: dedupe hit", "dedupeKey", dedupeKey, "existingID", existingID)
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("outbox dedupe check failed: %w", err)
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO outbox_messages (id, participant_id, kind, payload_json, status, attempts, dedupe_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'queued', 0, ?, ?, ?)`,
		id, participantID, kind, payloadJSON, nilIfEmpty(dedupeKey), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue outbox message failed: %w", err)
	}
	slog.Debug("SQLiteStore.EnqueueOutboxMessage", "id", id, "participantID", participantID, "kind", kind)
	return id, nil
}

func (s *SQLiteStore) ClaimDueOutboxMessages(now time.Time, limit int) ([]OutboxMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, participant_id, kind, payload_json, status, attempts, next_attempt_at, dedupe_key, locked_at, last_error, created_at, updated_at
		 FROM outbox_messages WHERE status = 'queued' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		 ORDER BY created_at ASC LIMIT ?`,
		now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim due outbox messages failed: %w", err)
	}
	defer rows.Close()

	var msgs []OutboxMessage
	for rows.Next() {
		m, err := scanOutboxMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim outbox iteration failed: %w", err)
	}

	for i := range msgs {
		_, err := s.db.Exec(
			`UPDATE outbox_messages SET status = 'sending', locked_at = ?, updated_at = ? WHERE id = ?`,
			now, now, msgs[i].ID,
		)
		if err != nil {
			return nil, fmt.Errorf("mark outbox sending failed: %w", err)
		}
		msgs[i].Status = OutboxStatusSending
		msgs[i].LockedAt = &now
	}

	return msgs, nil
}

func (s *SQLiteStore) MarkOutboxMessageSent(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'sent', updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("mark outbox sent failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) FailOutboxMessage(id string, errMsg string, nextAttemptAt time.Time) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'queued', attempts = attempts + 1, last_error = ?, next_attempt_at = ?, locked_at = NULL, updated_at = ? WHERE id = ?`,
		errMsg, nextAttemptAt, now, id,
	)
	if err != nil {
		return fmt.Errorf("fail outbox message failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RequeueStaleSendingMessages(staleBefore time.Time) (int, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'queued', locked_at = NULL, updated_at = ? WHERE status = 'sending' AND locked_at < ?`,
		now, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("requeue stale outbox messages failed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		slog.Info("SQLiteStore.RequeueStaleSendingMessages", "requeued", n)
	}
	return int(n), nil
}

func scanOutboxMessage(rows *sql.Rows) (OutboxMessage, error) {
	var m OutboxMessage
	var payloadJSON, dedupeKey, lastError sql.NullString
	var nextAttemptAt, lockedAt sql.NullTime
	err := rows.Scan(
		&m.ID, &m.ParticipantID, &m.Kind, &payloadJSON, &m.Status, &m.Attempts,
		&nextAttemptAt, &dedupeKey, &lockedAt, &lastError, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return m, fmt.Errorf("scan outbox message failed: %w", err)
	}
	m.PayloadJSON = payloadJSON.String
	m.DedupeKey = dedupeKey.String
	m.LastError = lastError.String
	if nextAttemptAt.Valid {
		m.NextAttemptAt = &nextAttemptAt.Time
	}
	if lockedAt.Valid {
		m.LockedAt = &lockedAt.Time
	}
	return m, nil
}
