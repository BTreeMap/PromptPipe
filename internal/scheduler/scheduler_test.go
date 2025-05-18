package scheduler

import "testing"

func TestSchedulerAddJob(t *testing.T) {
	s := NewScheduler()
	// Should add a valid cron job without error
	if err := s.AddJob("* * * * *", func() {}); err != nil {
		t.Errorf("Expected no error adding job, got %v", err)
	}
}
