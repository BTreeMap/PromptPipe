// Package store provides the JobRunner for executing durable jobs.
package store

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// JobHandler is a function that executes a job's work. It receives the job's
// payload JSON and returns an error if the execution failed.
type JobHandler func(ctx context.Context, payload string) error

// JobRunner periodically claims due jobs from the database and dispatches them
// to registered handlers.
type JobRunner struct {
	repo           JobRepo
	handlers       map[string]JobHandler
	mu             sync.RWMutex
	pollInterval   time.Duration
	staleThreshold time.Duration
	claimLimit     int
}

// NewJobRunner creates a new JobRunner.
func NewJobRunner(repo JobRepo, pollInterval time.Duration) *JobRunner {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	return &JobRunner{
		repo:           repo,
		handlers:       make(map[string]JobHandler),
		pollInterval:   pollInterval,
		staleThreshold: 5 * time.Minute,
		claimLimit:     10,
	}
}

// RegisterHandler registers a handler for a given job kind.
func (r *JobRunner) RegisterHandler(kind string, handler JobHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = handler
	slog.Debug("JobRunner.RegisterHandler", "kind", kind)
}

// RecoverStaleJobs requeues jobs that were running when the process crashed.
// Should be called once at startup.
func (r *JobRunner) RecoverStaleJobs() error {
	staleBefore := time.Now().Add(-r.staleThreshold)
	n, err := r.repo.RequeueStaleRunningJobs(staleBefore)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("JobRunner.RecoverStaleJobs: requeued stale jobs", "count", n)
	}
	return nil
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (r *JobRunner) Run(ctx context.Context) {
	slog.Info("JobRunner.Run: starting job runner", "pollInterval", r.pollInterval)

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("JobRunner.Run: stopping")
			return
		case <-ticker.C:
			r.poll(ctx)
		}
	}
}

func (r *JobRunner) poll(ctx context.Context) {
	now := time.Now()
	jobs, err := r.repo.ClaimDueJobs(now, r.claimLimit)
	if err != nil {
		slog.Error("JobRunner.poll: claim failed", "error", err)
		return
	}

	for _, job := range jobs {
		r.mu.RLock()
		handler, ok := r.handlers[job.Kind]
		r.mu.RUnlock()

		if !ok {
			slog.Warn("JobRunner.poll: no handler for job kind", "kind", job.Kind, "id", job.ID)
			nextRun := now.Add(time.Minute)
			if err := r.repo.FailJob(job.ID, "no handler registered for kind: "+job.Kind, nextRun); err != nil {
				slog.Error("JobRunner.poll: fail job error", "id", job.ID, "error", err)
			}
			continue
		}

		slog.Debug("JobRunner.poll: executing job", "id", job.ID, "kind", job.Kind, "attempt", job.Attempt)
		if err := handler(ctx, job.PayloadJSON); err != nil {
			slog.Error("JobRunner.poll: job execution failed", "id", job.ID, "kind", job.Kind, "error", err)
			// Exponential backoff: 30s, 60s, 120s, ...
			backoff := time.Duration(30*(1<<job.Attempt)) * time.Second
			nextRun := now.Add(backoff)
			if err := r.repo.FailJob(job.ID, err.Error(), nextRun); err != nil {
				slog.Error("JobRunner.poll: fail job error", "id", job.ID, "error", err)
			}
		} else {
			if err := r.repo.CompleteJob(job.ID); err != nil {
				slog.Error("JobRunner.poll: complete job error", "id", job.ID, "error", err)
			}
			slog.Debug("JobRunner.poll: job completed", "id", job.ID, "kind", job.Kind)
		}
	}
}
