// Package store provides storage backends for PromptPipe.
//
// It includes an in-memory store for receipts and will support persistent storage (e.g., PostgreSQL).
package store

import (
	"sync"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Store defines the interface for storage operations related to receipts and responses.
// This allows for different storage implementations (e.g., in-memory, PostgreSQL).
type Store interface {
	AddReceipt(r models.Receipt) error
	GetReceipts() ([]models.Receipt, error)
	AddResponse(r models.Response) error
	GetResponses() ([]models.Response, error)
}

// InMemoryStore is a simple in-memory store for receipts and responses, primarily for testing or simple deployments.
// It uses a mutex to handle concurrent access safely.
type InMemoryStore struct {
	mu        sync.RWMutex
	receipts  []models.Receipt
	responses []models.Response
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		receipts:  make([]models.Receipt, 0),
		responses: make([]models.Response, 0),
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

// AddResponse adds a new response to the in-memory store.
func (s *InMemoryStore) AddResponse(r models.Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, r)
	return nil
}

// GetResponses retrieves all responses from the in-memory store.
func (s *InMemoryStore) GetResponses() ([]models.Response, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copiedResponses := make([]models.Response, len(s.responses))
	copy(copiedResponses, s.responses)
	return copiedResponses, nil
}
