// Package flow provides timer implementations for scheduled actions.
package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// timerEntry tracks information about a scheduled timer
type timerEntry struct {
	timer       *time.Timer
	scheduledAt time.Time
	expiresAt   time.Time
	description string
}

// SimpleTimer implements the Timer interface using Go's standard time package.
type SimpleTimer struct {
	timers map[string]*timerEntry
	mu     sync.RWMutex
	nextID int64
}

// NewSimpleTimer creates a new SimpleTimer.
func NewSimpleTimer() *SimpleTimer {
	slog.Debug("Creating SimpleTimer")
	return &SimpleTimer{
		timers: make(map[string]*timerEntry),
	}
}

// ScheduleAfter schedules a function to run after a delay.
func (t *SimpleTimer) ScheduleAfter(delay time.Duration, fn func()) (string, error) {
	t.mu.Lock()
	t.nextID++
	id := fmt.Sprintf("timer_%d", t.nextID)
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleAfter", "id", id, "delay", delay)

	now := time.Now()
	expiresAt := now.Add(delay)

	timer := time.AfterFunc(delay, func() {
		slog.Debug("SimpleTimer executing scheduled function", "id", id)
		fn()
		// Clean up timer reference
		t.mu.Lock()
		delete(t.timers, id)
		t.mu.Unlock()
	})

	t.mu.Lock()
	t.timers[id] = &timerEntry{
		timer:       timer,
		scheduledAt: now,
		expiresAt:   expiresAt,
		description: fmt.Sprintf("Timer scheduled for %v", delay),
	}
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleAfter succeeded", "id", id, "delay", delay)
	return id, nil
}

// ScheduleAt schedules a function to run at a specific time.
func (t *SimpleTimer) ScheduleAt(when time.Time, fn func()) (string, error) {
	delay := time.Until(when)
	if delay < 0 {
		slog.Warn("SimpleTimer ScheduleAt: time is in the past, executing immediately", "when", when)
		go fn()
		return "", nil
	}

	slog.Debug("SimpleTimer ScheduleAt", "when", when, "delay", delay)
	return t.ScheduleAfter(delay, fn)
}

// Cancel cancels a scheduled function by ID.
func (t *SimpleTimer) Cancel(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry, exists := t.timers[id]; exists {
		entry.timer.Stop()
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
	for id, entry := range t.timers {
		entry.timer.Stop()
		slog.Debug("SimpleTimer stopped timer", "id", id)
	}
	t.timers = make(map[string]*timerEntry)
	slog.Info("SimpleTimer stopped all timers")
}

// ListActive returns information about all active timers.
func (t *SimpleTimer) ListActive() []models.TimerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]models.TimerInfo, 0, len(t.timers))
	now := time.Now()

	for id, entry := range t.timers {
		remaining := entry.expiresAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}

		result = append(result, models.TimerInfo{
			ID:          id,
			ScheduledAt: entry.scheduledAt,
			ExpiresAt:   entry.expiresAt,
			Remaining:   remaining.String(),
			Description: entry.description,
		})
	}

	slog.Debug("SimpleTimer ListActive", "count", len(result))
	return result
}

// GetTimer returns information about a specific timer by ID.
func (t *SimpleTimer) GetTimer(id string) (*models.TimerInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entry, exists := t.timers[id]
	if !exists {
		return nil, fmt.Errorf("timer with ID %s not found", id)
	}

	now := time.Now()
	remaining := entry.expiresAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}

	info := &models.TimerInfo{
		ID:          id,
		ScheduledAt: entry.scheduledAt,
		ExpiresAt:   entry.expiresAt,
		Remaining:   remaining.String(),
		Description: entry.description,
	}

	slog.Debug("SimpleTimer GetTimer", "id", id, "remaining", remaining)
	return info, nil
}
