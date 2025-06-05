// Package store provides storage backends for PromptPipe.
//
// This file defines the common interfaces and option types used by all store implementations.
package store

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Store defines the interface for storing receipts and responses.
type Store interface {
	AddReceipt(r models.Receipt) error
	GetReceipts() ([]models.Receipt, error)
	AddResponse(r models.Response) error
	GetResponses() ([]models.Response, error)
	ClearReceipts() error // for tests
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
	if strings.Contains(dsn, "postgres://") || strings.Contains(dsn, "host=") || strings.Count(dsn, "=") > 0 {
		return "postgres"
	}
	return "sqlite3"
}

// InMemoryStore is a simple in-memory implementation of the Store interface.
// Data is stored in memory and will be lost when the application restarts.
type InMemoryStore struct {
	receipts  []models.Receipt
	responses []models.Response
	mu        sync.RWMutex
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	slog.Debug("Creating new in-memory store")
	return &InMemoryStore{
		receipts:  make([]models.Receipt, 0),
		responses: make([]models.Response, 0),
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
