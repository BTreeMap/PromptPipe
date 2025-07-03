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
		respHandler: messaging.NewResponseHandler(mockMsgService),
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
		respHandler: messaging.NewResponseHandler(mockMsgService),
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
		respHandler: messaging.NewResponseHandler(mockMsgService),
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
