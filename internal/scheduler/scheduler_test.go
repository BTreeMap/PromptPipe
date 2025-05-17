package scheduler

import "testing"

func TestSchedulerAddJob(t *testing.T) {
	s := NewScheduler()
	_ = false // Remove unused variable warning
	s.AddJob("* * * * *", func() {})
	if len(s.jobs) != 1 {
		t.Error("Job not added to scheduler")
	}
}
