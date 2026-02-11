package store

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "sqlite_job_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	dbPath := filepath.Join(tempDir, "test.db")
	s, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Job repo tests ---

func TestSQLiteStore_JobRepo_EnqueueAndGet(t *testing.T) {
	s := newTestSQLiteStore(t)

	runAt := time.Now().Add(time.Hour)
	id, err := s.EnqueueJob("test_kind", runAt, `{"key":"value"}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}
	if id == "" {
		t.Fatal("EnqueueJob returned empty ID")
	}

	job, err := s.GetJob(id)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if job == nil {
		t.Fatal("GetJob returned nil")
	}
	if job.Kind != "test_kind" {
		t.Errorf("Expected kind 'test_kind', got %q", job.Kind)
	}
	if job.Status != JobStatusQueued {
		t.Errorf("Expected status 'queued', got %q", job.Status)
	}
	if job.PayloadJSON != `{"key":"value"}` {
		t.Errorf("Expected payload, got %q", job.PayloadJSON)
	}
}

func TestSQLiteStore_JobRepo_DedupeKey(t *testing.T) {
	s := newTestSQLiteStore(t)

	runAt := time.Now().Add(time.Hour)
	id1, err := s.EnqueueJob("test_kind", runAt, `{}`, "unique-key-1")
	if err != nil {
		t.Fatalf("EnqueueJob 1 failed: %v", err)
	}

	// Same dedupe key should return existing ID
	id2, err := s.EnqueueJob("test_kind", runAt, `{}`, "unique-key-1")
	if err != nil {
		t.Fatalf("EnqueueJob 2 failed: %v", err)
	}
	if id2 != id1 {
		t.Errorf("Expected dedupe to return same ID %q, got %q", id1, id2)
	}

	// Different dedupe key should create new job
	id3, err := s.EnqueueJob("test_kind", runAt, `{}`, "unique-key-2")
	if err != nil {
		t.Fatalf("EnqueueJob 3 failed: %v", err)
	}
	if id3 == id1 {
		t.Error("Expected different ID for different dedupe key")
	}
}

func TestSQLiteStore_JobRepo_DedupeKeyAfterComplete(t *testing.T) {
	s := newTestSQLiteStore(t)

	runAt := time.Now().Add(time.Hour)
	id1, err := s.EnqueueJob("test_kind", runAt, `{}`, "reuse-key")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	// Complete the job
	if err := s.CompleteJob(id1); err != nil {
		t.Fatalf("CompleteJob failed: %v", err)
	}

	// Same dedupe key should now create a new job (since old one is done)
	id2, err := s.EnqueueJob("test_kind", runAt, `{}`, "reuse-key")
	if err != nil {
		t.Fatalf("EnqueueJob 2 failed: %v", err)
	}
	if id2 == id1 {
		t.Error("Expected new ID after completing old job with same dedupe key")
	}
}

func TestSQLiteStore_JobRepo_ClaimDueJobs(t *testing.T) {
	s := newTestSQLiteStore(t)

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	_, err := s.EnqueueJob("past_job", past, `{"when":"past"}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob past failed: %v", err)
	}
	_, err = s.EnqueueJob("future_job", future, `{"when":"future"}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob future failed: %v", err)
	}

	now := time.Now()
	jobs, err := s.ClaimDueJobs(now, 10)
	if err != nil {
		t.Fatalf("ClaimDueJobs failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("Expected 1 due job, got %d", len(jobs))
	}
	if jobs[0].Kind != "past_job" {
		t.Errorf("Expected kind 'past_job', got %q", jobs[0].Kind)
	}
	if jobs[0].Status != JobStatusRunning {
		t.Errorf("Expected status 'running', got %q", jobs[0].Status)
	}
}

func TestSQLiteStore_JobRepo_FailAndRetry(t *testing.T) {
	s := newTestSQLiteStore(t)

	past := time.Now().Add(-time.Minute)
	id, err := s.EnqueueJob("retry_job", past, `{}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	// Claim it
	jobs, err := s.ClaimDueJobs(time.Now(), 10)
	if err != nil {
		t.Fatalf("ClaimDueJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	// Fail it (attempt 1 of 3)
	nextRun := time.Now().Add(time.Minute)
	if err := s.FailJob(id, "transient error", nextRun); err != nil {
		t.Fatalf("FailJob failed: %v", err)
	}

	job, _ := s.GetJob(id)
	if job.Status != JobStatusQueued {
		t.Errorf("Expected status 'queued' after first failure, got %q", job.Status)
	}
	if job.Attempt != 1 {
		t.Errorf("Expected attempt 1, got %d", job.Attempt)
	}
	if job.LastError != "transient error" {
		t.Errorf("Expected error message, got %q", job.LastError)
	}
}

func TestSQLiteStore_JobRepo_FailMaxAttempts(t *testing.T) {
	s := newTestSQLiteStore(t)

	past := time.Now().Add(-time.Minute)
	id, err := s.EnqueueJob("fail_job", past, `{}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	nextRun := time.Now().Add(time.Minute)
	for i := 0; i < 3; i++ {
		// Claim
		s.ClaimDueJobs(time.Now(), 10)
		// Fail
		if err := s.FailJob(id, "persistent error", nextRun); err != nil {
			t.Fatalf("FailJob iteration %d failed: %v", i, err)
		}
	}

	job, _ := s.GetJob(id)
	if job.Status != JobStatusFailed {
		t.Errorf("Expected status 'failed' after max attempts, got %q", job.Status)
	}
	if job.Attempt != 3 {
		t.Errorf("Expected attempt 3, got %d", job.Attempt)
	}
}

func TestSQLiteStore_JobRepo_CancelJob(t *testing.T) {
	s := newTestSQLiteStore(t)

	id, err := s.EnqueueJob("cancel_job", time.Now().Add(time.Hour), `{}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	if err := s.CancelJob(id); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	job, _ := s.GetJob(id)
	if job.Status != JobStatusCanceled {
		t.Errorf("Expected status 'canceled', got %q", job.Status)
	}
}

func TestSQLiteStore_JobRepo_RequeueStale(t *testing.T) {
	s := newTestSQLiteStore(t)

	past := time.Now().Add(-time.Hour)
	_, err := s.EnqueueJob("stale_job", past, `{}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	// Claim it (marks as running)
	jobs, err := s.ClaimDueJobs(time.Now(), 10)
	if err != nil {
		t.Fatalf("ClaimDueJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(jobs))
	}

	// Requeue stale jobs (locked more than 1 minute ago)
	staleBefore := time.Now().Add(time.Minute) // Everything is stale
	n, err := s.RequeueStaleRunningJobs(staleBefore)
	if err != nil {
		t.Fatalf("RequeueStaleRunningJobs failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Expected 1 requeued, got %d", n)
	}

	// Verify it's back to queued
	job, _ := s.GetJob(jobs[0].ID)
	if job.Status != JobStatusQueued {
		t.Errorf("Expected status 'queued' after requeue, got %q", job.Status)
	}
}

// --- Outbox repo tests ---

func TestSQLiteStore_OutboxRepo_EnqueueAndClaim(t *testing.T) {
	s := newTestSQLiteStore(t)

	id, err := s.EnqueueOutboxMessage("participant-1", "prompt", `{"body":"Hello"}`, "")
	if err != nil {
		t.Fatalf("EnqueueOutboxMessage failed: %v", err)
	}
	if id == "" {
		t.Fatal("EnqueueOutboxMessage returned empty ID")
	}

	now := time.Now()
	msgs, err := s.ClaimDueOutboxMessages(now, 10)
	if err != nil {
		t.Fatalf("ClaimDueOutboxMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ParticipantID != "participant-1" {
		t.Errorf("Expected participant 'participant-1', got %q", msgs[0].ParticipantID)
	}
	if msgs[0].Status != OutboxStatusSending {
		t.Errorf("Expected status 'sending', got %q", msgs[0].Status)
	}
}

func TestSQLiteStore_OutboxRepo_DedupeKey(t *testing.T) {
	s := newTestSQLiteStore(t)

	id1, err := s.EnqueueOutboxMessage("p1", "prompt", `{}`, "dedupe-1")
	if err != nil {
		t.Fatalf("EnqueueOutboxMessage 1 failed: %v", err)
	}

	id2, err := s.EnqueueOutboxMessage("p1", "prompt", `{}`, "dedupe-1")
	if err != nil {
		t.Fatalf("EnqueueOutboxMessage 2 failed: %v", err)
	}
	if id2 != id1 {
		t.Errorf("Expected same ID for duplicate dedupe key, got %q and %q", id1, id2)
	}
}

func TestSQLiteStore_OutboxRepo_MarkSent(t *testing.T) {
	s := newTestSQLiteStore(t)

	id, _ := s.EnqueueOutboxMessage("p1", "prompt", `{}`, "")
	msgs, _ := s.ClaimDueOutboxMessages(time.Now(), 10)
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if err := s.MarkOutboxMessageSent(id); err != nil {
		t.Fatalf("MarkOutboxMessageSent failed: %v", err)
	}

	// Should not be claimable again
	msgs2, _ := s.ClaimDueOutboxMessages(time.Now(), 10)
	if len(msgs2) != 0 {
		t.Errorf("Expected 0 messages after sent, got %d", len(msgs2))
	}
}

func TestSQLiteStore_OutboxRepo_FailAndRetry(t *testing.T) {
	s := newTestSQLiteStore(t)

	id, _ := s.EnqueueOutboxMessage("p1", "prompt", `{}`, "")
	s.ClaimDueOutboxMessages(time.Now(), 10)

	nextAttempt := time.Now().Add(-time.Second) // Already due for retry
	if err := s.FailOutboxMessage(id, "send error", nextAttempt); err != nil {
		t.Fatalf("FailOutboxMessage failed: %v", err)
	}

	// Should be claimable again
	msgs, _ := s.ClaimDueOutboxMessages(time.Now(), 10)
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 retryable message, got %d", len(msgs))
	}
}

func TestSQLiteStore_OutboxRepo_RequeueStale(t *testing.T) {
	s := newTestSQLiteStore(t)

	s.EnqueueOutboxMessage("p1", "prompt", `{}`, "")
	s.ClaimDueOutboxMessages(time.Now(), 10)

	staleBefore := time.Now().Add(time.Minute)
	n, err := s.RequeueStaleSendingMessages(staleBefore)
	if err != nil {
		t.Fatalf("RequeueStaleSendingMessages failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Expected 1 requeued, got %d", n)
	}
}

// --- Dedup repo tests ---

func TestSQLiteStore_DedupRepo_Basic(t *testing.T) {
	s := newTestSQLiteStore(t)

	// Should not be a duplicate initially
	dup, err := s.IsDuplicate("msg-1")
	if err != nil {
		t.Fatalf("IsDuplicate failed: %v", err)
	}
	if dup {
		t.Error("Expected false for new message")
	}

	// Record it
	isNew, err := s.RecordInbound("msg-1", "participant-1")
	if err != nil {
		t.Fatalf("RecordInbound failed: %v", err)
	}
	if !isNew {
		t.Error("Expected isNew=true for first record")
	}

	// Should now be a duplicate
	dup, err = s.IsDuplicate("msg-1")
	if err != nil {
		t.Fatalf("IsDuplicate after record failed: %v", err)
	}
	if !dup {
		t.Error("Expected true for duplicate message")
	}

	// Record same message again
	isNew2, err := s.RecordInbound("msg-1", "participant-1")
	if err != nil {
		t.Fatalf("RecordInbound duplicate failed: %v", err)
	}
	if isNew2 {
		t.Error("Expected isNew=false for duplicate record")
	}
}

func TestSQLiteStore_DedupRepo_MarkProcessed(t *testing.T) {
	s := newTestSQLiteStore(t)

	s.RecordInbound("msg-2", "participant-2")
	if err := s.MarkProcessed("msg-2"); err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}
}

// --- JobRunner tests ---

func TestJobRunner_Basic(t *testing.T) {
	s := newTestSQLiteStore(t)

	runner := NewJobRunner(s, 50*time.Millisecond)

	var executed int32
	runner.RegisterHandler("test_kind", func(ctx context.Context, payload string) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})

	// Enqueue a job due immediately
	past := time.Now().Add(-time.Second)
	_, err := s.EnqueueJob("test_kind", past, `{"test":true}`, "")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go runner.Run(ctx)
	<-ctx.Done()

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("Expected 1 execution, got %d", atomic.LoadInt32(&executed))
	}
}

// --- OutboxSender tests ---

func TestOutboxSender_Basic(t *testing.T) {
	s := newTestSQLiteStore(t)

	var sent int32
	sendFunc := func(ctx context.Context, msg OutboxMessage) error {
		atomic.AddInt32(&sent, 1)
		return nil
	}

	sender := NewOutboxSender(s, sendFunc, 50*time.Millisecond)

	// Enqueue a message
	_, err := s.EnqueueOutboxMessage("p1", "prompt", `{"body":"Hello"}`, "")
	if err != nil {
		t.Fatalf("EnqueueOutboxMessage failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go sender.Run(ctx)
	<-ctx.Done()

	if atomic.LoadInt32(&sent) != 1 {
		t.Errorf("Expected 1 send, got %d", atomic.LoadInt32(&sent))
	}
}
