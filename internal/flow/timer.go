// Package flow provides timer implementations for scheduled actions.
package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SimpleTimer implements the Timer interface using Go's standard time package.
type SimpleTimer struct {
	timers map[string]*time.Timer
	mu     sync.RWMutex
	nextID int64
}

// NewSimpleTimer creates a new SimpleTimer.
func NewSimpleTimer() *SimpleTimer {
	slog.Debug("Creating SimpleTimer")
	return &SimpleTimer{
		timers: make(map[string]*time.Timer),
	}
}

// ScheduleAfter schedules a function to run after a delay.
func (t *SimpleTimer) ScheduleAfter(delay time.Duration, fn func()) error {
	t.mu.Lock()
	t.nextID++
	id := fmt.Sprintf("timer_%d", t.nextID)
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleAfter", "id", id, "delay", delay)

	timer := time.AfterFunc(delay, func() {
		slog.Debug("SimpleTimer executing scheduled function", "id", id)
		fn()
		// Clean up timer reference
		t.mu.Lock()
		delete(t.timers, id)
		t.mu.Unlock()
	})

	t.mu.Lock()
	t.timers[id] = timer
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleAfter succeeded", "id", id, "delay", delay)
	return nil
}

// ScheduleAt schedules a function to run at a specific time.
func (t *SimpleTimer) ScheduleAt(when time.Time, fn func()) error {
	delay := time.Until(when)
	if delay < 0 {
		slog.Warn("SimpleTimer ScheduleAt: time is in the past, executing immediately", "when", when)
		go fn()
		return nil
	}

	slog.Debug("SimpleTimer ScheduleAt", "when", when, "delay", delay)
	return t.ScheduleAfter(delay, fn)
}

// Cancel cancels a scheduled function by ID.
func (t *SimpleTimer) Cancel(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if timer, exists := t.timers[id]; exists {
		timer.Stop()
		delete(t.timers, id)
		slog.Debug("SimpleTimer Cancel succeeded", "id", id)
		return nil
	}

	slog.Debug("SimpleTimer Cancel: timer not found", "id", id)
	return nil
}

// Stop cancels all scheduled timers.
func (t *SimpleTimer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	slog.Debug("SimpleTimer stopping all timers", "count", len(t.timers))
	for id, timer := range t.timers {
		timer.Stop()
		slog.Debug("SimpleTimer stopped timer", "id", id)
	}
	t.timers = make(map[string]*time.Timer)
	slog.Info("SimpleTimer stopped all timers")
}
