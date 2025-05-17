// Package scheduler provides scheduling logic for PromptPipe.
//
// It allows jobs (such as sending WhatsApp prompts) to be scheduled using cron expressions.
package scheduler

import (
	"time"
)

type Job struct {
	Cron string
	Task func()
}

// Scheduler is a placeholder for cron scheduling logic

type Scheduler struct {
	jobs []Job
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) AddJob(cron string, task func()) error {
	// TODO: Parse cron and schedule task
	s.jobs = append(s.jobs, Job{Cron: cron, Task: task})
	return nil
}

func (s *Scheduler) Start() {
	// TODO: Implement cron-based scheduling
	for _, job := range s.jobs {
		go func(j Job) {
			for {
				// Placeholder: run every minute
				j.Task()
				time.Sleep(time.Minute)
			}
		}(job)
	}
}
