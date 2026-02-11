package store

import (
	"database/sql"
	"fmt"
)

// nilIfEmpty returns nil if s is empty, otherwise returns s.
// Used for nullable database columns.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// scanJob scans a Job from sql.Rows.
func scanJob(rows *sql.Rows) (Job, error) {
	var j Job
	var payloadJSON, lastError, dedupeKey sql.NullString
	var lockedAt sql.NullTime
	err := rows.Scan(
		&j.ID, &j.Kind, &j.RunAt, &payloadJSON, &j.Status, &j.Attempt, &j.MaxAttempts,
		&lastError, &lockedAt, &dedupeKey, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return j, fmt.Errorf("scan job failed: %w", err)
	}
	j.PayloadJSON = payloadJSON.String
	j.LastError = lastError.String
	j.DedupeKey = dedupeKey.String
	if lockedAt.Valid {
		j.LockedAt = &lockedAt.Time
	}
	return j, nil
}

// scanJobRow scans a Job from a single sql.Row.
func scanJobRow(row *sql.Row) (Job, error) {
	var j Job
	var payloadJSON, lastError, dedupeKey sql.NullString
	var lockedAt sql.NullTime
	err := row.Scan(
		&j.ID, &j.Kind, &j.RunAt, &payloadJSON, &j.Status, &j.Attempt, &j.MaxAttempts,
		&lastError, &lockedAt, &dedupeKey, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return j, err
	}
	j.PayloadJSON = payloadJSON.String
	j.LastError = lastError.String
	j.DedupeKey = dedupeKey.String
	if lockedAt.Valid {
		j.LockedAt = &lockedAt.Time
	}
	return j, nil
}

// scanOutboxMessage scans an OutboxMessage from sql.Rows.
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
