package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockAcquisition(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "lockfile_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test acquiring a lock
	lock, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	// Verify the lock file exists
	lockPath := filepath.Join(tempDir, LockFileName)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file was not created: %s", lockPath)
	}

	// Verify the lock file contains our PID
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}

	expectedContent := fmt.Sprintf("pid=%d\n", os.Getpid())
	if string(content) != expectedContent {
		t.Errorf("Lock file content mismatch. Expected: %q, Got: %q", expectedContent, string(content))
	}
}

func TestLockConflict(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "lockfile_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Acquire first lock
	lock1, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	defer lock1.Release()

	// Try to acquire second lock - should fail
	lock2, err := AcquireLock(tempDir)
	if err == nil {
		lock2.Release()
		t.Fatalf("Second lock acquisition should have failed")
	}

	// Verify it's a LockError
	var lockErr *LockError
	if !errors.As(err, &lockErr) {
		t.Errorf("Expected LockError, got: %T", err)
	}

	// Verify error message contains helpful information
	errMsg := err.Error()
	if !strings.Contains(errMsg, "Another PromptPipe instance is already running") {
		t.Errorf("Error message should mention another instance running: %s", errMsg)
	}
	if !strings.Contains(errMsg, tempDir) {
		t.Errorf("Error message should contain the lock path: %s", errMsg)
	}
}

func TestLockRelease(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "lockfile_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Acquire lock
	lock, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	lockPath := filepath.Join(tempDir, LockFileName)

	// Verify lock file exists
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file should exist before release: %s", lockPath)
	}

	// Release the lock
	if err := lock.Release(); err != nil {
		t.Errorf("Failed to release lock: %v", err)
	}

	// Verify lock file is removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("Lock file should be removed after release: %s", lockPath)
	}

	// Test multiple releases (should be safe)
	if err := lock.Release(); err != nil {
		t.Errorf("Multiple releases should be safe: %v", err)
	}
}

func TestLockReacquisition(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "lockfile_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Acquire and release first lock
	lock1, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	lock1.Release()

	// Should be able to acquire lock again
	lock2, err := AcquireLock(tempDir)
	if err != nil {
		t.Fatalf("Failed to reacquire lock after release: %v", err)
	}
	defer lock2.Release()
}

func TestExtractPIDFromLockInfo(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"valid pid", "pid=12345\n", 12345},
		{"pid with extra content", "pid=67890\nother=info", 67890},
		{"no pid", "other=info", 0},
		{"empty content", "", 0},
		{"invalid pid", "pid=abc", 0},
		{"no equals", "pid12345", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPIDFromLockInfo(tt.content)
			if result != tt.expected {
				t.Errorf("extractPIDFromLockInfo(%q) = %d, want %d", tt.content, result, tt.expected)
			}
		})
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Test with our own PID (should be running)
	ourPID := os.Getpid()
	if !isProcessRunning(ourPID) {
		t.Errorf("Our own process should be detected as running")
	}

	// Test with PID 1 (init process, should exist on Unix systems)
	if !isProcessRunning(1) {
		t.Logf("PID 1 not detected as running (might be expected in some environments)")
	}

	// Test with a very high PID that's unlikely to exist
	if isProcessRunning(999999) {
		t.Logf("High PID detected as running (unexpected but not necessarily wrong)")
	}
}

func TestNonExistentDirectory(t *testing.T) {
	// Try to acquire lock in a non-existent directory
	nonExistentDir := "/tmp/this_should_not_exist_" + fmt.Sprintf("%d", time.Now().UnixNano())

	lock, err := AcquireLock(nonExistentDir)
	if err != nil {
		t.Fatalf("Should be able to create directory and acquire lock: %v", err)
	}
	defer lock.Release()

	// Verify the directory was created
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		t.Errorf("Directory should have been created: %s", nonExistentDir)
	}

	// Clean up
	os.RemoveAll(nonExistentDir)
}
