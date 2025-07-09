package messaging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
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

func (m *MockStateManager) GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error) {
	key := participantID + ":" + string(flowType)
	return models.StateType(m.states[key]), nil
}

func (m *MockStateManager) SetCurrentState(ctx context.Context, participantID string, flowType models.FlowType, state models.StateType) error {
	key := participantID + ":" + string(flowType)
	m.states[key] = string(state)
	return nil
}

func (m *MockStateManager) GetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey) (string, error) {
	dataKey := participantID + ":" + string(flowType) + ":" + string(key)
	return m.data[dataKey], nil
}

func (m *MockStateManager) SetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey, value string) error {
	dataKey := participantID + ":" + string(flowType) + ":" + string(key)
	m.data[dataKey] = value
	return nil
}

func (m *MockStateManager) TransitionState(ctx context.Context, participantID string, flowType models.FlowType, fromState, toState models.StateType) error {
	return m.SetCurrentState(ctx, participantID, flowType, toState)
}

func (m *MockStateManager) ResetState(ctx context.Context, participantID string, flowType models.FlowType) error {
	// Remove state and data for this participant and flow type
	stateKey := participantID + ":" + string(flowType)
	delete(m.states, stateKey)

	// Remove all state data for this participant and flow type
	prefix := participantID + ":" + string(flowType) + ":"
	for dataKey := range m.data {
		if strings.HasPrefix(dataKey, prefix) {
			delete(m.data, dataKey)
		}
	}
	return nil
}
func TestResponseHandler_RegisterHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

	testPhone := "+1234567890"
	expectedCanonical := "1234567890"

	// Test registering a hook
	testHook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
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
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

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
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

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
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

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
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

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
	stateManager.SetCurrentState(context.Background(), participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)

	// Create intervention hook
	timer := flow.NewSimpleTimer()
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService, timer)

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
	newState, _ := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if newState != models.StateCommitmentPrompt {
		t.Errorf("Expected state %s, got %s", models.StateCommitmentPrompt, newState)
	}
}

func TestCreateInterventionHook_CommitmentResponse(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	stateManager := NewMockStateManager()

	participantID := "test_participant"
	phoneNumber := "+1234567890"

	// Set state to COMMITMENT_PROMPT
	stateManager.SetCurrentState(context.Background(), participantID, models.FlowTypeMicroHealthIntervention, models.StateCommitmentPrompt)

	// Create intervention hook
	timer := flow.NewSimpleTimer()
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService, timer)

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
	newState, _ := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if newState != models.StateFeelingPrompt {
		t.Errorf("Expected state %s, got %s", models.StateFeelingPrompt, newState)
	}
}

func TestCreateInterventionHook_FeelingResponse(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	stateManager := NewMockStateManager()

	participantID := "test_participant"
	phoneNumber := "+1234567890"

	// Set state to FEELING_PROMPT
	stateManager.SetCurrentState(context.Background(), participantID, models.FlowTypeMicroHealthIntervention, models.StateFeelingPrompt)

	// Create intervention hook
	timer := flow.NewSimpleTimer()
	hook := CreateInterventionHook(participantID, phoneNumber, stateManager, msgService, timer)

	// Test "3" response (Motivated)
	ctx := context.Background()
	handled, err := hook(ctx, phoneNumber, "3", time.Now().Unix())

	if err != nil {
		t.Fatalf("Hook failed: %v", err)
	}

	if !handled {
		t.Error("Feeling response should have been handled")
	}

	// Verify state transition - should be one of the intervention states
	newState, _ := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if newState != models.StateSendInterventionImmediate && newState != models.StateSendInterventionReflective {
		t.Errorf("Expected state %s or %s, got %s", models.StateSendInterventionImmediate, models.StateSendInterventionReflective, newState)
	}

	// Verify feeling response was stored
	feelingResponse, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse)
	if feelingResponse != "3" {
		t.Errorf("Expected feeling response 3, got %s", feelingResponse)
	}

	// Verify flow assignment was stored
	flowAssignment, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFlowAssignment)
	if flowAssignment != string(models.FlowAssignmentImmediate) && flowAssignment != string(models.FlowAssignmentReflective) {
		t.Errorf("Expected flow assignment %s or %s, got %s", models.FlowAssignmentImmediate, models.FlowAssignmentReflective, flowAssignment)
	}
}

func TestCreateBranchHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	branchOptions := []models.BranchOption{
		{Label: "Option A", Body: "You selected option A"},
		{Label: "Option B", Body: "You selected option B"},
	}

	hook := CreateBranchHook(branchOptions, msgService)
	ctx := context.Background()
	testPhone := "1234567890"

	// Test valid selection "1"
	handled, err := hook(ctx, testPhone, "1", time.Now().Unix())
	if err != nil {
		t.Fatalf("BranchHook failed with valid selection: %v", err)
	}
	if !handled {
		t.Error("BranchHook should have handled valid selection")
	}

	// Verify confirmation message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(mockClient.SentMessages))
	}
	sentMsg := mockClient.SentMessages[0]
	if !strings.Contains(sentMsg.Body, "Option A") {
		t.Errorf("Expected confirmation to contain 'Option A', got: %s", sentMsg.Body)
	}

	// Reset mock client
	mockClient.SentMessages = nil

	// Test invalid selection "9"
	handled, err = hook(ctx, testPhone, "9", time.Now().Unix())
	if err != nil {
		t.Fatalf("BranchHook failed with invalid selection: %v", err)
	}
	if !handled {
		t.Error("BranchHook should have handled invalid selection")
	}

	// Verify guidance message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 guidance message sent, got %d", len(mockClient.SentMessages))
	}
	guidanceMsg := mockClient.SentMessages[0]
	if !strings.Contains(guidanceMsg.Body, "valid option") {
		t.Errorf("Expected guidance message, got: %s", guidanceMsg.Body)
	}
}

func TestCreateGenAIHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	originalPrompt := models.Prompt{
		Type:         models.PromptTypeGenAI,
		SystemPrompt: "You are a helpful assistant",
		UserPrompt:   "Help the user",
	}

	hook := CreateGenAIHook(originalPrompt, msgService)
	ctx := context.Background()
	testPhone := "1234567890"

	// Test short response
	handled, err := hook(ctx, testPhone, "Yes", time.Now().Unix())
	if err != nil {
		t.Fatalf("GenAIHook failed with short response: %v", err)
	}
	if !handled {
		t.Error("GenAIHook should have handled response")
	}

	// Verify acknowledgment was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(mockClient.SentMessages))
	}
	ackMsg := mockClient.SentMessages[0]
	if !strings.Contains(ackMsg.Body, "Thanks") && !strings.Contains(ackMsg.Body, "Thank") {
		t.Errorf("Expected acknowledgment message, got: %s", ackMsg.Body)
	}
}

func TestCreateStaticHook(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	hook := CreateStaticHook(msgService)
	ctx := context.Background()
	testPhone := "1234567890"

	handled, err := hook(ctx, testPhone, "Some response", time.Now().Unix())
	if err != nil {
		t.Fatalf("StaticHook failed: %v", err)
	}
	if !handled {
		t.Error("StaticHook should have handled response")
	}

	// Verify acknowledgment was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(mockClient.SentMessages))
	}
	ackMsg := mockClient.SentMessages[0]
	if !strings.Contains(ackMsg.Body, "Thanks") && !strings.Contains(ackMsg.Body, "recorded") {
		t.Errorf("Expected acknowledgment message, got: %s", ackMsg.Body)
	}
}

func TestResponseHandlerFactory_CreateHandlerForPrompt(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	factory := NewResponseHandlerFactory(msgService)

	// Test branch prompt
	branchPrompt := models.Prompt{
		Type: models.PromptTypeBranch,
		BranchOptions: []models.BranchOption{
			{Label: "A", Body: "Option A"},
			{Label: "B", Body: "Option B"},
		},
	}
	handler := factory.CreateHandlerForPrompt(branchPrompt)
	if handler == nil {
		t.Error("Factory should create handler for branch prompt")
	}

	// Test GenAI prompt
	genaiPrompt := models.Prompt{
		Type:         models.PromptTypeGenAI,
		SystemPrompt: "System",
		UserPrompt:   "User",
	}
	handler = factory.CreateHandlerForPrompt(genaiPrompt)
	if handler == nil {
		t.Error("Factory should create handler for GenAI prompt")
	}

	// Test static prompt that expects response
	staticPromptWithQuestion := models.Prompt{
		Type: models.PromptTypeStatic,
		Body: "How are you feeling? Please reply with your answer.",
	}
	handler = factory.CreateHandlerForPrompt(staticPromptWithQuestion)
	if handler == nil {
		t.Error("Factory should create handler for static prompt with question")
	}

	// Test static prompt that doesn't expect response
	staticPromptNoResponse := models.Prompt{
		Type: models.PromptTypeStatic,
		Body: "This is just an informational message.",
	}
	handler = factory.CreateHandlerForPrompt(staticPromptNoResponse)
	if handler != nil {
		t.Error("Factory should not create handler for static prompt without question")
	}

	// Test custom prompt (should not create handler)
	customPrompt := models.Prompt{
		Type: models.PromptTypeCustom,
		Body: "Custom flow",
	}
	handler = factory.CreateHandlerForPrompt(customPrompt)
	if handler != nil {
		t.Error("Factory should not create handler for custom prompt")
	}
}

func TestResponseHandler_AutoRegisterResponseHandler(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	handler := NewResponseHandler(msgService, store.NewInMemoryStore())

	// Test with branch prompt
	branchPrompt := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeBranch,
		BranchOptions: []models.BranchOption{
			{Label: "Yes", Body: "You said yes"},
			{Label: "No", Body: "You said no"},
		},
	}

	registered := handler.AutoRegisterResponseHandler(branchPrompt)
	if !registered {
		t.Error("Should have registered handler for branch prompt")
	}

	// Verify handler is registered
	if !handler.IsHookRegistered("1234567890") {
		t.Error("Handler should be registered for canonicalized number")
	}

	// Test with static prompt that doesn't need handler
	staticPrompt := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeStatic,
		Body: "Just an info message",
	}

	registered = handler.AutoRegisterResponseHandler(staticPrompt)
	if registered {
		t.Error("Should not have registered handler for static prompt without question")
	}
}

// TestResponseHandler_ValidateAndCleanupHooks tests the validation and cleanup functionality
func TestResponseHandler_ValidateAndCleanupHooks(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	inMemoryStore := store.NewInMemoryStore()
	handler := NewResponseHandler(msgService, inMemoryStore)

	// Create test participants
	activeParticipant := models.InterventionParticipant{
		ID:          "active123",
		PhoneNumber: "+1234567890",
		Status:      models.ParticipantStatusActive,
		Name:        "Active Participant",
	}
	inactiveParticipant := models.InterventionParticipant{
		ID:          "inactive123",
		PhoneNumber: "+9876543210",
		Status:      models.ParticipantStatusPaused,
		Name:        "Inactive Participant",
	}

	// Save participants to store
	err := inMemoryStore.SaveInterventionParticipant(activeParticipant)
	if err != nil {
		t.Fatalf("Failed to save active participant: %v", err)
	}
	err = inMemoryStore.SaveInterventionParticipant(inactiveParticipant)
	if err != nil {
		t.Fatalf("Failed to save inactive participant: %v", err)
	}

	// Register hooks for both participants
	testHook := func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		return true, nil
	}

	err = handler.RegisterHook(activeParticipant.PhoneNumber, testHook)
	if err != nil {
		t.Fatalf("Failed to register hook for active participant: %v", err)
	}
	err = handler.RegisterHook(inactiveParticipant.PhoneNumber, testHook)
	if err != nil {
		t.Fatalf("Failed to register hook for inactive participant: %v", err)
	}

	// Verify both hooks are registered
	if handler.GetHookCount() != 2 {
		t.Errorf("Expected 2 hooks registered, got %d", handler.GetHookCount())
	}

	// Run validation and cleanup
	ctx := context.Background()
	err = handler.ValidateAndCleanupHooks(ctx)
	if err != nil {
		t.Errorf("ValidateAndCleanupHooks returned error: %v", err)
	}

	// Verify only active participant hook remains
	if handler.GetHookCount() != 1 {
		t.Errorf("Expected 1 hook after cleanup, got %d", handler.GetHookCount())
	}

	// Verify the correct hook remains
	if !handler.IsHookRegistered("1234567890") {
		t.Error("Active participant hook should still be registered")
	}
	if handler.IsHookRegistered("9876543210") {
		t.Error("Inactive participant hook should have been removed")
	}
}

// TestResponseHandler_IsParticipantActive tests the participant activity check
func TestResponseHandler_IsParticipantActive(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	inMemoryStore := store.NewInMemoryStore()
	handler := NewResponseHandler(msgService, inMemoryStore)

	// Create test participants with canonicalized phone numbers
	activeParticipant := models.InterventionParticipant{
		ID:          "active123",
		PhoneNumber: "1234567890", // Canonicalized format
		Status:      models.ParticipantStatusActive,
		Name:        "Active Participant",
	}
	inactiveParticipant := models.InterventionParticipant{
		ID:          "inactive123",
		PhoneNumber: "9876543210", // Canonicalized format
		Status:      models.ParticipantStatusPaused,
		Name:        "Inactive Participant",
	}

	// Save participants to store
	err := inMemoryStore.SaveInterventionParticipant(activeParticipant)
	if err != nil {
		t.Fatalf("Failed to save active participant: %v", err)
	}
	err = inMemoryStore.SaveInterventionParticipant(inactiveParticipant)
	if err != nil {
		t.Fatalf("Failed to save inactive participant: %v", err)
	}

	ctx := context.Background()

	// Test active participant
	isActive, err := handler.IsParticipantActive(ctx, activeParticipant.PhoneNumber)
	if err != nil {
		t.Fatalf("Error checking active participant: %v", err)
	}
	if !isActive {
		t.Error("Active participant should be reported as active")
	}

	// Test inactive participant
	isActive, err = handler.IsParticipantActive(ctx, inactiveParticipant.PhoneNumber)
	if err != nil {
		t.Fatalf("Error checking inactive participant: %v", err)
	}
	if isActive {
		t.Error("Inactive participant should be reported as inactive")
	}

	// Test non-existent participant
	isActive, err = handler.IsParticipantActive(ctx, "+5555555555")
	if err != nil {
		t.Fatalf("Error checking non-existent participant: %v", err)
	}
	if isActive {
		t.Error("Non-existent participant should be reported as inactive")
	}
}
