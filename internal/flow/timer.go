// Package flow provides timer implementations for scheduled actions.
package flow

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// timerEntry tracks information about a scheduled timer
type timerEntry struct {
	timer       *time.Timer // For both one-time and recurring timers
	timerType   string      // "once" or "recurring" 
	scheduledAt time.Time
	expiresAt   time.Time   // For one-time timers
	interval    time.Duration // For recurring timers
	pattern     string      // Original pattern like "daily", "hourly", etc.
	nextRun     time.Time   // Next execution time
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

// ScheduleCron schedules a function to run according to a simple pattern.
// Supported patterns: "daily", "hourly", "every Xm" (minutes), "every Xh" (hours), "every Xs" (seconds)
func (t *SimpleTimer) ScheduleCron(pattern string, fn func()) (string, error) {
	interval, err := parseSchedulePattern(pattern)
	if err != nil {
		slog.Error("SimpleTimer ScheduleCron failed to parse pattern", "pattern", pattern, "error", err)
		return "", fmt.Errorf("failed to parse schedule pattern: %w", err)
	}

	t.mu.Lock()
	t.nextID++
	id := fmt.Sprintf("recurring_%d", t.nextID)
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleCron", "id", id, "pattern", pattern, "interval", interval)

	now := time.Now()
	nextRun := now.Add(interval)

	// Create a recurring timer function
	var scheduleNext func()
	scheduleNext = func() {
		slog.Debug("SimpleTimer executing recurring function", "id", id, "pattern", pattern)
		
		// Execute the user function
		fn()
		
		// Reschedule the next execution
		t.mu.Lock()
		if entry, exists := t.timers[id]; exists {
			entry.nextRun = time.Now().Add(interval)
			entry.timer = time.AfterFunc(interval, scheduleNext)
			slog.Debug("SimpleTimer rescheduled", "id", id, "nextRun", entry.nextRun)
		}
		t.mu.Unlock()
	}

	// Schedule the first execution
	timer := time.AfterFunc(interval, scheduleNext)

	t.mu.Lock()
	t.timers[id] = &timerEntry{
		timer:       timer,
		timerType:   "recurring",
		scheduledAt: now,
		interval:    interval,
		pattern:     pattern,
		nextRun:     nextRun,
		description: fmt.Sprintf("Recurring timer with pattern %s (every %v)", pattern, interval),
	}
	t.mu.Unlock()

	slog.Debug("SimpleTimer ScheduleCron succeeded", "id", id, "pattern", pattern, "interval", interval, "nextRun", nextRun)
	return id, nil
}

// parseSchedulePattern parses simple schedule patterns into durations
func parseSchedulePattern(pattern string) (time.Duration, error) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	
	switch pattern {
	case "daily":
		return 24 * time.Hour, nil
	case "hourly":
		return time.Hour, nil
	default:
		// Handle "every Xm", "every Xh", "every Xs" patterns
		if strings.HasPrefix(pattern, "every ") {
			timeSpec := strings.TrimPrefix(pattern, "every ")
			if len(timeSpec) < 2 {
				return 0, fmt.Errorf("invalid time specification: %s", timeSpec)
			}
			
			unit := timeSpec[len(timeSpec)-1:]
			valueStr := timeSpec[:len(timeSpec)-1]
			
			value, err := strconv.Atoi(valueStr)
			if err != nil {
				return 0, fmt.Errorf("invalid numeric value: %s", valueStr)
			}
			
			switch unit {
			case "s":
				return time.Duration(value) * time.Second, nil
			case "m":
				return time.Duration(value) * time.Minute, nil
			case "h":
				return time.Duration(value) * time.Hour, nil
			default:
				return 0, fmt.Errorf("invalid time unit: %s (use s, m, or h)", unit)
			}
		}
	}
	
	return 0, fmt.Errorf("unsupported schedule pattern: %s", pattern)
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

		if entry.timerType == "once" {
			remaining := entry.expiresAt.Sub(now)
			if remaining < 0 {
				remaining = 0
			}
			info.ExpiresAt = entry.expiresAt
			info.Remaining = remaining.String()
		} else if entry.timerType == "recurring" {
			info.CronExpr = entry.pattern
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

	if entry.timerType == "once" {
		remaining := entry.expiresAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		info.ExpiresAt = entry.expiresAt
		info.Remaining = remaining.String()
	} else if entry.timerType == "recurring" {
		info.CronExpr = entry.pattern
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
