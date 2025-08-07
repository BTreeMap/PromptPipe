package flow

import (
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// NewMockStateManager creates a mock state manager for testing
func NewMockStateManager() StateManager {
	return NewStoreBasedStateManager(store.NewInMemoryStore())
}
