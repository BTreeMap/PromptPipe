// Package store provides storage backends for PromptPipe.
//
// This file defines the common interfaces and option types used by all store implementations.
package store

import (
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Store defines the interface for storing receipts, responses, and flow state.
type Store interface {
	AddReceipt(r models.Receipt) error
	GetReceipts() ([]models.Receipt, error)
	AddResponse(r models.Response) error
	GetResponses() ([]models.Response, error)
	ClearReceipts() error  // for tests
	ClearResponses() error // for tests
	Close() error          // for proper resource cleanup
	// Flow state management
	SaveFlowState(state models.FlowState) error
	GetFlowState(participantID, flowType string) (*models.FlowState, error)
	DeleteFlowState(participantID, flowType string) error
}

// Opts holds configuration options for store implementations.
type Opts struct {
	DSN string // Database connection string or file path for SQLite
}

// Option defines a configuration option for store implementations.
type Option func(*Opts)

// WithPostgresDSN sets the PostgreSQL database connection string.
func WithPostgresDSN(dsn string) Option {
	return func(o *Opts) {
		o.DSN = dsn
	}
}

// WithSQLiteDSN sets the SQLite database file path.
func WithSQLiteDSN(dsn string) Option {
	return func(o *Opts) {
		o.DSN = dsn
	}
}

// DetectDSNType analyzes a DSN and returns the appropriate database driver.
// Returns "postgres" for PostgreSQL DSNs, "sqlite3" for SQLite file paths.
func DetectDSNType(dsn string) string {
	if strings.HasPrefix(dsn, "file:") || strings.Contains(dsn, ".db") || strings.Contains(dsn, ".sqlite") || strings.Contains(dsn, ".sqlite3") {
		return "sqlite3"
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.Contains(dsn, "host=") || strings.Count(dsn, "=") > 0 {
		return "postgres"
	}
	return "sqlite3"
}

// ExtractDirFromSQLiteDSN extracts the directory path from a SQLite DSN string, handling both
// file URIs (e.g., "file:/path/to/file?_foreign_keys=on") and regular file paths.
// This function is specifically designed for SQLite DSNs and will return an error
// if called with non-SQLite DSNs (e.g., PostgreSQL DSNs).
// Returns the directory containing the SQLite database file, or an error if:
// - The DSN is not a SQLite DSN
// - The file URI cannot be parsed
// - The resulting path is invalid
func ExtractDirFromSQLiteDSN(dsn string) (string, error) {
	// Ensure this is only used with SQLite DSNs
	if DetectDSNType(dsn) != "sqlite3" {
		return "", fmt.Errorf("ExtractDirFromSQLiteDSN can only be used with SQLite DSNs, got: %s", dsn)
	}

	// Extract file path from DSN, handling file:// URI scheme
	dbPath := dsn
	if strings.HasPrefix(dbPath, "file:") {
		// Parse as URL to properly handle file:// scheme and query parameters
		parsedURL, err := url.Parse(dbPath)
		if err != nil {
			return "", fmt.Errorf("failed to parse SQLite file URI '%s': %w", dsn, err)
		}
		dbPath = parsedURL.Path
	}

	// Validate that we have a valid path
	if dbPath == "" {
		return "", fmt.Errorf("invalid SQLite DSN: empty path after parsing '%s'", dsn)
	}

	// Get directory path
	dir := filepath.Dir(dbPath)

	// Return empty string for current directory to avoid unnecessary creation
	if dir == "" || dir == "." {
		return "", nil
	}

	return dir, nil
}

// InMemoryStore is a simple in-memory implementation of the Store interface.
// Data is stored in memory and will be lost when the application restarts.
type InMemoryStore struct {
	receipts   []models.Receipt
	responses  []models.Response
	flowStates map[string]models.FlowState // key: participantID_flowType
	mu         sync.RWMutex
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	slog.Debug("Creating new in-memory store")
	return &InMemoryStore{
		receipts:   make([]models.Receipt, 0),
		responses:  make([]models.Response, 0),
		flowStates: make(map[string]models.FlowState),
	}
}

// AddReceipt stores a receipt in memory.
func (s *InMemoryStore) AddReceipt(r models.Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receipts = append(s.receipts, r)
	slog.Debug("InMemoryStore AddReceipt succeeded", "to", r.To, "status", r.Status)
	return nil
}

// GetReceipts retrieves all stored receipts from memory.
func (s *InMemoryStore) GetReceipts() ([]models.Receipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]models.Receipt, len(s.receipts))
	copy(result, s.receipts)
	slog.Debug("InMemoryStore GetReceipts succeeded", "count", len(result))
	return result, nil
}

// AddResponse stores an incoming response in memory.
func (s *InMemoryStore) AddResponse(r models.Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, r)
	slog.Debug("InMemoryStore AddResponse succeeded", "from", r.From)
	return nil
}

// GetResponses retrieves all stored responses from memory.
func (s *InMemoryStore) GetResponses() ([]models.Response, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]models.Response, len(s.responses))
	copy(result, s.responses)
	slog.Debug("InMemoryStore GetResponses succeeded", "count", len(result))
	return result, nil
}

// ClearReceipts clears all stored receipts (for tests).
func (s *InMemoryStore) ClearReceipts() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receipts = s.receipts[:0] // Clear slice efficiently
	slog.Debug("InMemoryStore ClearReceipts succeeded")
	return nil
}

// ClearResponses clears all stored responses (for tests).
func (s *InMemoryStore) ClearResponses() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = s.responses[:0] // Clear slice efficiently
	slog.Debug("InMemoryStore ClearResponses succeeded")
	return nil
}

// Close is a no-op for in-memory store as there are no resources to clean up.
func (s *InMemoryStore) Close() error {
	slog.Debug("InMemoryStore Close called (no-op)")
	return nil
}

// SaveFlowState stores or updates flow state for a participant.
func (s *InMemoryStore) SaveFlowState(state models.FlowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := state.ParticipantID + "_" + state.FlowType
	s.flowStates[key] = state
	slog.Debug("InMemoryStore SaveFlowState succeeded", "participantID", state.ParticipantID, "flowType", state.FlowType, "state", state.CurrentState)
	return nil
}

// GetFlowState retrieves flow state for a participant.
func (s *InMemoryStore) GetFlowState(participantID, flowType string) (*models.FlowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := participantID + "_" + flowType
	if state, exists := s.flowStates[key]; exists {
		slog.Debug("InMemoryStore GetFlowState found", "participantID", participantID, "flowType", flowType, "state", state.CurrentState)
		// Return a copy to prevent external modifications
		stateCopy := state
		return &stateCopy, nil
	}
	slog.Debug("InMemoryStore GetFlowState not found", "participantID", participantID, "flowType", flowType)
	return nil, nil
}

// DeleteFlowState removes flow state for a participant.
func (s *InMemoryStore) DeleteFlowState(participantID, flowType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := participantID + "_" + flowType
	delete(s.flowStates, key)
	slog.Debug("InMemoryStore DeleteFlowState succeeded", "participantID", participantID, "flowType", flowType)
	return nil
}
