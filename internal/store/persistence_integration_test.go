package store

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestJobRunnerRestartRecovery simulates a crash-and-restart scenario.
// It enqueues a job, "crashes" (stops the runner), restarts with a new runner
// on the same DB, and verifies the job executes exactly once.
func TestJobRunnerRestartRecovery(t *testing.T) {
	// Create a shared temp dir for the database
	tempDir, err := os.MkdirTemp("", "restart_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	// Phase 1: Create store, enqueue a due job, start runner, then "crash"
	s1, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 1) failed: %v", err)
	}

	var executedPhase1 int32
	runner1 := NewJobRunner(s1, 50*time.Millisecond)
	runner1.RegisterHandler("restart_test", func(ctx context.Context, payload string) error {
		atomic.AddInt32(&executedPhase1, 1)
		return nil
	})

	// Enqueue a job that's due in the future (so it won't execute in phase 1)
	runAt := time.Now().Add(200 * time.Millisecond)
	jobID, err := s1.EnqueueJob("restart_test", runAt, `{"test":"restart"}`, "restart-dedup")
	if err != nil {
		t.Fatalf("EnqueueJob failed: %v", err)
	}

	// Start runner briefly and stop before job is due
	ctx1, cancel1 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	go runner1.Run(ctx1)
	<-ctx1.Done()
	cancel1()

	// Job should NOT have executed yet
	if atomic.LoadInt32(&executedPhase1) != 0 {
		t.Fatalf("Expected 0 executions in phase 1, got %d", atomic.LoadInt32(&executedPhase1))
	}

	// Close store ("crash")
	s1.Close()

	// Phase 2: Open new store on same DB, recover stale jobs, run again
	s2, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 2) failed: %v", err)
	}
	defer s2.Close()

	var executedPhase2 int32
	runner2 := NewJobRunner(s2, 50*time.Millisecond)
	runner2.RegisterHandler("restart_test", func(ctx context.Context, payload string) error {
		atomic.AddInt32(&executedPhase2, 1)
		return nil
	})

	// Recover stale jobs (simulates crash recovery on startup)
	if err := runner2.RecoverStaleJobs(); err != nil {
		t.Fatalf("RecoverStaleJobs failed: %v", err)
	}

	// Wait for the job to become due and execute
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	go runner2.Run(ctx2)
	<-ctx2.Done()

	// Job should have executed exactly once
	if atomic.LoadInt32(&executedPhase2) != 1 {
		t.Errorf("Expected 1 execution in phase 2, got %d", atomic.LoadInt32(&executedPhase2))
	}

	// Verify job is marked done
	job, err := s2.GetJob(jobID)
	if err != nil {
		t.Fatalf("GetJob after restart failed: %v", err)
	}
	if job.Status != JobStatusDone {
		t.Errorf("Expected job status 'done', got %q", job.Status)
	}
}

// TestOutboxSenderRestartRecovery simulates a crash-and-restart for outbox messages.
func TestOutboxSenderRestartRecovery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "outbox_restart_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	// Phase 1: Enqueue message, claim it (marking as "sending"), then "crash"
	s1, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 1) failed: %v", err)
	}

	_, err = s1.EnqueueOutboxMessage("participant-1", "prompt", `{"body":"Hello!"}`, "outbox-restart-dedup")
	if err != nil {
		t.Fatalf("EnqueueOutboxMessage failed: %v", err)
	}

	// Claim it (simulates being mid-send when crash happens)
	msgs, err := s1.ClaimDueOutboxMessages(time.Now(), 10)
	if err != nil {
		t.Fatalf("ClaimDueOutboxMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Status != OutboxStatusSending {
		t.Errorf("Expected status 'sending', got %q", msgs[0].Status)
	}

	// "Crash" without marking sent
	s1.Close()

	// Phase 2: Open new store, recover, verify it gets sent
	s2, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 2) failed: %v", err)
	}
	defer s2.Close()

	var sent int32
	sender2 := NewOutboxSender(s2, func(ctx context.Context, msg OutboxMessage) error {
		atomic.AddInt32(&sent, 1)
		return nil
	}, 50*time.Millisecond)

	// Recover stale sending messages
	// Use a custom stale threshold to immediately recover
	staleBefore := time.Now().Add(time.Minute) // Everything is stale
	n, err := s2.RequeueStaleSendingMessages(staleBefore)
	if err != nil {
		t.Fatalf("RequeueStaleSendingMessages failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Expected 1 message requeued, got %d", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go sender2.Run(ctx)
	<-ctx.Done()

	if atomic.LoadInt32(&sent) != 1 {
		t.Errorf("Expected 1 send after recovery, got %d", atomic.LoadInt32(&sent))
	}
}

// TestPersistenceProviderInterface verifies that SQLiteStore implements PersistenceProvider.
func TestPersistenceProviderInterface(t *testing.T) {
	s := newTestSQLiteStore(t)

	// Use Store interface to test type assertion
	var st Store = s
	pp, ok := st.(PersistenceProvider)
	if !ok {
		t.Fatal("SQLiteStore does not implement PersistenceProvider")
	}

	if pp.JobRepo() == nil {
		t.Error("JobRepo() returned nil")
	}
	if pp.OutboxRepo() == nil {
		t.Error("OutboxRepo() returned nil")
	}
	if pp.DedupRepo() == nil {
		t.Error("DedupRepo() returned nil")
	}
}

// TestDedupRepoRestartSafety verifies that dedup records survive a store restart.
func TestDedupRepoRestartSafety(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dedup_restart_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	// Phase 1: Record an inbound message
	s1, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 1) failed: %v", err)
	}

	isNew, err := s1.RecordInbound("msg-restart-1", "participant-1")
	if err != nil {
		t.Fatalf("RecordInbound failed: %v", err)
	}
	if !isNew {
		t.Error("Expected isNew=true for first record")
	}

	s1.Close()

	// Phase 2: Reopen and verify it's a duplicate
	s2, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore (phase 2) failed: %v", err)
	}
	defer s2.Close()

	isNew2, err := s2.RecordInbound("msg-restart-1", "participant-1")
	if err != nil {
		t.Fatalf("RecordInbound duplicate failed: %v", err)
	}
	if isNew2 {
		t.Error("Expected isNew=false for duplicate after restart")
	}

	dup, err := s2.IsDuplicate("msg-restart-1")
	if err != nil {
		t.Fatalf("IsDuplicate failed: %v", err)
	}
	if !dup {
		t.Error("Expected true for duplicate message after restart")
	}
}
