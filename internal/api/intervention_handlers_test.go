package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

// TestDeleteInterventionParticipant tests the deletion of an intervention participant with unregister notification
func TestDeleteInterventionParticipant(t *testing.T) {
	// Create in-memory store
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create mock messaging service
	mockMessaging := NewMockMessagingServiceForIntervention()

	// Create the API server
	server := &Server{
		st:          st,
		msgService:  mockMessaging,
		respHandler: messaging.NewResponseHandler(mockMessaging, st),
	}

	// Add a test participant to the store
	participant := models.InterventionParticipant{
		ID:              "test-delete-intervention-123",
		PhoneNumber:     "+1234567890",
		Name:            "Test Delete Intervention User",
		Status:          models.ParticipantStatusActive,
		Timezone:        "UTC",
		DailyPromptTime: "10:00",
		EnrolledAt:      time.Now(),
		WeeklyReset:     time.Now().AddDate(0, 0, 7),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	err := st.SaveInterventionParticipant(participant)
	if err != nil {
		t.Fatalf("Failed to save test participant: %v", err)
	}

	// Create a mock request
	req := httptest.NewRequest("DELETE", "/intervention/participants/test-delete-intervention-123", nil)
	req = req.WithContext(context.WithValue(req.Context(), ContextKeyParticipantID, "test-delete-intervention-123"))

	// Create a response recorder
	w := httptest.NewRecorder()

	// Call the handler
	server.deleteParticipantHandler(w, req)

	// Check the response status
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify that the notification message was sent
	if !mockMessaging.MessageSent {
		t.Error("Expected unregister notification to be sent")
	}

	expectedMsg := "You have been unregistered from the micro health intervention experiment by the organizer. If you have any questions, please contact the organizer for assistance. Thank you for your participation."
	if mockMessaging.LastMessage != expectedMsg {
		t.Errorf("Expected notification message '%s', got '%s'", expectedMsg, mockMessaging.LastMessage)
	}

	if mockMessaging.LastRecipient != "+1234567890" {
		t.Errorf("Expected notification to be sent to '+1234567890', got '%s'", mockMessaging.LastRecipient)
	}

	// Verify participant was deleted from store
	deletedParticipant, _ := st.GetInterventionParticipant("test-delete-intervention-123")
	if deletedParticipant != nil {
		t.Error("Expected participant to be deleted from store")
	}
}

// MockMessagingServiceForIntervention for testing message sending in intervention tests
type MockMessagingServiceForIntervention struct {
	MessageSent   bool
	LastRecipient string
	LastMessage   string
}

func NewMockMessagingServiceForIntervention() *MockMessagingServiceForIntervention {
	return &MockMessagingServiceForIntervention{}
}

func (m *MockMessagingServiceForIntervention) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	return recipient, nil
}

func (m *MockMessagingServiceForIntervention) SendMessage(ctx context.Context, to string, body string) error {
	m.MessageSent = true
	m.LastRecipient = to
	m.LastMessage = body
	return nil
}

func (m *MockMessagingServiceForIntervention) Start(ctx context.Context) error {
	return nil
}

func (m *MockMessagingServiceForIntervention) Stop() error {
	return nil
}

func (m *MockMessagingServiceForIntervention) Receipts() <-chan models.Receipt {
	return make(<-chan models.Receipt)
}

func (m *MockMessagingServiceForIntervention) Responses() <-chan models.Response {
	return make(<-chan models.Response)
}
