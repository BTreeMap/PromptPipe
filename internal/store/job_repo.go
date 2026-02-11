// Package store provides the JobRepo interface and model for durable job scheduling.
package store

import (
	"time"
)

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	JobStatusQueued   JobStatus = "queued"
	JobStatusRunning  JobStatus = "running"
	JobStatusDone     JobStatus = "done"
	JobStatusFailed   JobStatus = "failed"
	JobStatusCanceled JobStatus = "canceled"
)

// Job represents a durable job record that replaces in-memory timers.
type Job struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	RunAt       time.Time `json:"run_at"`
	PayloadJSON string    `json:"payload_json"`
	Status      JobStatus `json:"status"`
	Attempt     int       `json:"attempt"`
	MaxAttempts int       `json:"max_attempts"`
	LastError   string    `json:"last_error"`
	LockedAt    *time.Time `json:"locked_at"`
	DedupeKey   string    `json:"dedupe_key"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// JobRepo defines the interface for durable job persistence.
type JobRepo interface {
	// EnqueueJob inserts a new job. If dedupeKey is non-empty and a non-terminal
	// job with that key already exists, the call returns the existing job ID
	// without inserting a duplicate.
	EnqueueJob(kind string, runAt time.Time, payloadJSON string, dedupeKey string) (string, error)

	// ClaimDueJobs marks up to limit queued jobs whose run_at <= now as running
	// and returns them.
	ClaimDueJobs(now time.Time, limit int) ([]Job, error)

	// CompleteJob marks a job as done.
	CompleteJob(id string) error

	// FailJob marks a job as failed, stores the error, and reschedules for retry
	// at nextRunAt if attempt < max_attempts; otherwise marks as permanently failed.
	FailJob(id string, errMsg string, nextRunAt time.Time) error

	// CancelJob marks a job as canceled.
	CancelJob(id string) error

	// RequeueStaleRunningJobs resets jobs that have been running since before
	// staleBefore back to queued status (crash recovery).
	RequeueStaleRunningJobs(staleBefore time.Time) (int, error)

	// GetJob retrieves a single job by ID.
	GetJob(id string) (*Job, error)
}
