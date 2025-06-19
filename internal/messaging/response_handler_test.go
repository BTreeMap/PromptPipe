package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// MockStateManager implements StateManager for testing
type MockStateManager struct {
	states map[string]string // participantID+flowType -> state
	data   map[string]string // participantID+flowType+key -> value
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		states: make(map[string]string),
		data:   make(map[string]string),
	}
}

func (m *MockStateManager) GetCurrentState(ctx context.Context, participantID, flowType string) (string, error) {
	key := participantID + ":" + flowType
	return m.states[key], nil
}

func (m *MockStateManager) SetCurrentState(ctx context.Context, participantID, flowType, state string) error {
	key := participantID + ":" + flowType
	m.states[key] = state
	return nil
}

func (m *MockStateManager) GetStateData(ctx context.Context, participantID, flowType, key string) (string, error) {
	dataKey := participantID + ":" + flowType + ":" + key
	return m.data[dataKey], nil
}

func (m *MockStateManager) SetStateData(ctx context.Context, participantID, flowType, key, value string) error {
	dataKey := participantID + ":" + flowType + ":" + key
	m.data[dataKey] = value
	return nil
}

func (m *MockStateManager) TransitionState(ctx context.Context, participantID, flowType, fromState, toState string) error {
	return m.SetCurrentState(ctx, participantID, flowType, toState)
}

func TestResponseHandler_RegisterHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService)

	testPhone := "+1234567890"
	expectedCanonical := "1234567890"

	// Test registering a hook
	hookCalled := false
	testHook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		hookCalled = true
		return true, nil
	}

	err := handler.RegisterHook(testPhone, testHook)
	if err != nil {
		t.Fatalf("RegisterHook failed: %v", err)
	}

	// Verify hook is registered
	if !handler.IsHookRegistered(expectedCanonical) {
		t.Error("Hook should be registered for canonical phone number")
	}

	// Test hook count
	if count := handler.GetHookCount(); count != 1 {
		t.Errorf("Expected 1 hook, got %d", count)
	}

	// Test listing recipients
	recipients := handler.ListRegisteredRecipients()
	if len(recipients) != 1 || recipients[0] != expectedCanonical {
		t.Errorf("Expected [%s], got %v", expectedCanonical, recipients)
	}
}

func TestResponseHandler_UnregisterHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService)

	testPhone := "+1234567890"
	testHook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		return true, nil
	}

	// Register then unregister
	err := handler.RegisterHook(testPhone, testHook)
	if err != nil {
		t.Fatalf("RegisterHook failed: %v", err)
	}

	err = handler.UnregisterHook(testPhone)
	if err != nil {
		t.Fatalf("UnregisterHook failed: %v", err)
	}

	// Verify hook is unregistered
	if handler.IsHookRegistered(testPhone) {
		t.Error("Hook should be unregistered")
	}

	if count := handler.GetHookCount(); count != 0 {
		t.Errorf("Expected 0 hooks, got %d", count)
	}
}

func TestResponseHandler_ProcessResponse_WithHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService)

	testPhone := "+1234567890"
	expectedCanonical := "1234567890"

	// Register a hook that handles the response
	hookCalled := false
	var receivedFrom, receivedText string
	var receivedTimestamp int64

	testHook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		hookCalled = true
		receivedFrom = from
		receivedText = responseText
		receivedTimestamp = timestamp
		return true, nil // Indicate response was handled
	}

	err := handler.RegisterHook(testPhone, testHook)
	if err != nil {
		t.Fatalf("RegisterHook failed: %v", err)
	}

	// Create a test response
	response := models.Response{
		From: testPhone,
		Body: "test message",
		Time: time.Now().Unix(),
	}

	// Process the response
	ctx := context.Background()
	err = handler.ProcessResponse(ctx, response)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// Verify hook was called with correct parameters
	if !hookCalled {
		t.Error("Hook should have been called")
	}

	if receivedFrom != expectedCanonical {
		t.Errorf("Expected from=%s, got %s", expectedCanonical, receivedFrom)
	}

	if receivedText != response.Body {
		t.Errorf("Expected text=%s, got %s", response.Body, receivedText)
	}

	if receivedTimestamp != response.Time {
		t.Errorf("Expected timestamp=%d, got %d", response.Time, receivedTimestamp)
	}
}

func TestResponseHandler_ProcessResponse_WithoutHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService)

	testPhone := "+1234567890"

	// Create a test response without registering a hook
	response := models.Response{
		From: testPhone,
		Body: "test message",
		Time: time.Now().Unix(),
	}

	// Process the response
	ctx := context.Background()
	err := handler.ProcessResponse(ctx, response)
	if err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	// In a real test, we'd verify that the default message was sent
	// For now, we just ensure no error occurred
}

func TestResponseHandler_SetDefaultMessage(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService)

	newMessage := "Custom default message"
	handler.SetDefaultMessage(newMessage)

	if got := handler.GetDefaultMessage(); got != newMessage {
		t.Errorf("Expected default message=%s, got %s", newMessage, got)
	}
}

func TestCreateInterventionHook_ReadyOverride(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	stateManager := NewMockStateManager()
	
	participantID := "test_participant"
	phoneNumber := "+1234567890"
	
	// Set initial state to END_OF_DAY
	stateManager.SetCurrentState(context.Background(), participantID, "micro_health_intervention", "END_OF_DAY")
	
	// Create intervention hook
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService)
	
	// Test "Ready" response
	ctx := context.Background()
	handled, err := hook(ctx, phoneNumber, "Ready", time.Now().Unix())
	
	if err != nil {
		t.Fatalf("Hook failed: %v", err)
	}
	
	if !handled {
		t.Error("Ready response should have been handled")
	}
	
	// Verify state transition
	newState, _ := stateManager.GetCurrentState(ctx, participantID, "micro_health_intervention")
	if newState != "COMMITMENT_PROMPT" {
		t.Errorf("Expected state COMMITMENT_PROMPT, got %s", newState)
	}
}

func TestCreateInterventionHook_CommitmentResponse(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	stateManager := NewMockStateManager()
	
	participantID := "test_participant"
	phoneNumber := "+1234567890"
	
	// Set state to COMMITMENT_PROMPT
	stateManager.SetCurrentState(context.Background(), participantID, "micro_health_intervention", "COMMITMENT_PROMPT")
	
	// Create intervention hook
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService)
	
	// Test "1" response (Let's do it!)
	ctx := context.Background()
	handled, err := hook(ctx, phoneNumber, "1", time.Now().Unix())
	
	if err != nil {
		t.Fatalf("Hook failed: %v", err)
	}
	
	if !handled {
		t.Error("Commitment response should have been handled")
	}
	
	// Verify state transition
	newState, _ := stateManager.GetCurrentState(ctx, participantID, "micro_health_intervention")
	if newState != "FEELING_PROMPT" {
		t.Errorf("Expected state FEELING_PROMPT, got %s", newState)
	}
}

func TestCreateInterventionHook_FeelingResponse(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	stateManager := NewMockStateManager()
	
	participantID := "test_participant"
	phoneNumber := "+1234567890"
	
	// Set state to FEELING_PROMPT
	stateManager.SetCurrentState(context.Background(), participantID, "micro_health_intervention", "FEELING_PROMPT")
	
	// Create intervention hook
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService)
	
	// Test "3" response (Motivated)
	ctx := context.Background()
	handled, err := hook(ctx, phoneNumber, "3", time.Now().Unix())
	
	if err != nil {
		t.Fatalf("Hook failed: %v", err)
	}
	
	if !handled {
		t.Error("Feeling response should have been handled")
	}
	
	// Verify state transition
	newState, _ := stateManager.GetCurrentState(ctx, participantID, "micro_health_intervention")
	if newState != "RANDOM_ASSIGNMENT" {
		t.Errorf("Expected state RANDOM_ASSIGNMENT, got %s", newState)
	}
	
	// Verify feeling response was stored
	feelingResponse, _ := stateManager.GetStateData(ctx, participantID, "micro_health_intervention", "feelingResponse")
	if feelingResponse != "3" {
		t.Errorf("Expected feeling response 3, got %s", feelingResponse)
	}
}
