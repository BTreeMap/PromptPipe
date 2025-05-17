// Package store provides storage backends for PromptPipe.
//
// It includes an in-memory store for receipts and will support persistent storage (e.g., PostgreSQL).
package store

import (
	"sync"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Store defines the interface for storage operations related to receipts.
// This allows for different storage implementations (e.g., in-memory, PostgreSQL).
type Store interface {
	AddReceipt(r models.Receipt) error
	GetReceipts() ([]models.Receipt, error)
}

// InMemoryStore is a simple in-memory store for receipts, primarily for testing or simple deployments.
// It uses a mutex to handle concurrent access safely.
type InMemoryStore struct {
	mu       sync.RWMutex
	receipts []models.Receipt
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		receipts: make([]models.Receipt, 0),
	}
}

// AddReceipt adds a new receipt to the in-memory store.
func (s *InMemoryStore) AddReceipt(r models.Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receipts = append(s.receipts, r)
	return nil
}

// GetReceipts retrieves all receipts from the in-memory store.
func (s *InMemoryStore) GetReceipts() ([]models.Receipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copiedReceipts := make([]models.Receipt, len(s.receipts))
	copy(copiedReceipts, s.receipts)
	return copiedReceipts, nil
}
