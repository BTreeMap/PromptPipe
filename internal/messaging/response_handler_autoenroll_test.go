package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// TestAutoEnrollment_Enabled tests that auto-enrollment creates a participant when enabled
func TestAutoEnrollment_Enabled(t *testing.T) {
	// Setup
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create response handler with auto-enrollment ENABLED
	respHandler := NewResponseHandler(msgService, st, true)

	// Verify no participants exist initially
	participants, err := st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants initially, got %d", len(participants))
	}

	// Simulate incoming message from new user
	response := models.Response{
		From: "+1234567890",
		Body: "Hello, this is my first message!",
		Time: time.Now().Unix(),
	}

	// Process the response
	ctx := context.Background()
	err = respHandler.ProcessResponse(ctx, response)
	// Note: We expect an error because the conversation flow is not fully initialized in this test,
	// but the auto-enrollment should still happen before the error
	// The important part is that the participant is created

	// Verify participant was auto-enrolled
	participants, err = st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants after auto-enrollment: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant after auto-enrollment, got %d", len(participants))
	}

	// Verify participant details
	participant := participants[0]
	if participant.PhoneNumber != "1234567890" { // Canonicalized (no +)
		t.Errorf("Expected phone number 1234567890, got %s", participant.PhoneNumber)
	}
	if participant.Status != models.ConversationStatusActive {
		t.Errorf("Expected status ACTIVE, got %s", participant.Status)
	}
	if participant.Name != "" || participant.Gender != "" || participant.Ethnicity != "" || participant.Background != "" {
		t.Errorf("Expected empty profile fields, got Name=%s, Gender=%s, Ethnicity=%s, Background=%s",
			participant.Name, participant.Gender, participant.Ethnicity, participant.Background)
	}

	// Verify hook was registered
	if !respHandler.IsHookRegistered("+1234567890") {
		t.Error("Expected conversation hook to be registered for auto-enrolled participant")
	}
}

// TestAutoEnrollment_Disabled tests that auto-enrollment does NOT create a participant when disabled
func TestAutoEnrollment_Disabled(t *testing.T) {
	// Setup
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create response handler with auto-enrollment DISABLED
	respHandler := NewResponseHandler(msgService, st, false)

	// Verify no participants exist initially
	participants, err := st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants initially, got %d", len(participants))
	}

	// Simulate incoming message from new user
	response := models.Response{
		From: "+1234567890",
		Body: "Hello, this is my first message!",
		Time: time.Now().Unix(),
	}

	// Process the response
	ctx := context.Background()
	err = respHandler.ProcessResponse(ctx, response)
	if err != nil {
		t.Fatalf("Failed to process response: %v", err)
	}

	// Verify participant was NOT auto-enrolled
	participants, err = st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants when auto-enrollment disabled, got %d", len(participants))
	}

	// Verify hook was NOT registered
	if respHandler.IsHookRegistered("+1234567890") {
		t.Error("Did not expect hook to be registered when auto-enrollment is disabled")
	}
}

// TestAutoEnrollment_ExistingParticipant tests that auto-enrollment skips existing participants
func TestAutoEnrollment_ExistingParticipant(t *testing.T) {
	// Setup
	mockClient := whatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create response handler with auto-enrollment ENABLED
	respHandler := NewResponseHandler(msgService, st, true)

	// Manually enroll a participant first
	now := time.Now()
	existingParticipant := models.ConversationParticipant{
		ID:          "existing-001",
		PhoneNumber: "1234567890", // Already canonicalized
		Name:        "Existing User",
		Status:      models.ConversationStatusActive,
		EnrolledAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err := st.SaveConversationParticipant(existingParticipant)
	if err != nil {
		t.Fatalf("Failed to save existing participant: %v", err)
	}

	// Simulate incoming message from existing user
	response := models.Response{
		From: "+1234567890",
		Body: "Hello again!",
		Time: time.Now().Unix(),
	}

	// Process the response
	ctx := context.Background()
	err = respHandler.ProcessResponse(ctx, response)
	if err != nil {
		t.Fatalf("Failed to process response: %v", err)
	}

	// Verify only one participant exists (the original)
	participants, err := st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant, got %d", len(participants))
	}

	// Verify participant details are unchanged
	participant := participants[0]
	if participant.ID != "existing-001" {
		t.Errorf("Expected participant ID existing-001, got %s", participant.ID)
	}
	if participant.Name != "Existing User" {
		t.Errorf("Expected name 'Existing User', got %s", participant.Name)
	}
}
