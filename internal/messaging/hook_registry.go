// Package messaging provides hook registry for persistent hook management
package messaging

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// HookFactory defines a function that creates a response hook from parameters
type HookFactory func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error)

// HookRegistry manages the mapping of hook type names to factory functions
type HookRegistry struct {
	factories map[models.HookType]HookFactory
	mu        sync.RWMutex
}

// NewHookRegistry creates a new hook registry with default factories
func NewHookRegistry() *HookRegistry {
	registry := &HookRegistry{
		factories: make(map[models.HookType]HookFactory),
	}

	// Register default hook factories
	registry.registerDefaultFactories()

	return registry
}

// registerDefaultFactories registers the default hook factory functions
func (hr *HookRegistry) registerDefaultFactories() {
	// Conversation hook factory
	hr.factories[models.HookTypeConversation] = func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
		participantID, exists := params["participant_id"]
		if !exists {
			return nil, fmt.Errorf("missing required parameter: participant_id")
		}
		return CreateConversationHook(participantID, msgService), nil
	}

	// Static hook factory
	hr.factories[models.HookTypeStatic] = func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
		return CreateStaticHook(msgService), nil
	}

	// Branch hook factory
	hr.factories[models.HookTypeBranch] = func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
		// For branch hooks, we would need to reconstruct the branch options
		// This is more complex as we need to store the options in parameters
		// For now, return an error as branch hooks are temporary and shouldn't be persisted long-term
		return nil, fmt.Errorf("branch hooks are not supported for persistence - they are temporary prompt-specific hooks")
	}

	// GenAI hook factory
	hr.factories[models.HookTypeGenAI] = func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
		// Similar to branch hooks, GenAI hooks are typically prompt-specific
		// For now, return an error as they shouldn't be persisted long-term
		return nil, fmt.Errorf("genai hooks are not supported for persistence - they are temporary prompt-specific hooks")
	}

	slog.Debug("HookRegistry registered default factories", "count", len(hr.factories))
}

// RegisterFactory registers a custom hook factory function
func (hr *HookRegistry) RegisterFactory(hookType models.HookType, factory HookFactory) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.factories[hookType] = factory
	slog.Debug("HookRegistry registered custom factory", "hookType", hookType)
}

// CreateHook creates a hook using the registered factory for the given type
func (hr *HookRegistry) CreateHook(hookType models.HookType, params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
	hr.mu.RLock()
	factory, exists := hr.factories[hookType]
	hr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no factory registered for hook type: %s", hookType)
	}

	hook, err := factory(params, stateManager, msgService, timer)
	if err != nil {
		return nil, fmt.Errorf("failed to create hook of type %s: %w", hookType, err)
	}

	slog.Debug("HookRegistry created hook successfully", "hookType", hookType)
	return hook, nil
}

// ListRegisteredTypes returns all registered hook types
func (hr *HookRegistry) ListRegisteredTypes() []models.HookType {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	types := make([]models.HookType, 0, len(hr.factories))
	for hookType := range hr.factories {
		types = append(types, hookType)
	}
	return types
}

// IsRegistered checks if a hook type has a registered factory
func (hr *HookRegistry) IsRegistered(hookType models.HookType) bool {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	_, exists := hr.factories[hookType]
	return exists
}
