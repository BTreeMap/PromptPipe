// Package scheduler provides scheduling logic for PromptPipe.
//
// It allows jobs (such as sending WhatsApp prompts) to be scheduled using cron expressions.
package scheduler

import (
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// Default configuration constants
const (
	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultCronParserFields defines the standard 5-field cron parser configuration
	DefaultCronParserFields = cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow
)

// Opts holds configuration options for the Scheduler (e.g., custom cron parser).
type Opts struct {
	ShutdownTimeout time.Duration // timeout for graceful shutdown
	ParserFields    cron.ParseOption // cron parser configuration
}

// Option defines a configuration option for the Scheduler.
type Option func(*Opts)

// WithShutdownTimeout overrides the shutdown timeout for the scheduler.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(o *Opts) {
		o.ShutdownTimeout = timeout
	}
}

// WithParserFields overrides the cron parser fields for the scheduler.
func WithParserFields(fields cron.ParseOption) Option {
	return func(o *Opts) {
		o.ParserFields = fields
	}
}

// Scheduler provides cron-based job scheduling.
type Scheduler struct {
	cron            *cron.Cron
	shutdownTimeout time.Duration
}

// NewScheduler creates and starts a cron scheduler, applying any provided options.
func NewScheduler(opts ...Option) *Scheduler {
	// Apply options with defaults
	cfg := Opts{
		ShutdownTimeout: DefaultShutdownTimeout,
		ParserFields:    DefaultCronParserFields,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	
	// Use configured cron parser and enable recovery
	parser := cron.NewParser(cfg.ParserFields)
	c := cron.New(cron.WithParser(parser), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	c.Start()
	
	slog.Info("Scheduler started", "shutdownTimeout", cfg.ShutdownTimeout)
	return &Scheduler{
		cron:            c,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// AddJob schedules a task using the provided cron expression.
// It returns an error if the expression is invalid.
func (s *Scheduler) AddJob(expr string, task func()) error {
	slog.Debug("Scheduler AddJob invoked", "expr", expr)
	id, err := s.cron.AddFunc(expr, task)
	if err != nil {
		slog.Error("Scheduler AddJob failed", "expr", expr, "error", err)
		return err
	}
	slog.Info("Scheduler job added", "expr", expr, "jobID", id)
	return nil
}

// Stop stops the cron scheduler and waits for running jobs to finish.
// Uses a configurable timeout for graceful shutdown.
func (s *Scheduler) Stop() {
	slog.Debug("Scheduler stopping", "shutdownTimeout", s.shutdownTimeout)
	ctx := s.cron.Stop()
	
	// Wait for running jobs to complete with timeout
	select {
	case <-ctx.Done():
		slog.Info("Scheduler stopped gracefully")
	case <-time.After(s.shutdownTimeout):
		slog.Warn("Scheduler stop timeout reached, some jobs may still be running", "timeout", s.shutdownTimeout)
	}
}

// GetJobCount returns the number of currently scheduled jobs.
func (s *Scheduler) GetJobCount() int {
	entries := s.cron.Entries()
	return len(entries)
}
