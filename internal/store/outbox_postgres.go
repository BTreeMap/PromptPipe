package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/util"
)

// Compile-time check that PostgresStore implements OutboxRepo.
var _ OutboxRepo = (*PostgresStore)(nil)

func (s *PostgresStore) EnqueueOutboxMessage(participantID, kind, payloadJSON, dedupeKey string) (string, error) {
	id := util.GenerateRandomID("outbox_", 32)
	now := time.Now()

	if dedupeKey != "" {
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM outbox_messages WHERE dedupe_key = $1 AND status NOT IN ('sent', 'canceled')`,
			dedupeKey,
		).Scan(&existingID)
		if err == nil {
			slog.Debug("PostgresStore.EnqueueOutboxMessage: dedupe hit", "dedupeKey", dedupeKey, "existingID", existingID)
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("outbox dedupe check failed: %w", err)
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO outbox_messages (id, participant_id, kind, payload_json, status, attempts, dedupe_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'queued', 0, $5, $6, $7)`,
		id, participantID, kind, payloadJSON, nilIfEmpty(dedupeKey), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue outbox message failed: %w", err)
	}
	slog.Debug("PostgresStore.EnqueueOutboxMessage", "id", id, "participantID", participantID, "kind", kind)
	return id, nil
}

func (s *PostgresStore) ClaimDueOutboxMessages(now time.Time, limit int) ([]OutboxMessage, error) {
	rows, err := s.db.Query(
		`UPDATE outbox_messages SET status = 'sending', locked_at = $1, updated_at = $1
		 WHERE id IN (
		   SELECT id FROM outbox_messages WHERE status = 'queued' AND (next_attempt_at IS NULL OR next_attempt_at <= $1)
		   ORDER BY created_at ASC LIMIT $2
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, participant_id, kind, payload_json, status, attempts, next_attempt_at, dedupe_key, locked_at, last_error, created_at, updated_at`,
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
	return msgs, nil
}

func (s *PostgresStore) MarkOutboxMessageSent(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'sent', updated_at = $1 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("mark outbox sent failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) FailOutboxMessage(id string, errMsg string, nextAttemptAt time.Time) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'queued', attempts = attempts + 1, last_error = $1, next_attempt_at = $2, locked_at = NULL, updated_at = $3 WHERE id = $4`,
		errMsg, nextAttemptAt, now, id,
	)
	if err != nil {
		return fmt.Errorf("fail outbox message failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) RequeueStaleSendingMessages(staleBefore time.Time) (int, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE outbox_messages SET status = 'queued', locked_at = NULL, updated_at = $1 WHERE status = 'sending' AND locked_at < $2`,
		now, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("requeue stale outbox messages failed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		slog.Info("PostgresStore.RequeueStaleSendingMessages", "requeued", n)
	}
	return int(n), nil
}
