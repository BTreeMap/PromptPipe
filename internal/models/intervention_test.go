package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestInterventionEnrollmentRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request InterventionEnrollmentRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with all fields",
			request: InterventionEnrollmentRequest{
				PhoneNumber:     "+1234567890",
				Name:            "John Doe",
				Timezone:        "America/New_York",
				DailyPromptTime: "10:00",
			},
			wantErr: false,
		},
		{
			name: "valid request with minimal fields",
			request: InterventionEnrollmentRequest{
				PhoneNumber: "+1234567890",
			},
			wantErr: false,
		},
		{
			name: "missing phone number",
			request: InterventionEnrollmentRequest{
				Name: "John Doe",
			},
			wantErr: true,
			errMsg:  "phone_number is required",
		},
		{
			name: "invalid timezone",
			request: InterventionEnrollmentRequest{
				PhoneNumber: "+1234567890",
				Timezone:    "Invalid/Timezone",
			},
			wantErr: true,
			errMsg:  "invalid timezone",
		},
		{
			name: "invalid daily prompt time format",
			request: InterventionEnrollmentRequest{
				PhoneNumber:     "+1234567890",
				DailyPromptTime: "25:00", // Invalid hour
			},
			wantErr: true,
			errMsg:  "daily_prompt_time must be in HH:MM format",
		},
		{
			name: "invalid daily prompt time format 2",
			request: InterventionEnrollmentRequest{
				PhoneNumber:     "+1234567890",
				DailyPromptTime: "not-a-time",
			},
			wantErr: true,
			errMsg:  "daily_prompt_time must be in HH:MM format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v; want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestIsValidParticipantStatus(t *testing.T) {
	tests := []struct {
		status   InterventionParticipantStatus
		expected bool
	}{
		{ParticipantStatusActive, true},
		{ParticipantStatusPaused, true},
		{ParticipantStatusCompleted, true},
		{ParticipantStatusWithdrawn, true},
		{InterventionParticipantStatus("invalid"), false},
		{InterventionParticipantStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := IsValidParticipantStatus(tt.status)
			if result != tt.expected {
				t.Errorf("IsValidParticipantStatus(%v) = %v; want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestInterventionParticipantJSONMarshaling(t *testing.T) {
	now := time.Now()
	participant := InterventionParticipant{
		ID:              "p_123",
		PhoneNumber:     "+1234567890",
		Name:            "John Doe",
		Timezone:        "America/New_York",
		Status:          ParticipantStatusActive,
		EnrolledAt:      now,
		DailyPromptTime: "10:00",
		WeeklyReset:     now.AddDate(0, 0, 7),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(participant)
	if err != nil {
		t.Fatalf("Error marshaling InterventionParticipant to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaledParticipant InterventionParticipant
	err = json.Unmarshal(jsonData, &unmarshaledParticipant)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON to InterventionParticipant: %v", err)
	}

	// Compare key fields
	if unmarshaledParticipant.ID != participant.ID {
		t.Errorf("ID mismatch: got %v, want %v", unmarshaledParticipant.ID, participant.ID)
	}
	if unmarshaledParticipant.PhoneNumber != participant.PhoneNumber {
		t.Errorf("PhoneNumber mismatch: got %v, want %v", unmarshaledParticipant.PhoneNumber, participant.PhoneNumber)
	}
	if unmarshaledParticipant.Status != participant.Status {
		t.Errorf("Status mismatch: got %v, want %v", unmarshaledParticipant.Status, participant.Status)
	}
}

func TestInterventionResponseJSONMarshaling(t *testing.T) {
	now := time.Now()
	response := InterventionResponse{
		ID:            "r_456",
		ParticipantID: "p_123",
		State:         "COMMITMENT_PROMPT",
		ResponseText:  "1",
		ResponseType:  "commitment",
		Timestamp:     now,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Error marshaling InterventionResponse to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaledResponse InterventionResponse
	err = json.Unmarshal(jsonData, &unmarshaledResponse)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON to InterventionResponse: %v", err)
	}

	// Compare key fields
	if unmarshaledResponse.ID != response.ID {
		t.Errorf("ID mismatch: got %v, want %v", unmarshaledResponse.ID, response.ID)
	}
	if unmarshaledResponse.ParticipantID != response.ParticipantID {
		t.Errorf("ParticipantID mismatch: got %v, want %v", unmarshaledResponse.ParticipantID, response.ParticipantID)
	}
	if unmarshaledResponse.ResponseText != response.ResponseText {
		t.Errorf("ResponseText mismatch: got %v, want %v", unmarshaledResponse.ResponseText, response.ResponseText)
	}
}
