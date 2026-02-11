package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/util"
)

// Compile-time check that PostgresStore implements JobRepo.
var _ JobRepo = (*PostgresStore)(nil)

func (s *PostgresStore) EnqueueJob(kind string, runAt time.Time, payloadJSON string, dedupeKey string) (string, error) {
	id := util.GenerateRandomID("job_", 32)
	now := time.Now()

	if dedupeKey != "" {
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM jobs WHERE dedupe_key = $1 AND status NOT IN ('done', 'canceled')`,
			dedupeKey,
		).Scan(&existingID)
		if err == nil {
			slog.Debug("PostgresStore.EnqueueJob: dedupe hit", "dedupeKey", dedupeKey, "existingID", existingID)
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("dedupe check failed: %w", err)
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO jobs (id, kind, run_at, payload_json, status, attempt, max_attempts, dedupe_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'queued', 0, 3, $5, $6, $7)`,
		id, kind, runAt, payloadJSON, nilIfEmpty(dedupeKey), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue job failed: %w", err)
	}
	slog.Debug("PostgresStore.EnqueueJob", "id", id, "kind", kind, "runAt", runAt)
	return id, nil
}

func (s *PostgresStore) ClaimDueJobs(now time.Time, limit int) ([]Job, error) {
	rows, err := s.db.Query(
		`UPDATE jobs SET status = 'running', locked_at = $1, updated_at = $1
		 WHERE id IN (
		   SELECT id FROM jobs WHERE status = 'queued' AND run_at <= $1
		   ORDER BY run_at ASC LIMIT $2
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, kind, run_at, payload_json, status, attempt, max_attempts, last_error, locked_at, dedupe_key, created_at, updated_at`,
		now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim due jobs failed: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim due jobs iteration failed: %w", err)
	}
	return jobs, nil
}

func (s *PostgresStore) CompleteJob(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'done', updated_at = $1 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("complete job failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) FailJob(id string, errMsg string, nextRunAt time.Time) error {
	now := time.Now()

	var attempt, maxAttempts int
	err := s.db.QueryRow(`SELECT attempt, max_attempts FROM jobs WHERE id = $1`, id).Scan(&attempt, &maxAttempts)
	if err != nil {
		return fmt.Errorf("fail job lookup failed: %w", err)
	}

	attempt++
	if attempt >= maxAttempts {
		_, err = s.db.Exec(
			`UPDATE jobs SET status = 'failed', attempt = $1, last_error = $2, locked_at = NULL, updated_at = $3 WHERE id = $4`,
			attempt, errMsg, now, id,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE jobs SET status = 'queued', attempt = $1, last_error = $2, run_at = $3, locked_at = NULL, updated_at = $4 WHERE id = $5`,
			attempt, errMsg, nextRunAt, now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("fail job update failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) CancelJob(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'canceled', locked_at = NULL, updated_at = $1 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cancel job failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) RequeueStaleRunningJobs(staleBefore time.Time) (int, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE jobs SET status = 'queued', locked_at = NULL, updated_at = $1 WHERE status = 'running' AND locked_at < $2`,
		now, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("requeue stale jobs failed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		slog.Info("PostgresStore.RequeueStaleRunningJobs", "requeued", n)
	}
	return int(n), nil
}

func (s *PostgresStore) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, kind, run_at, payload_json, status, attempt, max_attempts, last_error, locked_at, dedupe_key, created_at, updated_at
		 FROM jobs WHERE id = $1`, id,
	)
	j, err := scanJobRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get job failed: %w", err)
	}
	return &j, nil
}
