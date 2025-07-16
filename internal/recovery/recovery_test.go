package recovery

import (
	"context"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// MockTimer implements the Timer interface for testing
type MockTimer struct {
	scheduledCallbacks map[string]func()
	nextID             int
}

func NewMockTimer() *MockTimer {
	return &MockTimer{
		scheduledCallbacks: make(map[string]func()),
		nextID:             1,
	}
}

func (m *MockTimer) ScheduleAfter(duration time.Duration, callback func()) (string, error) {
	id := string(rune('0' + m.nextID))
	m.nextID++
	m.scheduledCallbacks[id] = callback
	return id, nil
}

func (m *MockTimer) ScheduleAt(timestamp time.Time, callback func()) (string, error) {
	return m.ScheduleAfter(0, callback)
}

func (m *MockTimer) ScheduleWithSchedule(schedule *models.Schedule, callback func()) (string, error) {
	return m.ScheduleAfter(0, callback)
}

func (m *MockTimer) Cancel(timerID string) error {
	delete(m.scheduledCallbacks, timerID)
	return nil
}

func (m *MockTimer) Stop() {
	m.scheduledCallbacks = make(map[string]func())
}

func (m *MockTimer) ListActive() []models.TimerInfo {
	var timers []models.TimerInfo
	for id := range m.scheduledCallbacks {
		timers = append(timers, models.TimerInfo{
			ID:          id,
			Type:        "mock",
			Description: "Mock timer for testing",
			NextRun:     time.Now(),
		})
	}
	return timers
}

func (m *MockTimer) GetTimer(id string) (*models.TimerInfo, error) {
	if _, exists := m.scheduledCallbacks[id]; exists {
		return &models.TimerInfo{
			ID:          id,
			Type:        "mock",
			Description: "Mock timer for testing",
			NextRun:     time.Now(),
		}, nil
	}
	return nil, nil
}

func (m *MockTimer) TriggerCallback(timerID string) {
	if callback, exists := m.scheduledCallbacks[timerID]; exists {
		callback()
	}
}

// MockRecoverable implements the Recoverable interface for testing
type MockRecoverable struct {
	name     string
	executed bool
}

func (m *MockRecoverable) RecoverState(ctx context.Context, registry *RecoveryRegistry) error {
	m.executed = true
	return nil
}

func TestRecoveryManager(t *testing.T) {
	// Create in-memory store and mock timer
	store := &store.InMemoryStore{}
	timer := NewMockTimer()

	// Create recovery manager
	manager := NewRecoveryManager(store, timer)

	// Register timer recovery callback
	timerRecoveryCallCount := 0
	manager.RegisterTimerRecovery(func(info TimerRecoveryInfo) (string, error) {
		timerRecoveryCallCount++
		return timer.ScheduleAfter(info.OriginalTTL, func() {
			// Mock timeout handler
		})
	})

	// Register response handler recovery callback
	handlerRecoveryCallCount := 0
	manager.RegisterHandlerRecovery(func(info ResponseHandlerRecoveryInfo) error {
		handlerRecoveryCallCount++
		return nil
	})

	// Register mock recoverable components
	mock1 := &MockRecoverable{name: "mock1"}
	mock2 := &MockRecoverable{name: "mock2"}
	manager.RegisterRecoverable(mock1)
	manager.RegisterRecoverable(mock2)

	// Test recovery execution
	ctx := context.Background()
	err := manager.RecoverAll(ctx)
	if err != nil {
		t.Fatalf("Recovery failed: %v", err)
	}

	// Verify all components were recovered
	if !mock1.executed {
		t.Error("mock1 was not executed")
	}
	if !mock2.executed {
		t.Error("mock2 was not executed")
	}

	// Test timer recovery functionality
	registry := manager.GetRegistry()
	timerInfo := TimerRecoveryInfo{
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeMicroHealthIntervention,
		StateType:     models.StateCommitmentPrompt,
		DataKey:       models.DataKeyCommitmentTimerID,
		OriginalTTL:   30 * time.Minute,
		CreatedAt:     time.Now(),
	}

	timerID, err := registry.RecoverTimer(timerInfo)
	if err != nil {
		t.Fatalf("Timer recovery failed: %v", err)
	}

	if timerID == "" {
		t.Error("Timer ID should not be empty")
	}

	if timerRecoveryCallCount != 1 {
		t.Errorf("Expected 1 timer recovery call, got %d", timerRecoveryCallCount)
	}

	// Test response handler recovery functionality
	handlerInfo := ResponseHandlerRecoveryInfo{
		PhoneNumber:   "+1234567890",
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeMicroHealthIntervention,
		HandlerType:   "intervention",
		TTL:           48 * time.Hour,
	}

	err = registry.RecoverResponseHandler(handlerInfo)
	if err != nil {
		t.Fatalf("Response handler recovery failed: %v", err)
	}

	if handlerRecoveryCallCount != 1 {
		t.Errorf("Expected 1 handler recovery call, got %d", handlerRecoveryCallCount)
	}
}

func TestRecoveryRegistryAccessors(t *testing.T) {
	store := &store.InMemoryStore{}
	timer := NewMockTimer()
	registry := NewRecoveryRegistry(store, timer)

	// Test store access
	if registry.GetStore() != store {
		t.Error("GetStore() should return the provided store")
	}

	// Test timer access
	if registry.GetTimer() != timer {
		t.Error("GetTimer() should return the provided timer")
	}
}

func TestRecoveryWithoutCallbacks(t *testing.T) {
	store := &store.InMemoryStore{}
	timer := NewMockTimer()
	registry := NewRecoveryRegistry(store, timer)

	// Test timer recovery without callback
	timerInfo := TimerRecoveryInfo{
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeMicroHealthIntervention,
		StateType:     models.StateCommitmentPrompt,
		DataKey:       models.DataKeyCommitmentTimerID,
		OriginalTTL:   30 * time.Minute,
		CreatedAt:     time.Now(),
	}

	_, err := registry.RecoverTimer(timerInfo)
	if err == nil {
		t.Error("Expected error when no timer recovery handler is registered")
	}

	// Test handler recovery without callback
	handlerInfo := ResponseHandlerRecoveryInfo{
		PhoneNumber:   "+1234567890",
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeMicroHealthIntervention,
		HandlerType:   "intervention",
		TTL:           48 * time.Hour,
	}

	err = registry.RecoverResponseHandler(handlerInfo)
	if err == nil {
		t.Error("Expected error when no response handler recovery is registered")
	}
}
