package flow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/store"
)

func newTestSQLiteStoreForFlow(t *testing.T) *store.SQLiteStore {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "flow_jobs_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	dbPath := filepath.Join(tempDir, "test.db")
	s, err := store.NewSQLiteStore(store.WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestDurableJobKindConstants verifies job kind constants are defined.
func TestDurableJobKindConstants(t *testing.T) {
	kinds := []string{
		JobKindStateTransition,
		JobKindFeedbackTimeout,
		JobKindFeedbackFollowup,
		JobKindDailyPromptReminder,
		JobKindAutoFeedbackEnforcement,
	}
	for _, k := range kinds {
		if k == "" {
			t.Error("Job kind constant is empty")
		}
	}
}

// TestStateTransitionPayloadSerialization tests JSON round-trip of payloads.
func TestStateTransitionPayloadSerialization(t *testing.T) {
	p := StateTransitionPayload{
		ParticipantID: "test-participant",
		TargetState:   "FEEDBACK",
		Reason:        "test reason",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var p2 StateTransitionPayload
	if err := json.Unmarshal(data, &p2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if p2.ParticipantID != p.ParticipantID {
		t.Errorf("Expected %q, got %q", p.ParticipantID, p2.ParticipantID)
	}
	if p2.TargetState != p.TargetState {
		t.Errorf("Expected %q, got %q", p.TargetState, p2.TargetState)
	}
	if p2.Reason != p.Reason {
		t.Errorf("Expected %q, got %q", p.Reason, p2.Reason)
	}
}

// TestAutoFeedbackEnforcementJobEnqueue verifies that when jobRepo is set,
// scheduleAutoFeedbackEnforcement enqueues a job instead of using timer.
func TestAutoFeedbackEnforcementJobEnqueue(t *testing.T) {
	s := newTestSQLiteStoreForFlow(t)
	stateManager := NewStoreBasedStateManager(s)

	// Create scheduler tool with no timer but with a jobRepo
	schedulerTool := &SchedulerTool{
		stateManager:        stateManager,
		jobRepo:             s,
		autoFeedbackEnabled: true,
	}

	ctx := context.Background()
	participantID := "test-participant-1"

	// Schedule auto feedback enforcement
	schedulerTool.scheduleAutoFeedbackEnforcement(ctx, participantID)

	// Check that a job was enqueued
	jobs, err := s.ClaimDueJobs(time.Now().Add(10*time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimDueJobs failed: %v", err)
	}

	found := false
	for _, j := range jobs {
		if j.Kind == JobKindAutoFeedbackEnforcement {
			found = true
			var payload AutoFeedbackEnforcementPayload
			if err := json.Unmarshal([]byte(j.PayloadJSON), &payload); err != nil {
				t.Fatalf("Failed to unmarshal payload: %v", err)
			}
			if payload.ParticipantID != participantID {
				t.Errorf("Expected participantID %q, got %q", participantID, payload.ParticipantID)
			}
		}
	}

	if !found {
		t.Error("Expected to find auto_feedback_enforcement job")
	}
}

// TestDailyPromptReminderJobEnqueue verifies that when jobRepo is set,
// scheduleDailyPromptReminder enqueues a job instead of using timer.
func TestDailyPromptReminderJobEnqueue(t *testing.T) {
	s := newTestSQLiteStoreForFlow(t)
	stateManager := NewStoreBasedStateManager(s)

	schedulerTool := &SchedulerTool{
		stateManager:             stateManager,
		jobRepo:                  s,
		dailyPromptReminderDelay: 5 * time.Hour,
	}

	// Need a mock messaging service
	schedulerTool.msgService = &mockMsgService{}

	ctx := context.Background()
	participantID := "test-participant-2"
	to := "+15551234567"

	schedulerTool.scheduleDailyPromptReminder(ctx, participantID, to)

	// Check that a job was enqueued
	jobs, err := s.ClaimDueJobs(time.Now().Add(6*time.Hour), 10)
	if err != nil {
		t.Fatalf("ClaimDueJobs failed: %v", err)
	}

	found := false
	for _, j := range jobs {
		if j.Kind == JobKindDailyPromptReminder {
			found = true
			var payload DailyPromptReminderPayload
			if err := json.Unmarshal([]byte(j.PayloadJSON), &payload); err != nil {
				t.Fatalf("Failed to unmarshal payload: %v", err)
			}
			if payload.ParticipantID != participantID {
				t.Errorf("Expected participantID %q, got %q", participantID, payload.ParticipantID)
			}
			if payload.To != to {
				t.Errorf("Expected to %q, got %q", to, payload.To)
			}
		}
	}

	if !found {
		t.Error("Expected to find daily_prompt_reminder job")
	}
}

// TestJobHandlerIdempotency verifies that job handlers are idempotent.
func TestJobHandlerIdempotency(t *testing.T) {
	s := newTestSQLiteStoreForFlow(t)

	var executionCount int32

	runner := store.NewJobRunner(s, 50*time.Millisecond)

	// Register a custom handler that tracks executions
	runner.RegisterHandler("test_idempotent", func(ctx context.Context, payload string) error {
		atomic.AddInt32(&executionCount, 1)
		return nil
	})

	// Enqueue a job due immediately with a dedupe key
	_, err := s.EnqueueJob("test_idempotent", time.Now().Add(-time.Second), `{"test":true}`, "idempotent-key")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	// Try to enqueue duplicate - should be deduped
	_, err = s.EnqueueJob("test_idempotent", time.Now().Add(-time.Second), `{"test":true}`, "idempotent-key")
	if err != nil {
		t.Fatalf("EnqueueJob duplicate failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go runner.Run(ctx)
	<-ctx.Done()

	if atomic.LoadInt32(&executionCount) != 1 {
		t.Errorf("Expected exactly 1 execution, got %d", atomic.LoadInt32(&executionCount))
	}
}

// mockMsgService is a minimal mock for testing flow tools.
type mockMsgService struct{}

func (m *mockMsgService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	return recipient, nil
}
func (m *mockMsgService) SendMessage(ctx context.Context, to, message string) error {
	return nil
}
func (m *mockMsgService) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	return nil
}
