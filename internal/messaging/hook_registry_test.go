package messaging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// MockTimer implements models.Timer interface for testing
type MockTimer struct{}

func (m *MockTimer) ScheduleAfter(delay time.Duration, fn func()) (string, error) {
	// For testing, just call the callback immediately
	fn()
	return "test-timer-id", nil
}

func (m *MockTimer) ScheduleAt(when time.Time, fn func()) (string, error) {
	// For testing, just call the callback immediately
	fn()
	return "test-timer-id", nil
}

func (m *MockTimer) ScheduleWithSchedule(schedule *models.Schedule, fn func()) (string, error) {
	// For testing, just call the callback immediately
	fn()
	return "test-timer-id", nil
}

func (m *MockTimer) Cancel(id string) error {
	return nil
}

func (m *MockTimer) Stop() {
}

func (m *MockTimer) ListActive() []models.TimerInfo {
	return []models.TimerInfo{}
}

func (m *MockTimer) GetTimer(id string) (*models.TimerInfo, error) {
	return nil, nil
}

// MockService implements messaging.Service interface for testing
type MockService struct {
	sentMessages []struct {
		recipient string
		message   string
	}
	receipts  chan models.Receipt
	responses chan models.Response
}

func NewMockService() *MockService {
	return &MockService{
		receipts:  make(chan models.Receipt, 10),
		responses: make(chan models.Response, 10),
	}
}

func (m *MockService) SendMessage(ctx context.Context, recipient, message string) error {
	m.sentMessages = append(m.sentMessages, struct {
		recipient string
		message   string
	}{recipient, message})
	return nil
}

func (m *MockService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	return recipient, nil
}

func (m *MockService) Start(ctx context.Context) error {
	return nil
}

func (m *MockService) Stop() error {
	return nil
}

func (m *MockService) Receipts() <-chan models.Receipt {
	return m.receipts
}

func (m *MockService) Responses() <-chan models.Response {
	return m.responses
}

func TestNewHookRegistry(t *testing.T) {
	registry := NewHookRegistry()

	if registry == nil {
		t.Fatal("NewHookRegistry returned nil")
	}

	if registry.factories == nil {
		t.Fatal("Registry factories map is nil")
	}

	// Check that default factories are registered
	expectedTypes := []models.HookType{
		models.HookTypeIntervention,
		models.HookTypeBranch,
		models.HookTypeGenAI,
		models.HookTypeStatic,
	}

	for _, hookType := range expectedTypes {
		if !registry.IsRegistered(hookType) {
			t.Errorf("Default hook type %s is not registered", hookType)
		}
	}
}

func TestHookRegistry_RegisterFactory(t *testing.T) {
	registry := NewHookRegistry()

	// Register a custom factory
	customType := models.HookType("custom")
	factory := func(params map[string]string, stateManager flow.StateManager, msgService Service, timer models.Timer) (ResponseAction, error) {
		return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
			return true, nil
		}, nil
	}

	registry.RegisterFactory(customType, factory)

	if !registry.IsRegistered(customType) {
		t.Errorf("Custom hook type %s was not registered", customType)
	}
}

func TestHookRegistry_CreateHook_Intervention(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	params := map[string]string{
		"participant_id": "test-participant",
		"phone_number":   "+1234567890",
	}

	hook, err := registry.CreateHook(models.HookTypeIntervention, params, stateManager, msgService, timer)
	if err != nil {
		t.Fatalf("Failed to create intervention hook: %v", err)
	}

	if hook == nil {
		t.Fatal("Created hook is nil")
	}

	// Test that the hook function works
	ctx := context.Background()
	handled, err := hook(ctx, "+1234567890", "test message", time.Now().Unix())
	if err != nil {
		t.Errorf("Hook execution failed: %v", err)
	}
	if !handled {
		t.Error("Hook should have handled the message")
	}
}

func TestHookRegistry_CreateHook_Branch(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	params := map[string]string{
		"participant_id": "test-participant",
		"flow_type":      "test-flow",
		"branches":       "branch1,branch2",
	}

	hook, err := registry.CreateHook(models.HookTypeBranch, params, stateManager, msgService, timer)

	// Branch hooks are not supported for persistence, so expect an error
	if err == nil {
		t.Fatal("Expected error for branch hook creation, got nil")
	}

	if hook != nil {
		t.Errorf("Expected nil hook for unsupported type, got %T", hook)
	}

	expectedError := "branch hooks are not supported for persistence"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}

func TestHookRegistry_CreateHook_GenAI(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	params := map[string]string{
		"participant_id": "test-participant",
		"flow_type":      "test-flow",
		"system_prompt":  "You are a helpful assistant",
	}

	hook, err := registry.CreateHook(models.HookTypeGenAI, params, stateManager, msgService, timer)

	// GenAI hooks are not supported for persistence, so expect an error
	if err == nil {
		t.Fatal("Expected error for GenAI hook creation, got nil")
	}

	if hook != nil {
		t.Errorf("Expected nil hook for unsupported type, got %T", hook)
	}

	expectedError := "genai hooks are not supported for persistence"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}

func TestHookRegistry_CreateHook_Static(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	params := map[string]string{
		"message": "This is a static response",
	}

	hook, err := registry.CreateHook(models.HookTypeStatic, params, stateManager, msgService, timer)
	if err != nil {
		t.Fatalf("Failed to create static hook: %v", err)
	}

	if hook == nil {
		t.Fatal("Created hook is nil")
	}

	// Test that the hook function works
	ctx := context.Background()
	handled, err := hook(ctx, "+1234567890", "test message", time.Now().Unix())
	if err != nil {
		t.Errorf("Hook execution failed: %v", err)
	}
	if !handled {
		t.Error("Hook should have handled the message")
	}
}

func TestHookRegistry_CreateHook_UnknownType(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	unknownType := models.HookType("unknown")
	params := map[string]string{}

	hook, err := registry.CreateHook(unknownType, params, stateManager, msgService, timer)

	if err == nil {
		t.Fatal("Expected error for unknown hook type, got nil")
	}

	if hook != nil {
		t.Errorf("Expected nil hook for unknown type, got %T", hook)
	}

	expectedError := "no factory registered for hook type: unknown"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

func TestHookRegistry_CreateHook_MissingParams(t *testing.T) {
	registry := NewHookRegistry()
	stateManager := NewMockStateManager()
	msgService := NewMockService()
	timer := &MockTimer{}

	// Test intervention hook with missing required parameters
	params := map[string]string{
		// Missing participant_id and phone_number
	}

	hook, err := registry.CreateHook(models.HookTypeIntervention, params, stateManager, msgService, timer)

	if err == nil {
		t.Fatal("Expected error for missing parameters, got nil")
	}

	if hook != nil {
		t.Errorf("Expected nil hook for missing parameters, got %T", hook)
	}
}

func TestHookRegistry_ListRegisteredTypes(t *testing.T) {
	registry := NewHookRegistry()

	types := registry.ListRegisteredTypes()

	if len(types) == 0 {
		t.Fatal("No registered hook types found")
	}

	expectedTypes := map[models.HookType]bool{
		models.HookTypeIntervention: false,
		models.HookTypeBranch:       false,
		models.HookTypeGenAI:        false,
		models.HookTypeStatic:       false,
	}

	for _, hookType := range types {
		if _, exists := expectedTypes[hookType]; exists {
			expectedTypes[hookType] = true
		}
	}

	for hookType, found := range expectedTypes {
		if !found {
			t.Errorf("Expected hook type %s not found in registry", hookType)
		}
	}
}

func TestHookRegistry_IsRegistered(t *testing.T) {
	registry := NewHookRegistry()

	// Test with registered types
	if !registry.IsRegistered(models.HookTypeIntervention) {
		t.Error("HookTypeIntervention should be registered")
	}

	if !registry.IsRegistered(models.HookTypeBranch) {
		t.Error("HookTypeBranch should be registered")
	}

	// Test with unregistered type
	unknownType := models.HookType("unknown")
	if registry.IsRegistered(unknownType) {
		t.Error("Unknown hook type should not be registered")
	}
}
