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

// TestMockService wraps the WhatsApp service for testing
type TestMockService struct {
	*WhatsAppService
	mockClient    *whatsapp.MockClient
	sentMessages  []TestSentMessage
	responsesChan chan models.Response
}

type TestSentMessage struct {
	recipient string
	message   string
}

// NewTestMockService creates a mock messaging service for testing
func NewTestMockService() *TestMockService {
	mockClient := whatsapp.NewMockClient()
	whatsappService := NewWhatsAppService(mockClient)

	return &TestMockService{
		WhatsAppService: whatsappService,
		mockClient:      mockClient,
		sentMessages:    make([]TestSentMessage, 0),
		responsesChan:   make(chan models.Response, 100),
	}
}

// SendMessage overrides the base implementation to track sent messages
func (m *TestMockService) SendMessage(ctx context.Context, to, body string) error {
	err := m.WhatsAppService.SendMessage(ctx, to, body)
	if err == nil {
		m.sentMessages = append(m.sentMessages, TestSentMessage{
			recipient: to,
			message:   body,
		})
	}
	return err
}

// TestMockTimer implements models.Timer for testing
type TestMockTimer struct {
	scheduledFunctions []func()
}

func (m *TestMockTimer) ScheduleAfter(delay time.Duration, fn func()) (string, error) {
	m.scheduledFunctions = append(m.scheduledFunctions, fn)
	return "mock-timer-id", nil
}

func (m *TestMockTimer) ScheduleAt(when time.Time, fn func()) (string, error) {
	m.scheduledFunctions = append(m.scheduledFunctions, fn)
	return "mock-timer-id", nil
}

func (m *TestMockTimer) ScheduleWithSchedule(schedule *models.Schedule, fn func()) (string, error) {
	m.scheduledFunctions = append(m.scheduledFunctions, fn)
	return "mock-timer-id", nil
}

func (m *TestMockTimer) Cancel(id string) error {
	return nil
}

func (m *TestMockTimer) Stop() {
	// no-op for mock
}

func (m *TestMockTimer) ListActive() []models.TimerInfo {
	return []models.TimerInfo{}
}

func (m *TestMockTimer) GetTimer(id string) (*models.TimerInfo, error) {
	return nil, nil
}

// TestHookPersistence_EndToEndFlow tests the complete hook persistence workflow
func TestHookPersistence_EndToEndFlow(t *testing.T) {
	// Setup
	st := store.NewInMemoryStore()
	msgService := NewTestMockService()

	// Simulate server shutdown and restart
	t.Run("Initial Registration", func(t *testing.T) {
		// Set up participant first
		participant := models.InterventionParticipant{
			ID:          "test-participant-123",
			PhoneNumber: "1234567890",
			Name:        "Test User",
			Status:      models.ParticipantStatusActive,
			EnrolledAt:  time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := st.SaveInterventionParticipant(participant); err != nil {
			t.Fatalf("Failed to save test participant: %v", err)
		}

		// Create initial handler (simulating server start)
		handler := NewResponseHandler(msgService, st)
		stateManager := flow.NewStoreBasedStateManager(st)
		timer := &TestMockTimer{}
		handler.SetDependencies(stateManager, timer)

		// Set up initial flow state for the participant
		bgCtx := context.Background()
		if err := stateManager.SetCurrentState(bgCtx, "test-participant-123", models.FlowTypeMicroHealthIntervention, models.StateOrientation); err != nil {
			t.Fatalf("Failed to set participant state: %v", err)
		}

		// Register a persistent hook (simulating participant enrollment)
		params := map[string]string{
			"participant_id": "test-participant-123",
			"phone_number":   "+1234567890",
		}

		err := handler.RegisterPersistentHook("+1234567890", models.HookTypeIntervention, params)
		if err != nil {
			t.Fatalf("Failed to register persistent hook: %v", err)
		}

		// Verify hook is stored in database
		hook, err := st.GetRegisteredHook("1234567890") // Use canonicalized phone number
		if err != nil {
			t.Fatalf("Failed to get stored hook: %v", err)
		}

		if hook == nil {
			t.Fatal("Hook was not persisted to database")
		}

		// Test that the hook works immediately
		ctx := context.Background()
		response := models.Response{
			From: "+1234567890",
			Body: "ready",
			Time: time.Now().Unix(),
		}

		err = handler.ProcessResponse(ctx, response)
		if err != nil {
			t.Errorf("Failed to process response with registered hook: %v", err)
		}

		// Verify that a message was sent (intervention hook should send a response)
		if len(msgService.sentMessages) == 0 {
			t.Error("Expected intervention hook to send a message, but none were sent")
		}
	})

	t.Run("Server Restart and Recovery", func(t *testing.T) {
		// Reset the mock service to simulate server restart
		msgService.sentMessages = nil

		// Create new handler (simulating server restart)
		newHandler := NewResponseHandler(msgService, st)
		stateManager := flow.NewStoreBasedStateManager(st)
		timer := &TestMockTimer{}
		newHandler.SetDependencies(stateManager, timer)

		// Recover persistent hooks from database
		ctx := context.Background()
		err := newHandler.RecoverPersistentHooks(ctx)
		if err != nil {
			t.Fatalf("Failed to recover persistent hooks: %v", err)
		}

		// Test that recovered hook still works
		response := models.Response{
			From: "+1234567890",
			Body: "ready",
			Time: time.Now().Unix(),
		}

		err = newHandler.ProcessResponse(ctx, response)
		if err != nil {
			t.Errorf("Failed to process response with recovered hook: %v", err)
		}

		// Verify that the recovered hook sent a message
		if len(msgService.sentMessages) == 0 {
			t.Error("Expected recovered hook to send a message, but none were sent")
		}
	})

	t.Run("Hook Unregistration", func(t *testing.T) {
		// Create handler with recovered hooks
		handler := NewResponseHandler(msgService, st)
		stateManager := flow.NewStoreBasedStateManager(st)
		timer := &TestMockTimer{}
		handler.SetDependencies(stateManager, timer)

		// Recover hooks first
		ctx := context.Background()
		err := handler.RecoverPersistentHooks(ctx)
		if err != nil {
			t.Fatalf("Failed to recover hooks for unregistration test: %v", err)
		}

		// Verify hook is active before unregistration
		msgService.sentMessages = nil
		response := models.Response{
			From: "+1234567890",
			Body: "ready",
			Time: time.Now().Unix(),
		}

		err = handler.ProcessResponse(ctx, response)
		if err != nil {
			t.Errorf("ProcessResponse should work with recovered hook: %v", err)
		}

		if len(msgService.sentMessages) == 0 {
			t.Error("Expected recovered hook to send a message before unregistration")
		}

		// Unregister the hook (simulating participant deletion)
		err = handler.UnregisterPersistentHook("+1234567890")
		if err != nil {
			t.Fatalf("Failed to unregister persistent hook: %v", err)
		}

		// Verify hook is removed from database
		hook, err := st.GetRegisteredHook("1234567890") // Use canonicalized phone number
		if err != nil {
			t.Fatalf("Failed to check hook removal: %v", err)
		}

		if hook != nil {
			t.Error("Hook should have been removed from database")
		}

		// Reset sent messages counter
		msgService.sentMessages = nil

		// Test that hook no longer responds
		response = models.Response{
			From: "+1234567890",
			Body: "ready",
			Time: time.Now().Unix(),
		}

		err = handler.ProcessResponse(ctx, response)
		if err != nil {
			t.Errorf("ProcessResponse should not fail even without hook: %v", err)
		}

		// Verify no message was sent by hook (default response handler may still send a message)
		hookMessageSent := false
		for _, msg := range msgService.sentMessages {
			// Check if this is an intervention-style message (hook message)
			// Default messages typically say "Thanks for your message" or contain "recorded"
			// Hook messages are usually longer and relate to the intervention flow
			if !strings.Contains(msg.message, "recorded") &&
				!strings.Contains(msg.message, "Thank") &&
				len(msg.message) > 50 { // Intervention messages are typically longer
				hookMessageSent = true
				t.Logf("Found potential hook message: %s", msg.message)
				break
			}
		}

		if hookMessageSent {
			t.Error("Expected no hook messages after hook unregistration, but hook-style message was sent")
		}
	})
}

// TestHookPersistence_MultipleParticipants tests hook persistence with multiple participants
func TestHookPersistence_MultipleParticipants(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewTestMockService()
	handler := NewResponseHandler(msgService, st)
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &TestMockTimer{}
	handler.SetDependencies(stateManager, timer)

	// Register multiple participants
	participants := []struct {
		phone  string
		params map[string]string
		id     string
	}{
		{
			phone: "+1111111111",
			id:    "participant-1",
			params: map[string]string{
				"participant_id": "participant-1",
				"phone_number":   "+1111111111",
			},
		},
		{
			phone: "+2222222222",
			id:    "participant-2",
			params: map[string]string{
				"participant_id": "participant-2",
				"phone_number":   "+2222222222",
			},
		},
		{
			phone: "+3333333333",
			id:    "participant-3",
			params: map[string]string{
				"participant_id": "participant-3",
				"phone_number":   "+3333333333",
			},
		},
	}

	// Create participants in the database and set up flow state
	for _, p := range participants {
		participant := models.InterventionParticipant{
			ID:          p.id,
			PhoneNumber: strings.TrimPrefix(p.phone, "+"), // Store canonicalized phone
			Name:        "Test User " + p.id,
			Status:      models.ParticipantStatusActive,
			EnrolledAt:  time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := st.SaveInterventionParticipant(participant); err != nil {
			t.Fatalf("Failed to save participant %s: %v", p.id, err)
		}

		// Set up flow state
		ctx := context.Background()
		if err := stateManager.SetCurrentState(ctx, p.id, models.FlowTypeMicroHealthIntervention, models.StateOrientation); err != nil {
			t.Fatalf("Failed to set participant state for %s: %v", p.id, err)
		}
	}

	// Register all participants
	for _, p := range participants {
		err := handler.RegisterPersistentHook(p.phone, models.HookTypeIntervention, p.params)
		if err != nil {
			t.Fatalf("Failed to register hook for %s: %v", p.phone, err)
		}
	}

	// Verify all hooks are stored
	hooks, err := st.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("Failed to list hooks: %v", err)
	}

	if len(hooks) != len(participants) {
		t.Errorf("Expected %d hooks, got %d", len(participants), len(hooks))
	}

	// Simulate server restart
	newHandler := NewResponseHandler(msgService, st)
	newHandler.SetDependencies(stateManager, timer)

	ctx := context.Background()
	err = newHandler.RecoverPersistentHooks(ctx)
	if err != nil {
		t.Fatalf("Failed to recover hooks: %v", err)
	}

	// Test that all recovered hooks work
	msgService.sentMessages = nil
	for _, p := range participants {
		response := models.Response{
			From: p.phone,
			Body: "ready",
			Time: time.Now().Unix(),
		}

		err = newHandler.ProcessResponse(ctx, response)
		if err != nil {
			t.Errorf("Failed to process response for %s: %v", p.phone, err)
		}
	}

	// Verify that messages were sent for all participants
	if len(msgService.sentMessages) != len(participants) {
		t.Errorf("Expected %d messages, got %d", len(participants), len(msgService.sentMessages))
	}

	// Verify each participant got a response
	receivedResponses := make(map[string]bool)
	for _, msg := range msgService.sentMessages {
		receivedResponses[msg.recipient] = true
	}

	// Check canonicalized phone numbers
	canonicalPhones := []string{"1111111111", "2222222222", "3333333333"}
	for _, phone := range canonicalPhones {
		if !receivedResponses[phone] {
			t.Errorf("Participant %s did not receive a response", phone)
		}
	}
}

// TestHookPersistence_FailureRecovery tests hook recovery behavior when some hooks fail to recreate
func TestHookPersistence_FailureRecovery(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewTestMockService()

	// Set up a participant for the valid hook to work properly
	participant := models.InterventionParticipant{
		ID:          "valid-participant",
		PhoneNumber: "1111111111",
		Name:        "Test User",
		Status:      models.ParticipantStatusActive,
		EnrolledAt:  time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := st.SaveInterventionParticipant(participant); err != nil {
		t.Fatalf("Failed to save test participant: %v", err)
	}

	// Set up flow state for the participant
	stateMgr := flow.NewStoreBasedStateManager(st)
	bgCtx := context.Background()
	if err := stateMgr.SetCurrentState(bgCtx, "valid-participant", models.FlowTypeMicroHealthIntervention, models.StateOrientation); err != nil {
		t.Fatalf("Failed to set participant state: %v", err)
	}

	// Manually store hooks in database - one valid, one invalid
	validHook := models.RegisteredHook{
		PhoneNumber: "1111111111", // Use canonicalized phone number
		HookType:    models.HookTypeIntervention,
		Parameters: map[string]string{
			"participant_id": "valid-participant",
			"phone_number":   "1111111111", // Use canonicalized phone number
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	invalidHook := models.RegisteredHook{
		PhoneNumber: "2222222222", // Use canonicalized phone number
		HookType:    models.HookTypeIntervention,
		Parameters:  map[string]string{
			// Missing required parameters
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := st.SaveRegisteredHook(validHook)
	if err != nil {
		t.Fatalf("Failed to save valid hook: %v", err)
	}

	err = st.SaveRegisteredHook(invalidHook)
	if err != nil {
		t.Fatalf("Failed to save invalid hook: %v", err)
	}

	// Create new handler and recover hooks
	handler := NewResponseHandler(msgService, st)
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &TestMockTimer{}
	handler.SetDependencies(stateManager, timer)

	ctx := context.Background()
	err = handler.RecoverPersistentHooks(ctx)
	// Should not fail even if some hooks fail to recreate
	if err != nil {
		t.Fatalf("RecoverPersistentHooks should not fail even with invalid hooks: %v", err)
	}

	// Test that valid hook works
	response1 := models.Response{
		From: "1111111111", // Use canonicalized phone number
		Body: "ready",
		Time: time.Now().Unix(),
	}

	err = handler.ProcessResponse(ctx, response1)
	if err != nil {
		t.Errorf("Valid hook should work: %v", err)
	}

	// Test that invalid hook doesn't crash the system
	response2 := models.Response{
		From: "2222222222", // Use canonicalized phone number
		Body: "ready",
		Time: time.Now().Unix(),
	}

	err = handler.ProcessResponse(ctx, response2)
	if err != nil {
		t.Errorf("ProcessResponse should not fail for unrecovered hook: %v", err)
	}

	// Valid hook should have sent a message, invalid one should not
	validSent := false
	invalidSent := false

	for _, msg := range msgService.sentMessages {
		if msg.recipient == "1111111111" {
			validSent = true
		}
		if msg.recipient == "2222222222" {
			invalidSent = true
		}
	}

	if !validSent {
		t.Error("Valid hook should have sent a message")
	}

	// For the invalid hook, we expect either no message or a default message
	// But we should NOT get a hook-specific intervention message
	if invalidSent {
		// Check if it's just a default message, which is acceptable
		for _, msg := range msgService.sentMessages {
			if msg.recipient == "2222222222" {
				// If it's a short default message, that's fine
				// If it's a long intervention message, that's a problem
				if len(msg.message) > 50 && !strings.Contains(msg.message, "recorded") {
					t.Error("Invalid hook should not have sent an intervention message")
				}
			}
		}
	}
}
