package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

func TestEnrollConversationParticipant(t *testing.T) {
	// Create in-memory store
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create mock messaging service
	mockMsgService := messaging.NewWhatsAppService(whatsapp.NewMockClient())

	// Create server
	server := &Server{
		msgService:  mockMsgService,
		respHandler: messaging.NewResponseHandler(mockMsgService, st),
		st:          st,
		timer:       flow.NewSimpleTimer(),
		gaClient:    nil, // GenAI client can be nil for this test
	}

	// Initialize conversation flow
	err := server.initializeConversationFlow()
	if err != nil {
		t.Fatalf("Failed to initialize conversation flow: %v", err)
	}

	// Test enrollment request
	enrollReq := models.ConversationEnrollmentRequest{
		PhoneNumber: "+1234567890",
		Name:        "Test User",
		Gender:      "non-binary",
		Ethnicity:   "Mixed",
		Background:  "Interested in mental health and technology",
	}

	reqBody, err := json.Marshal(enrollReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/conversation/participants", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Call handler
	server.enrollConversationParticipantHandler(rec, req)

	// Check response
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, rec.Code)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Parse response
	var response models.APIResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Status != string(models.APIStatusOK) {
		t.Errorf("Expected status=%s, got status=%s, message=%s", models.APIStatusOK, response.Status, response.Message)
	}

	// Verify participant was saved
	participants, err := st.ListConversationParticipants()
	if err != nil {
		t.Fatalf("Failed to list participants: %v", err)
	}

	if len(participants) != 1 {
		t.Errorf("Expected 1 participant, got %d", len(participants))
	}

	participant := participants[0]
	// Phone number gets normalized, so we should expect the normalized version
	if participant.PhoneNumber != "1234567890" {
		t.Errorf("Expected phone number 1234567890, got %s", participant.PhoneNumber)
	}
	if participant.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got %s", participant.Name)
	}
	if participant.Gender != "non-binary" {
		t.Errorf("Expected gender 'non-binary', got %s", participant.Gender)
	}
	if participant.Status != models.ConversationStatusActive {
		t.Errorf("Expected status active, got %s", participant.Status)
	}
}

func TestListConversationParticipants(t *testing.T) {
	// Create in-memory store
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create mock messaging service
	mockMsgService := messaging.NewWhatsAppService(whatsapp.NewMockClient())

	// Create server
	server := &Server{
		msgService:  mockMsgService,
		respHandler: messaging.NewResponseHandler(mockMsgService, st),
		st:          st,
		timer:       flow.NewSimpleTimer(),
	}

	// Add test participants
	now := time.Now()
	participant1 := models.ConversationParticipant{
		ID:          "test-001",
		PhoneNumber: "+1111111111",
		Name:        "Alice",
		Status:      models.ConversationStatusActive,
		EnrolledAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	participant2 := models.ConversationParticipant{
		ID:          "test-002",
		PhoneNumber: "+2222222222",
		Name:        "Bob",
		Status:      models.ConversationStatusPaused,
		EnrolledAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := st.SaveConversationParticipant(participant1)
	if err != nil {
		t.Fatalf("Failed to save participant1: %v", err)
	}
	err = st.SaveConversationParticipant(participant2)
	if err != nil {
		t.Fatalf("Failed to save participant2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/conversation/participants", nil)
	rec := httptest.NewRecorder()

	// Call handler
	server.listConversationParticipantsHandler(rec, req)

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Parse response
	var response models.APIResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Status != string(models.APIStatusOK) {
		t.Errorf("Expected status=%s, got status=%s", models.APIStatusOK, response.Status)
	}

	// Check that we got 2 participants back
	participantsData, ok := response.Result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be a slice, got %T", response.Result)
	}

	if len(participantsData) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(participantsData))
	}
}

func TestGetConversationParticipant(t *testing.T) {
	// Create in-memory store
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create mock messaging service
	mockMsgService := messaging.NewWhatsAppService(whatsapp.NewMockClient())

	// Create server
	server := &Server{
		msgService:  mockMsgService,
		respHandler: messaging.NewResponseHandler(mockMsgService, st),
		st:          st,
		timer:       flow.NewSimpleTimer(),
	}

	// Add test participant
	now := time.Now()
	participant := models.ConversationParticipant{
		ID:          "test-123",
		PhoneNumber: "+1234567890",
		Name:        "Test User",
		Gender:      "female",
		Ethnicity:   "Hispanic",
		Background:  "Software engineer interested in AI",
		Status:      models.ConversationStatusActive,
		EnrolledAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := st.SaveConversationParticipant(participant)
	if err != nil {
		t.Fatalf("Failed to save participant: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/conversation/participants/test-123", nil)
	ctx := context.WithValue(req.Context(), ContextKeyParticipantID, "test-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	// Call handler
	server.getConversationParticipantHandler(rec, req)

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Parse response
	var response models.APIResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Status != string(models.APIStatusOK) {
		t.Errorf("Expected status=%s, got status=%s", models.APIStatusOK, response.Status)
	}

	// Verify participant data
	participantMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}

	if participantMap["id"] != "test-123" {
		t.Errorf("Expected id 'test-123', got %v", participantMap["id"])
	}
	if participantMap["name"] != "Test User" {
		t.Errorf("Expected name 'Test User', got %v", participantMap["name"])
	}
	if participantMap["gender"] != "female" {
		t.Errorf("Expected gender 'female', got %v", participantMap["gender"])
	}
}

// TestDeleteConversationParticipant tests the deletion of a conversation participant with unregister notification
func TestDeleteConversationParticipant(t *testing.T) {
	// Create in-memory store
	st := store.NewInMemoryStore()
	defer st.Close()

	// Create mock messaging service
	mockMessaging := NewMockMessagingService()
	
	// Create the API server
	server := &Server{
		st: st,
		msgService: mockMessaging,
		respHandler: messaging.NewResponseHandler(mockMessaging, st),
	}

	// Add a test participant to the store
	participant := models.ConversationParticipant{
		ID:          "test-delete-123",
		PhoneNumber: "+1234567890",
		Name:        "Test Delete User",
		Status:      models.ConversationStatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := st.SaveConversationParticipant(participant)
	if err != nil {
		t.Fatalf("Failed to save test participant: %v", err)
	}

	// Create a mock request
	req := httptest.NewRequest("DELETE", "/conversation/participants/test-delete-123", nil)
	req = req.WithContext(context.WithValue(req.Context(), ContextKeyParticipantID, "test-delete-123"))
	
	// Create a response recorder
	w := httptest.NewRecorder()
	
	// Call the handler
	server.deleteConversationParticipantHandler(w, req)

	// Check the response status
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify that the notification message was sent
	if !mockMessaging.MessageSent {
		t.Error("Expected unregister notification to be sent")
	}

	expectedMsg := "You have been unregistered from the conversation experiment by the organizer. If you have any questions, please contact the organizer for assistance. Thank you for your participation."
	if mockMessaging.LastMessage != expectedMsg {
		t.Errorf("Expected notification message '%s', got '%s'", expectedMsg, mockMessaging.LastMessage)
	}

	if mockMessaging.LastRecipient != "+1234567890" {
		t.Errorf("Expected notification to be sent to '+1234567890', got '%s'", mockMessaging.LastRecipient)
	}

	// Verify participant was deleted from store
	deletedParticipant, _ := st.GetConversationParticipant("test-delete-123")
	if deletedParticipant != nil {
		t.Error("Expected participant to be deleted from store")
	}
}

// MockMessagingService for testing message sending
type MockMessagingService struct {
	MessageSent   bool
	LastRecipient string
	LastMessage   string
}

func NewMockMessagingService() *MockMessagingService {
	return &MockMessagingService{}
}

func (m *MockMessagingService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	return recipient, nil
}

func (m *MockMessagingService) SendMessage(ctx context.Context, to string, body string) error {
	m.MessageSent = true
	m.LastRecipient = to
	m.LastMessage = body
	return nil
}

func (m *MockMessagingService) Start(ctx context.Context) error {
	return nil
}

func (m *MockMessagingService) Stop() error {
	return nil
}

func (m *MockMessagingService) Receipts() <-chan models.Receipt {
	return make(<-chan models.Receipt)
}

func (m *MockMessagingService) Responses() <-chan models.Response {
	return make(<-chan models.Response)
}
