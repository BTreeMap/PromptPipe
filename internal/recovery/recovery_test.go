package recovery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// Mock timer for testing
type mockTimer struct {
	scheduledTimers map[string]time.Duration
	scheduleError   error
}

func (m *mockTimer) ScheduleAfter(delay time.Duration, fn func()) (string, error) {
	if m.scheduleError != nil {
		return "", m.scheduleError
	}
	timerID := fmt.Sprintf("timer-%d", len(m.scheduledTimers))
	m.scheduledTimers[timerID] = delay
	return timerID, nil
}

func (m *mockTimer) ScheduleAt(when time.Time, fn func()) (string, error) {
	if m.scheduleError != nil {
		return "", m.scheduleError
	}
	timerID := fmt.Sprintf("timer-%d", len(m.scheduledTimers))
	m.scheduledTimers[timerID] = time.Until(when)
	return timerID, nil
}

func (m *mockTimer) ScheduleWithSchedule(schedule *models.Schedule, fn func()) (string, error) {
	if m.scheduleError != nil {
		return "", m.scheduleError
	}
	timerID := fmt.Sprintf("timer-%d", len(m.scheduledTimers))
	m.scheduledTimers[timerID] = 0
	return timerID, nil
}

func (m *mockTimer) Cancel(id string) error {
	delete(m.scheduledTimers, id)
	return nil
}

func (m *mockTimer) Stop() {}

func (m *mockTimer) ListActive() []models.TimerInfo {
	return nil
}

func (m *mockTimer) GetTimer(id string) (*models.TimerInfo, error) {
	return nil, nil
}

// Mock recoverable for testing
type mockRecoverable struct {
	name          string
	recoverError  error
	recoverCalled bool
}

func (m *mockRecoverable) RecoverState(ctx context.Context, registry *RecoveryRegistry) error {
	m.recoverCalled = true
	return m.recoverError
}

func TestNewRecoveryRegistry(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}

	registry := NewRecoveryRegistry(store, timer)

	if registry == nil {
		t.Fatal("NewRecoveryRegistry returned nil")
	}

	if registry.GetStore() != store {
		t.Error("Registry store does not match provided store")
	}

	if registry.GetTimer() != timer {
		t.Error("Registry timer does not match provided timer")
	}
}

func TestRecoveryRegistry_RegisterTimerRecovery(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}
	registry := NewRecoveryRegistry(store, timer)

	callbackCalled := false
	callback := func(info TimerRecoveryInfo) (string, error) {
		callbackCalled = true
		return "test-timer-id", nil
	}

	registry.RegisterTimerRecovery(callback)

	info := TimerRecoveryInfo{
		ParticipantID: "test",
		FlowType:      models.FlowTypeConversation,
		OriginalTTL:   time.Hour,
		CreatedAt:     time.Now(),
	}
	_, err := registry.RecoverTimer(info)

	if err != nil {
		t.Errorf("RecoverTimer failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Timer recovery callback was not called")
	}
}

func TestRecoveryRegistry_RecoverTimer_NoCallback(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}
	registry := NewRecoveryRegistry(store, timer)

	info := TimerRecoveryInfo{
		ParticipantID: "test",
		FlowType:      models.FlowTypeConversation,
		OriginalTTL:   time.Hour,
		CreatedAt:     time.Now(),
	}
	_, err := registry.RecoverTimer(info)

	if err == nil {
		t.Error("Expected error when no timer recovery callback is registered")
	}
}

func TestNewRecoveryManager(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}

	manager := NewRecoveryManager(store, timer)

	if manager == nil {
		t.Fatal("NewRecoveryManager returned nil")
	}

	if manager.GetRegistry() == nil {
		t.Error("RecoveryManager registry is nil")
	}
}

func TestRecoveryManager_RecoverAll_Success(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}
	manager := NewRecoveryManager(store, timer)

	mock1 := &mockRecoverable{name: "mock1"}
	mock2 := &mockRecoverable{name: "mock2"}

	manager.RegisterRecoverable(mock1)
	manager.RegisterRecoverable(mock2)

	ctx := context.Background()
	err := manager.RecoverAll(ctx)

	if err != nil {
		t.Errorf("RecoverAll failed: %v", err)
	}

	if !mock1.recoverCalled {
		t.Error("mock1 RecoverState was not called")
	}

	if !mock2.recoverCalled {
		t.Error("mock2 RecoverState was not called")
	}
}

func TestRecoveryManager_RecoverAll_WithErrors(t *testing.T) {
	store := store.NewInMemoryStore()
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}
	manager := NewRecoveryManager(store, timer)

	mock1 := &mockRecoverable{name: "mock1", recoverError: fmt.Errorf("recovery failed")}
	mock2 := &mockRecoverable{name: "mock2"}

	manager.RegisterRecoverable(mock1)
	manager.RegisterRecoverable(mock2)

	ctx := context.Background()
	err := manager.RecoverAll(ctx)

	if err == nil {
		t.Error("Expected error from RecoverAll when components fail")
	}

	if !mock1.recoverCalled || !mock2.recoverCalled {
		t.Error("All recoverables should be called despite errors")
	}
}
