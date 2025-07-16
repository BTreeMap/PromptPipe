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
	timer       *time.Timer // For both one-time and recurring timers
	timerType   string      // "once" or "recurring"
	scheduledAt time.Time
	expiresAt   time.Time        // For one-time timers
	schedule    *models.Schedule // For recurring timers
	nextRun     time.Time        // Next execution time
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
		timerType:   "once",
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

// ScheduleWithSchedule schedules a function to run according to a Schedule.
func (t *SimpleTimer) ScheduleWithSchedule(schedule *models.Schedule, fn func()) (string, error) {
	if err := schedule.Validate(); err != nil {
		slog.Error("SimpleTimer ScheduleWithSchedule failed validation", "schedule", schedule, "error", err)
		return "", fmt.Errorf("invalid schedule: %w", err)
	}

	t.mu.Lock()
	t.nextID++
	id := fmt.Sprintf("recurring_%d", t.nextID)
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleWithSchedule", "id", id, "schedule", schedule.ToCronString())

	now := time.Now()

	// Calculate next run time based on schedule
	nextRun := t.calculateNextRun(schedule, now)
	if nextRun.IsZero() {
		return "", fmt.Errorf("cannot calculate next run time for schedule")
	}

	// Create a recurring timer function
	var scheduleNext func()
	scheduleNext = func() {
		slog.Debug("SimpleTimer executing recurring function", "id", id, "schedule", schedule.ToCronString())

		// Execute the user function
		fn()

		// Reschedule the next execution
		t.mu.Lock()
		if entry, exists := t.timers[id]; exists {
			entry.nextRun = t.calculateNextRun(schedule, time.Now())
			delay := time.Until(entry.nextRun)
			if delay > 0 {
				entry.timer = time.AfterFunc(delay, scheduleNext)
				slog.Debug("SimpleTimer rescheduled", "id", id, "nextRun", entry.nextRun)
			} else {
				slog.Warn("SimpleTimer cannot reschedule, next run time is in the past", "id", id)
			}
		}
		t.mu.Unlock()
	}

	// Schedule the first execution
	delay := time.Until(nextRun)
	if delay <= 0 {
		delay = time.Minute // Default to 1 minute if calculation failed
	}
	timer := time.AfterFunc(delay, scheduleNext)

	t.mu.Lock()
	t.timers[id] = &timerEntry{
		timer:       timer,
		timerType:   "recurring",
		scheduledAt: now,
		schedule:    schedule,
		nextRun:     nextRun,
		description: fmt.Sprintf("Recurring timer with schedule %s", schedule.String()),
	}
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleWithSchedule succeeded", "id", id, "schedule", schedule.ToCronString(), "nextRun", nextRun)
	return id, nil
}

// Cancel cancels a scheduled function by ID.
func (t *SimpleTimer) Cancel(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry, exists := t.timers[id]; exists {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(t.timers, id)
		slog.Debug("SimpleTimer Cancel succeeded", "id", id, "type", entry.timerType)
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
		if entry.timer != nil {
			entry.timer.Stop()
		}
		slog.Debug("SimpleTimer stopped timer", "id", id, "type", entry.timerType)
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
		info := models.TimerInfo{
			ID:          id,
			Type:        entry.timerType,
			ScheduledAt: entry.scheduledAt,
			Description: entry.description,
		}

		switch entry.timerType {
		case "once":
			remaining := entry.expiresAt.Sub(now)
			if remaining < 0 {
				remaining = 0
			}
			info.ExpiresAt = entry.expiresAt
			info.Remaining = remaining.String()
		case "recurring":
			info.Schedule = entry.schedule
			info.NextRun = entry.nextRun
			remaining := entry.nextRun.Sub(now)
			if remaining < 0 {
				remaining = 0
			}
			info.Remaining = remaining.String()
		}

		result = append(result, info)
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
	info := &models.TimerInfo{
		ID:          id,
		Type:        entry.timerType,
		ScheduledAt: entry.scheduledAt,
		Description: entry.description,
	}

	switch entry.timerType {
	case "once":
		remaining := entry.expiresAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		info.ExpiresAt = entry.expiresAt
		info.Remaining = remaining.String()
	case "recurring":
		info.Schedule = entry.schedule
		info.NextRun = entry.nextRun
		remaining := entry.nextRun.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		info.Remaining = remaining.String()
	}

	slog.Debug("SimpleTimer GetTimer", "id", id, "type", entry.timerType)
	return info, nil
}

// calculateNextRun calculates the next execution time for a given schedule
func (t *SimpleTimer) calculateNextRun(schedule *models.Schedule, from time.Time) time.Time {
	// Load timezone if specified
	loc := time.UTC
	if schedule.Timezone != "" {
		if tz, err := time.LoadLocation(schedule.Timezone); err == nil {
			loc = tz
		}
	}

	// Convert to target timezone
	fromLocal := from.In(loc)

	// Start with tomorrow to ensure we find the next occurrence
	next := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)

	// Try up to 366 days to find next match (covers leap years)
	for i := 0; i < 366; i++ {
		if t.scheduleMatches(schedule, next) {
			// Set the time based on schedule
			hour := 0
			if schedule.Hour != nil {
				hour = *schedule.Hour
			}
			minute := 0
			if schedule.Minute != nil {
				minute = *schedule.Minute
			}

			result := time.Date(next.Year(), next.Month(), next.Day(), hour, minute, 0, 0, loc)

			// Make sure it's after the from time
			if result.After(from) {
				return result
			}
		}
		next = next.Add(24 * time.Hour)
	}

	// Fallback to 1 hour from now if no match found
	return from.Add(time.Hour)
}

// scheduleMatches checks if a given time matches all the schedule constraints
func (t *SimpleTimer) scheduleMatches(schedule *models.Schedule, when time.Time) bool {
	// Check month
	if schedule.Month != nil && when.Month() != time.Month(*schedule.Month) {
		return false
	}

	// Check day of month
	if schedule.Day != nil && when.Day() != *schedule.Day {
		return false
	}

	// Check weekday (0=Sunday, 1=Monday, etc.)
	if schedule.Weekday != nil && int(when.Weekday()) != *schedule.Weekday {
		return false
	}

	return true
}
