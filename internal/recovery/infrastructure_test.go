package recovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestTimerRecoveryHandler(t *testing.T) {
	timer := &mockTimer{scheduledTimers: make(map[string]time.Duration)}
	handler := TimerRecoveryHandler(timer)

	if handler == nil {
		t.Fatal("TimerRecoveryHandler returned nil")
	}

	info := TimerRecoveryInfo{
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeConversation,
		StateType:     models.StateConversationActive,
		DataKey:       models.DataKeyDailyPromptReminderTimerID,
		OriginalTTL:   2 * time.Hour,
		CreatedAt:     time.Now(),
	}

	timerID, err := handler(info)

	if err != nil {
		t.Errorf("TimerRecoveryHandler failed: %v", err)
	}

	if timerID == "" {
		t.Error("TimerRecoveryHandler returned empty timer ID")
	}

	if len(timer.scheduledTimers) != 1 {
		t.Errorf("Expected 1 timer to be scheduled, got %d", len(timer.scheduledTimers))
	}

	// Verify the timer was scheduled with the correct delay
	if delay, exists := timer.scheduledTimers[timerID]; exists {
		if delay != 2*time.Hour {
			t.Errorf("Expected timer delay of 2h, got %v", delay)
		}
	} else {
		t.Error("Timer ID not found in scheduled timers")
	}
}

func TestTimerRecoveryHandler_Error(t *testing.T) {
	timer := &mockTimer{
		scheduledTimers: make(map[string]time.Duration),
		scheduleError:   fmt.Errorf("timer scheduling failed"),
	}
	handler := TimerRecoveryHandler(timer)

	info := TimerRecoveryInfo{
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeConversation,
		OriginalTTL:   time.Hour,
		CreatedAt:     time.Now(),
	}

	_, err := handler(info)

	if err == nil {
		t.Error("Expected error from TimerRecoveryHandler when timer scheduling fails")
	}

	if err.Error() != "failed to schedule recovery timer: timer scheduling failed" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestCreateResponseHandlerRecoveryHandler(t *testing.T) {
	callbackCalled := false
	var receivedInfo ResponseHandlerRecoveryInfo

	callback := func(info ResponseHandlerRecoveryInfo) error {
		callbackCalled = true
		receivedInfo = info
		return nil
	}

	handler := CreateResponseHandlerRecoveryHandler(callback)

	if handler == nil {
		t.Fatal("CreateResponseHandlerRecoveryHandler returned nil")
	}

	info := ResponseHandlerRecoveryInfo{
		PhoneNumber:   "+15551234567",
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeConversation,
		HandlerType:   "conversation",
		TTL:           24 * time.Hour,
	}

	err := handler(info)

	if err != nil {
		t.Errorf("ResponseHandlerRecoveryHandler failed: %v", err)
	}

	if !callbackCalled {
		t.Error("Callback was not called")
	}

	if receivedInfo.PhoneNumber != info.PhoneNumber {
		t.Errorf("Expected phone number %s, got %s", info.PhoneNumber, receivedInfo.PhoneNumber)
	}

	if receivedInfo.ParticipantID != info.ParticipantID {
		t.Errorf("Expected participant ID %s, got %s", info.ParticipantID, receivedInfo.ParticipantID)
	}
}

func TestCreateResponseHandlerRecoveryHandler_NilCallback(t *testing.T) {
	handler := CreateResponseHandlerRecoveryHandler(nil)

	info := ResponseHandlerRecoveryInfo{
		PhoneNumber:   "+15551234567",
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeConversation,
		HandlerType:   "conversation",
		TTL:           24 * time.Hour,
	}

	err := handler(info)

	if err == nil {
		t.Error("Expected error when callback is nil")
	}

	if err.Error() != "no response handler recovery callback provided" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestCreateResponseHandlerRecoveryHandler_CallbackError(t *testing.T) {
	expectedError := fmt.Errorf("callback processing failed")

	callback := func(info ResponseHandlerRecoveryInfo) error {
		return expectedError
	}

	handler := CreateResponseHandlerRecoveryHandler(callback)

	info := ResponseHandlerRecoveryInfo{
		PhoneNumber:   "+15551234567",
		ParticipantID: "test-participant",
		FlowType:      models.FlowTypeConversation,
		HandlerType:   "conversation",
		TTL:           24 * time.Hour,
	}

	err := handler(info)

	if err == nil {
		t.Error("Expected error to be propagated from callback")
	}

	if err != expectedError {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}
