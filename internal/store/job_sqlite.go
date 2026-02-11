package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/util"
)

// Compile-time check that SQLiteStore implements JobRepo.
var _ JobRepo = (*SQLiteStore)(nil)

func (s *SQLiteStore) EnqueueJob(kind string, runAt time.Time, payloadJSON string, dedupeKey string) (string, error) {
	id := util.GenerateRandomID("job_", 32)
	now := time.Now()

	if dedupeKey != "" {
		// Check for existing non-terminal job with same dedupe key
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM jobs WHERE dedupe_key = ? AND status NOT IN ('done', 'canceled')`,
			dedupeKey,
		).Scan(&existingID)
		if err == nil {
			slog.Debug("SQLiteStore.EnqueueJob: dedupe hit", "dedupeKey", dedupeKey, "existingID", existingID)
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("dedupe check failed: %w", err)
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO jobs (id, kind, run_at, payload_json, status, attempt, max_attempts, dedupe_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'queued', 0, 3, ?, ?, ?)`,
		id, kind, runAt, payloadJSON, nilIfEmpty(dedupeKey), now, now,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue job failed: %w", err)
	}
	slog.Debug("SQLiteStore.EnqueueJob", "id", id, "kind", kind, "runAt", runAt)
	return id, nil
}

func (s *SQLiteStore) ClaimDueJobs(now time.Time, limit int) ([]Job, error) {
	rows, err := s.db.Query(
		`SELECT id, kind, run_at, payload_json, status, attempt, max_attempts, last_error, locked_at, dedupe_key, created_at, updated_at
		 FROM jobs WHERE status = 'queued' AND run_at <= ? ORDER BY run_at ASC LIMIT ?`,
		now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim due jobs query failed: %w", err)
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

	// Mark claimed jobs as running
	for i := range jobs {
		_, err := s.db.Exec(
			`UPDATE jobs SET status = 'running', locked_at = ?, updated_at = ? WHERE id = ?`,
			now, now, jobs[i].ID,
		)
		if err != nil {
			return nil, fmt.Errorf("mark job running failed: %w", err)
		}
		jobs[i].Status = JobStatusRunning
		jobs[i].LockedAt = &now
	}

	return jobs, nil
}

func (s *SQLiteStore) CompleteJob(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'done', updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("complete job failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) FailJob(id string, errMsg string, nextRunAt time.Time) error {
	now := time.Now()

	var attempt, maxAttempts int
	err := s.db.QueryRow(`SELECT attempt, max_attempts FROM jobs WHERE id = ?`, id).Scan(&attempt, &maxAttempts)
	if err != nil {
		return fmt.Errorf("fail job lookup failed: %w", err)
	}

	attempt++
	if attempt >= maxAttempts {
		_, err = s.db.Exec(
			`UPDATE jobs SET status = 'failed', attempt = ?, last_error = ?, locked_at = NULL, updated_at = ? WHERE id = ?`,
			attempt, errMsg, now, id,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE jobs SET status = 'queued', attempt = ?, last_error = ?, run_at = ?, locked_at = NULL, updated_at = ? WHERE id = ?`,
			attempt, errMsg, nextRunAt, now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("fail job update failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CancelJob(id string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'canceled', locked_at = NULL, updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("cancel job failed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RequeueStaleRunningJobs(staleBefore time.Time) (int, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE jobs SET status = 'queued', locked_at = NULL, updated_at = ? WHERE status = 'running' AND locked_at < ?`,
		now, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("requeue stale jobs failed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		slog.Info("SQLiteStore.RequeueStaleRunningJobs", "requeued", n)
	}
	return int(n), nil
}

func (s *SQLiteStore) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, kind, run_at, payload_json, status, attempt, max_attempts, last_error, locked_at, dedupe_key, created_at, updated_at
		 FROM jobs WHERE id = ?`, id,
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
