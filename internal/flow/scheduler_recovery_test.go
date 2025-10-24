package flow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestSchedulerTool_RecoverPendingReminders(t *testing.T) {
	// Setup
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()

	tool := newSchedulerToolForTest(timer, msgService, stateManager)
	tool.SetDailyPromptReminderDelay(4 * time.Hour)

	ctx := context.Background()
	participantID := "test-recovery-participant"

	// Create a pending reminder that should have fired 1 hour ago (overdue)
	overdueReminder := dailyPromptPendingState{
		SentAt:        time.Now().Add(-5 * time.Hour).Format(time.RFC3339),
		To:            "+15551234567",
		ReminderDueAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	overdueJSON, _ := json.Marshal(overdueReminder)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, string(overdueJSON))

	// Create a future reminder that should fire in 2 hours
	futureParticipantID := "test-future-reminder"
	futureReminder := dailyPromptPendingState{
		SentAt:        time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		To:            "+15559876543",
		ReminderDueAt: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	}
	futureJSON, _ := json.Marshal(futureReminder)
	stateManager.SetStateData(ctx, futureParticipantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, string(futureJSON))

	// Create a participant with no pending reminder
	noReminderParticipantID := "test-no-reminder"

	// Test recovery
	participantIDs := []string{participantID, futureParticipantID, noReminderParticipantID}
	err := tool.RecoverPendingReminders(ctx, participantIDs)

	// Verify
	if err != nil {
		t.Errorf("RecoverPendingReminders returned error: %v", err)
	}

	// Check that timers were scheduled
	if len(timer.scheduledCalls) != 2 {
		t.Errorf("Expected 2 timers to be scheduled, got %d", len(timer.scheduledCalls))
	}

	// Check that overdue reminder was scheduled with short delay
	overdueTimerID, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID)
	if err != nil || overdueTimerID == "" {
		t.Errorf("Expected timer ID to be stored for overdue reminder")
	}

	// Check that overdue reminder has short delay (should be ~5 seconds)
	if len(timer.scheduledCalls) > 0 && timer.scheduledCalls[0].Delay != nil {
		if *timer.scheduledCalls[0].Delay > 10*time.Second {
			t.Errorf("Expected overdue reminder to have short delay, got %v", *timer.scheduledCalls[0].Delay)
		}
	}

	// Check that future reminder was scheduled with correct remaining delay
	futureTimerID, err := stateManager.GetStateData(ctx, futureParticipantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID)
	if err != nil || futureTimerID == "" {
		t.Errorf("Expected timer ID to be stored for future reminder")
	}

	// Check future reminder delay (should be approximately 2 hours)
	if len(timer.scheduledCalls) > 1 && timer.scheduledCalls[1].Delay != nil {
		delay := *timer.scheduledCalls[1].Delay
		// Should be approximately 2 hours (with some tolerance for test execution time)
		if delay < 1*time.Hour+50*time.Minute || delay > 2*time.Hour+10*time.Minute {
			t.Errorf("Expected future reminder to have ~2 hour delay, got %v", delay)
		}
	}

	// Check that no timer was created for participant without pending reminder
	noReminderTimerID, _ := stateManager.GetStateData(ctx, noReminderParticipantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID)
	if noReminderTimerID != "" {
		t.Errorf("Expected no timer ID for participant without pending reminder, got %s", noReminderTimerID)
	}
}

func TestSchedulerTool_RecoverPendingReminders_DisabledDelay(t *testing.T) {
	// Setup with reminder delay disabled
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()

	tool := newSchedulerToolForTest(timer, msgService, stateManager)
	tool.SetDailyPromptReminderDelay(0) // Disable reminders

	ctx := context.Background()
	participantID := "test-disabled-recovery"

	// Create a pending reminder
	reminder := dailyPromptPendingState{
		SentAt:        time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		To:            "+15551111111",
		ReminderDueAt: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	}
	reminderJSON, _ := json.Marshal(reminder)
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, string(reminderJSON))

	// Test recovery
	err := tool.RecoverPendingReminders(ctx, []string{participantID})

	// Should not error, but should skip recovery
	if err != nil {
		t.Errorf("RecoverPendingReminders with disabled delay returned error: %v", err)
	}

	// No timers should be scheduled
	if len(timer.scheduledCalls) != 0 {
		t.Errorf("Expected no timers to be scheduled when delay is disabled, got %d", len(timer.scheduledCalls))
	}
}

func TestSchedulerTool_RecoverPendingReminders_InvalidState(t *testing.T) {
	// Setup
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()

	tool := newSchedulerToolForTest(timer, msgService, stateManager)
	tool.SetDailyPromptReminderDelay(4 * time.Hour)

	ctx := context.Background()
	participantID := "test-invalid-state"

	// Create invalid JSON in pending state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending, "{invalid json}")

	// Test recovery - should not fail, just skip invalid state
	err := tool.RecoverPendingReminders(ctx, []string{participantID})

	// Should complete with errors noted but not fail
	if err == nil {
		t.Log("RecoverPendingReminders handled invalid state gracefully")
	}

	// No timer should be scheduled for invalid state
	if len(timer.scheduledCalls) != 0 {
		t.Errorf("Expected no timers to be scheduled for invalid state, got %d", len(timer.scheduledCalls))
	}
}
