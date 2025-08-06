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
	// Intervention participant management
	SaveInterventionParticipant(participant models.InterventionParticipant) error
	GetInterventionParticipant(id string) (*models.InterventionParticipant, error)
	GetInterventionParticipantByPhone(phoneNumber string) (*models.InterventionParticipant, error)
	ListInterventionParticipants() ([]models.InterventionParticipant, error)
	DeleteInterventionParticipant(id string) error
	// Intervention response management
	SaveInterventionResponse(response models.InterventionResponse) error
	GetInterventionResponses(participantID string) ([]models.InterventionResponse, error)
	ListAllInterventionResponses() ([]models.InterventionResponse, error)
	// Conversation participant management
	SaveConversationParticipant(participant models.ConversationParticipant) error
	GetConversationParticipant(id string) (*models.ConversationParticipant, error)
	GetConversationParticipantByPhone(phoneNumber string) (*models.ConversationParticipant, error)
	ListConversationParticipants() ([]models.ConversationParticipant, error)
	DeleteConversationParticipant(id string) error
	// Response hook persistence management
	SaveRegisteredHook(hook models.RegisteredHook) error
	GetRegisteredHook(phoneNumber string) (*models.RegisteredHook, error)
	ListRegisteredHooks() ([]models.RegisteredHook, error)
	DeleteRegisteredHook(phoneNumber string) error
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
	receipts                 []models.Receipt
	responses                []models.Response
	flowStates               map[string]models.FlowState               // key: participantID_flowType
	interventionParticipants map[string]models.InterventionParticipant // key: participant ID
	conversationParticipants map[string]models.ConversationParticipant // key: participant ID
	interventionResponses    []models.InterventionResponse             // chronological list
	registeredHooks          map[string]models.RegisteredHook          // key: phone number
	mu                       sync.RWMutex
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	slog.Debug("InMemoryStore.NewInMemoryStore: creating new in-memory store")
	return &InMemoryStore{
		receipts:                 make([]models.Receipt, 0),
		responses:                make([]models.Response, 0),
		flowStates:               make(map[string]models.FlowState),
		interventionParticipants: make(map[string]models.InterventionParticipant),
		conversationParticipants: make(map[string]models.ConversationParticipant),
		interventionResponses:    make([]models.InterventionResponse, 0),
		registeredHooks:          make(map[string]models.RegisteredHook),
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
	key := state.ParticipantID + "_" + string(state.FlowType)
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

// Intervention participant management methods - InMemory implementation

// SaveInterventionParticipant stores or updates an intervention participant.
func (s *InMemoryStore) SaveInterventionParticipant(participant models.InterventionParticipant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interventionParticipants[participant.ID] = participant
	slog.Debug("InMemoryStore SaveInterventionParticipant succeeded", "id", participant.ID, "phone", participant.PhoneNumber)
	return nil
}

// GetInterventionParticipant retrieves an intervention participant by ID.
func (s *InMemoryStore) GetInterventionParticipant(id string) (*models.InterventionParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if participant, exists := s.interventionParticipants[id]; exists {
		slog.Debug("InMemoryStore GetInterventionParticipant found", "id", id)
		// Return a copy to prevent external modifications
		participantCopy := participant
		return &participantCopy, nil
	}
	slog.Debug("InMemoryStore GetInterventionParticipant not found", "id", id)
	return nil, nil
}

// GetInterventionParticipantByPhone retrieves an intervention participant by phone number.
func (s *InMemoryStore) GetInterventionParticipantByPhone(phoneNumber string) (*models.InterventionParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, participant := range s.interventionParticipants {
		if participant.PhoneNumber == phoneNumber {
			slog.Debug("InMemoryStore GetInterventionParticipantByPhone found", "phone", phoneNumber, "id", participant.ID)
			// Return a copy to prevent external modifications
			participantCopy := participant
			return &participantCopy, nil
		}
	}
	slog.Debug("InMemoryStore GetInterventionParticipantByPhone not found", "phone", phoneNumber)
	return nil, nil
}

// ListInterventionParticipants retrieves all intervention participants.
func (s *InMemoryStore) ListInterventionParticipants() ([]models.InterventionParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.InterventionParticipant, 0, len(s.interventionParticipants))
	for _, participant := range s.interventionParticipants {
		result = append(result, participant)
	}
	slog.Debug("InMemoryStore ListInterventionParticipants succeeded", "count", len(result))
	return result, nil
}

// DeleteInterventionParticipant removes an intervention participant.
func (s *InMemoryStore) DeleteInterventionParticipant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.interventionParticipants, id)
	slog.Debug("InMemoryStore DeleteInterventionParticipant succeeded", "id", id)
	return nil
}

// SaveInterventionResponse stores an intervention response.
func (s *InMemoryStore) SaveInterventionResponse(response models.InterventionResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interventionResponses = append(s.interventionResponses, response)
	slog.Debug("InMemoryStore SaveInterventionResponse succeeded", "participantID", response.ParticipantID, "state", response.State)
	return nil
}

// GetInterventionResponses retrieves all responses for a participant.
func (s *InMemoryStore) GetInterventionResponses(participantID string) ([]models.InterventionResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.InterventionResponse, 0)
	for _, response := range s.interventionResponses {
		if response.ParticipantID == participantID {
			result = append(result, response)
		}
	}
	slog.Debug("InMemoryStore GetInterventionResponses succeeded", "participantID", participantID, "count", len(result))
	return result, nil
}

// ListAllInterventionResponses retrieves all intervention responses.
func (s *InMemoryStore) ListAllInterventionResponses() ([]models.InterventionResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]models.InterventionResponse, len(s.interventionResponses))
	copy(result, s.interventionResponses)
	slog.Debug("InMemoryStore ListAllInterventionResponses succeeded", "count", len(result))
	return result, nil
}

// Conversation participant management methods - InMemory implementation

// SaveConversationParticipant stores or updates a conversation participant.
func (s *InMemoryStore) SaveConversationParticipant(participant models.ConversationParticipant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversationParticipants[participant.ID] = participant
	slog.Debug("InMemoryStore SaveConversationParticipant succeeded", "id", participant.ID, "phone", participant.PhoneNumber)
	return nil
}

// GetConversationParticipant retrieves a conversation participant by ID.
func (s *InMemoryStore) GetConversationParticipant(id string) (*models.ConversationParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if participant, exists := s.conversationParticipants[id]; exists {
		slog.Debug("InMemoryStore GetConversationParticipant found", "id", id)
		// Return a copy to prevent external modifications
		participantCopy := participant
		return &participantCopy, nil
	}
	slog.Debug("InMemoryStore GetConversationParticipant not found", "id", id)
	return nil, nil
}

// GetConversationParticipantByPhone retrieves a conversation participant by phone number.
func (s *InMemoryStore) GetConversationParticipantByPhone(phoneNumber string) (*models.ConversationParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, participant := range s.conversationParticipants {
		if participant.PhoneNumber == phoneNumber {
			slog.Debug("InMemoryStore GetConversationParticipantByPhone found", "phone", phoneNumber, "id", participant.ID)
			// Return a copy to prevent external modifications
			participantCopy := participant
			return &participantCopy, nil
		}
	}
	slog.Debug("InMemoryStore GetConversationParticipantByPhone not found", "phone", phoneNumber)
	return nil, nil
}

// ListConversationParticipants retrieves all conversation participants.
func (s *InMemoryStore) ListConversationParticipants() ([]models.ConversationParticipant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.ConversationParticipant, 0, len(s.conversationParticipants))
	for _, participant := range s.conversationParticipants {
		result = append(result, participant)
	}
	slog.Debug("InMemoryStore ListConversationParticipants succeeded", "count", len(result))
	return result, nil
}

// DeleteConversationParticipant removes a conversation participant.
func (s *InMemoryStore) DeleteConversationParticipant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conversationParticipants, id)
	slog.Debug("InMemoryStore DeleteConversationParticipant succeeded", "id", id)
	return nil
}

// SaveRegisteredHook stores a registered hook.
func (s *InMemoryStore) SaveRegisteredHook(hook models.RegisteredHook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registeredHooks[hook.PhoneNumber] = hook
	slog.Debug("InMemoryStore SaveRegisteredHook succeeded", "phoneNumber", hook.PhoneNumber)
	return nil
}

// GetRegisteredHook retrieves a registered hook by phone number.
func (s *InMemoryStore) GetRegisteredHook(phoneNumber string) (*models.RegisteredHook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if hook, exists := s.registeredHooks[phoneNumber]; exists {
		slog.Debug("InMemoryStore GetRegisteredHook found", "phoneNumber", phoneNumber)
		// Return a copy to prevent external modifications
		hookCopy := hook
		return &hookCopy, nil
	}
	slog.Debug("InMemoryStore GetRegisteredHook not found", "phoneNumber", phoneNumber)
	return nil, nil
}

// ListRegisteredHooks retrieves all registered hooks.
func (s *InMemoryStore) ListRegisteredHooks() ([]models.RegisteredHook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.RegisteredHook, 0, len(s.registeredHooks))
	for _, hook := range s.registeredHooks {
		result = append(result, hook)
	}
	slog.Debug("InMemoryStore ListRegisteredHooks succeeded", "count", len(result))
	return result, nil
}

// DeleteRegisteredHook removes a registered hook by phone number.
func (s *InMemoryStore) DeleteRegisteredHook(phoneNumber string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.registeredHooks, phoneNumber)
	slog.Debug("InMemoryStore DeleteRegisteredHook succeeded", "phoneNumber", phoneNumber)
	return nil
}
