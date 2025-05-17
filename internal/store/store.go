// Package store provides storage backends for PromptPipe.
//
// It includes an in-memory store for receipts and will support persistent storage (e.g., PostgreSQL).
package store

import "github.com/BTreeMap/PromptPipe/internal/models"

// InMemoryStore is a simple in-memory store for receipts

type InMemoryStore struct {
	receipts []models.Receipt
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

func (s *InMemoryStore) AddReceipt(r models.Receipt) {
	s.receipts = append(s.receipts, r)
}

func (s *InMemoryStore) GetReceipts() []models.Receipt {
	return s.receipts
}
