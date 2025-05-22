// Package scheduler provides scheduling logic for PromptPipe.
//
// It allows jobs (such as sending WhatsApp prompts) to be scheduled using cron expressions.
package scheduler

import (
	"github.com/robfig/cron/v3"
)

// Opts holds configuration options for the Scheduler (e.g., custom cron parser).
type Opts struct {
	// Placeholder for future scheduler configurations
}

// Option defines a configuration option for the Scheduler.
type Option func(*Opts)

// Scheduler provides cron-based job scheduling.
type Scheduler struct {
	cron *cron.Cron
}

// NewScheduler creates and starts a cron scheduler, applying any provided options.
func NewScheduler(opts ...Option) *Scheduler {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	// Use standard 5-field cron parser (min, hour, dom, month, dow) and enable recovery
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	c := cron.New(cron.WithParser(parser), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	c.Start()
	return &Scheduler{cron: c}
}

// AddJob schedules a task using the provided cron expression.
// It returns an error if the expression is invalid.
func (s *Scheduler) AddJob(expr string, task func()) error {
	_, err := s.cron.AddFunc(expr, task)
	return err
}

// Stop stops the cron scheduler and waits for running jobs to finish.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
