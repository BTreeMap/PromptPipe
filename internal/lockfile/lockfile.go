// Package lockfile provides directory-based locking to prevent multiple PromptPipe instances.
//
// This package implements a robust file locking mechanism using syscall-level locks
// that are automatically released when the process exits (gracefully or not).
package lockfile

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// LockFileName is the name of the lock file created in the state directory
const LockFileName = "promptpipe.lock"

// Lock represents an active directory lock
type Lock struct {
	file     *os.File
	path     string
	acquired bool
}

// AcquireLock attempts to acquire an exclusive lock on the state directory.
// Returns a Lock instance if successful, or an error with detailed information
// about the conflicting process if the lock is already held.
func AcquireLock(stateDir string) (*Lock, error) {
	lockPath := filepath.Join(stateDir, LockFileName)

	slog.Debug("Attempting to acquire lock", "lock_path", lockPath, "state_dir", stateDir)

	// Ensure the state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		slog.Error("Failed to create state directory for lock", "error", err, "state_dir", stateDir)
		return nil, fmt.Errorf("failed to create state directory %s: %w", stateDir, err)
	}

	// Open the lock file for writing (create if it doesn't exist)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		slog.Error("Failed to open lock file", "error", err, "lock_path", lockPath)
		return nil, fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}

	// Attempt to acquire an exclusive lock using flock
	// This will fail immediately if another process holds the lock
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// Close the file since we couldn't acquire the lock
		file.Close()

		// Try to read the existing lock file to provide helpful error information
		lockInfo := readExistingLockInfo(lockPath)

		slog.Error("Failed to acquire lock - another PromptPipe instance is running",
			"error", err, "lock_path", lockPath, "existing_lock_info", lockInfo)

		return nil, &LockError{
			LockPath:     lockPath,
			ExistingInfo: lockInfo,
			Cause:        err,
		}
	}

	// Write our process information to the lock file
	lockInfo := fmt.Sprintf("pid=%d\n", os.Getpid())
	if _, err := file.WriteString(lockInfo); err != nil {
		// If we can't write to the lock file, release the lock and fail
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()

		slog.Error("Failed to write lock information", "error", err, "lock_path", lockPath)
		return nil, fmt.Errorf("failed to write lock information to %s: %w", lockPath, err)
	}

	// Sync the file to ensure the lock information is written to disk
	if err := file.Sync(); err != nil {
		slog.Warn("Failed to sync lock file", "error", err, "lock_path", lockPath)
		// Continue anyway - this is not critical
	}

	lock := &Lock{
		file:     file,
		path:     lockPath,
		acquired: true,
	}

	slog.Info("Successfully acquired state directory lock", "lock_path", lockPath, "pid", os.Getpid())
	return lock, nil
}

// Release releases the lock and removes the lock file.
// This method is safe to call multiple times.
func (l *Lock) Release() error {
	if !l.acquired || l.file == nil {
		slog.Debug("Lock already released or not acquired", "lock_path", l.path)
		return nil
	}

	slog.Debug("Releasing lock", "lock_path", l.path, "pid", os.Getpid())

	// Release the flock first
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		slog.Error("Failed to release flock", "error", err, "lock_path", l.path)
		// Continue anyway to clean up the file
	}

	// Close the file
	if err := l.file.Close(); err != nil {
		slog.Error("Failed to close lock file", "error", err, "lock_path", l.path)
		// Continue anyway to remove the file
	}

	// Remove the lock file
	if err := os.Remove(l.path); err != nil {
		slog.Error("Failed to remove lock file", "error", err, "lock_path", l.path)
		// This is not critical - the flock has been released
	}

	l.acquired = false
	l.file = nil

	slog.Info("Successfully released state directory lock", "lock_path", l.path)
	return nil
}

// LockError represents an error when failing to acquire a lock due to another process
type LockError struct {
	LockPath     string
	ExistingInfo string
	Cause        error
}

func (e *LockError) Error() string {
	baseMsg := fmt.Sprintf("Another PromptPipe instance is already running using the same state directory.\n\nLock file: %s", e.LockPath)

	if e.ExistingInfo != "" {
		baseMsg += fmt.Sprintf("\nExisting process: %s", e.ExistingInfo)
	}

	baseMsg += "\n\nIf you're certain no other PromptPipe instance is running, the lock file may be stale.\n" +
		"You can manually remove it with:\n" +
		fmt.Sprintf("  rm %s", e.LockPath) +
		"\n\nWARNING: Only remove the lock file if you're absolutely sure no other PromptPipe instance is running,\n" +
		"as this could lead to data corruption if multiple instances access the same state directory."

	return baseMsg
}

func (e *LockError) Unwrap() error {
	return e.Cause
}

// readExistingLockInfo attempts to read information from an existing lock file
// to provide helpful error messages. Returns empty string if unable to read.
func readExistingLockInfo(lockPath string) string {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "unable to read lock file information"
	}

	content := string(data)
	if content == "" {
		return "lock file exists but contains no process information"
	}

	// Try to extract PID and check if the process is still running
	if pid := extractPIDFromLockInfo(content); pid > 0 {
		if isProcessRunning(pid) {
			return fmt.Sprintf("PID %d (running)", pid)
		} else {
			return fmt.Sprintf("PID %d (not running - stale lock)", pid)
		}
	}

	return fmt.Sprintf("process information: %s", content)
}

// extractPIDFromLockInfo attempts to extract a PID from lock file content
func extractPIDFromLockInfo(content string) int {
	// Look for "pid=NNNN" pattern
	const pidPrefix = "pid="
	if idx := strings.Index(content, pidPrefix); idx != -1 {
		start := idx + len(pidPrefix)
		end := start
		for end < len(content) && content[end] >= '0' && content[end] <= '9' {
			end++
		}
		if end > start {
			if pid, err := strconv.Atoi(content[start:end]); err == nil {
				return pid
			}
		}
	}
	return 0
}

// isProcessRunning checks if a process with the given PID is currently running
func isProcessRunning(pid int) bool {
	// On Unix systems, we can send signal 0 to check if a process exists
	// without actually sending a signal to it
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 is a special case - it checks if we can send a signal to the process
	// without actually sending one. If the process doesn't exist, we get an error.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
