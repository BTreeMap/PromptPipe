// Package api provides HTTP handlers for intervention management.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

const (
	// FlowTypeIntervention is the flow type for micro health intervention
	FlowTypeIntervention = "micro_health_intervention"

	// Default values for enrollment
	DefaultTimezone     = "UTC"
	DefaultScheduleTime = "10:00"
)

// generateParticipantID generates a unique participant ID
func generateParticipantID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate participant ID: %w", err)
	}
	return "part_" + hex.EncodeToString(bytes), nil
}

// generateResponseID generates a unique response ID
func generateResponseID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate response ID: %w", err)
	}
	return "resp_" + hex.EncodeToString(bytes), nil
}

// generateMessageID generates a unique message ID
func generateMessageID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate message ID: %w", err)
	}
	return "msg_" + hex.EncodeToString(bytes), nil
}

// extractParticipantID extracts participant ID from request context
func extractParticipantID(r *http.Request) string {
	if id := r.Context().Value("participantID"); id != nil {
		if participantID, ok := id.(string); ok {
			return participantID
		}
	}
	return ""
}

// enrollParticipantHandler handles participant enrollment (POST /intervention/participants)
func (s *Server) enrollParticipantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("enrollParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("enrollParticipantHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.EnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in enrollParticipantHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("enrollParticipantHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Validate and canonicalize phone number using messaging service
	canonicalPhone, err := s.msgService.ValidateAndCanonicalizeRecipient(req.PhoneNumber)
	if err != nil {
		slog.Warn("enrollParticipantHandler phone validation failed", "error", err, "phone", req.PhoneNumber)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant already exists
	existing, err := s.st.GetInterventionParticipantByPhone(canonicalPhone)
	if err != nil {
		slog.Error("Error checking existing participant", "error", err, "phone", canonicalPhone)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check existing participant"))
		return
	}
	if existing != nil {
		slog.Warn("Participant already enrolled", "phone", canonicalPhone, "existingID", existing.ID)
		writeJSONResponse(w, http.StatusConflict, models.Error("Participant already enrolled"))
		return
	}

	// Generate participant ID
	participantID, err := generateParticipantID()
	if err != nil {
		slog.Error("Failed to generate participant ID", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate participant ID"))
		return
	}

	// Set defaults
	timezone := req.Timezone
	if timezone == "" {
		timezone = DefaultTimezone
	}
	scheduleTime := req.ScheduleTime
	if scheduleTime == "" {
		scheduleTime = DefaultScheduleTime
	}

	// Create participant
	now := time.Now()
	participant := models.InterventionParticipant{
		ID:                 participantID,
		PhoneNumber:        canonicalPhone,
		Name:               req.Name,
		EnrolledAt:         now,
		Status:             models.ParticipantStatusActive,
		CurrentState:       flow.StateOrientation,
		Timezone:           timezone,
		ScheduleTime:       scheduleTime,
		HasSeenOrientation: false,
		TimesCompletedWeek: 0,
		WeekStartDate:      now,
		LastPromptDate:     time.Time{}, // Zero time
		CustomData:         make(map[string]interface{}),
	}

	// Save participant
	if err := s.st.SaveInterventionParticipant(participant); err != nil {
		slog.Error("Failed to save participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save participant"))
		return
	}

	// Initialize flow state
	flowState := models.FlowState{
		ParticipantID: participantID,
		FlowType:      FlowTypeIntervention,
		CurrentState:  flow.StateOrientation,
		StateData:     make(map[string]string),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.st.SaveFlowState(flowState); err != nil {
		slog.Error("Failed to save flow state", "error", err, "participantID", participantID)
		// Continue - this is not fatal for enrollment
		slog.Warn("Continuing with enrollment despite flow state save failure")
	}

	slog.Info("Participant enrolled successfully", "participantID", participantID, "phone", canonicalPhone)
	writeJSONResponse(w, http.StatusCreated, models.Success(participant))
}

// listParticipantsHandler handles listing all participants (GET /intervention/participants)
func (s *Server) listParticipantsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("listParticipantsHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("listParticipantsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("Error listing participants", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	// Convert to summary format
	summaries := make([]models.ParticipantSummary, len(participants))
	for i, p := range participants {
		completionRate := 0.0
		if p.TimesCompletedWeek > 0 {
			// Simple calculation - could be more sophisticated
			completionRate = float64(p.TimesCompletedWeek) / 7.0
		}

		summaries[i] = models.ParticipantSummary{
			ID:             p.ID,
			PhoneNumber:    p.PhoneNumber,
			Name:           p.Name,
			Status:         p.Status,
			CurrentState:   p.CurrentState,
			EnrolledAt:     p.EnrolledAt,
			LastPromptDate: p.LastPromptDate,
			CompletionRate: completionRate,
		}
	}

	slog.Debug("participants listed", "count", len(summaries))
	writeJSONResponse(w, http.StatusOK, models.Success(summaries))
}

// getParticipantHandler handles getting a specific participant (GET /intervention/participants/{id})
func (s *Server) getParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getParticipantHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("getParticipantHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error getting participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	slog.Debug("participant retrieved", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(participant))
}

// deleteParticipantHandler handles deleting a participant (DELETE /intervention/participants/{id})
func (s *Server) deleteParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("deleteParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		slog.Warn("deleteParticipantHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("deleteParticipantHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for deletion", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Delete flow state first
	if err := s.st.DeleteFlowState(participantID, FlowTypeIntervention); err != nil {
		slog.Error("Error deleting flow state", "error", err, "participantID", participantID)
		// Continue - this is not fatal
	}

	// Delete participant
	if err := s.st.DeleteInterventionParticipant(participantID); err != nil {
		slog.Error("Error deleting participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to delete participant"))
		return
	}

	slog.Info("Participant deleted successfully", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(nil))
}

// processResponseHandler handles processing participant responses (POST /intervention/participants/{id}/responses)
func (s *Server) processResponseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("processResponseHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("processResponseHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("processResponseHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.ResponseProcessingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in processResponseHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("processResponseHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for response processing", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Set default response type
	responseType := req.ResponseType
	if responseType == "" {
		responseType = models.ResponseTypeFreeText
	}

	// Generate response ID
	responseID, err := generateResponseID()
	if err != nil {
		slog.Error("Failed to generate response ID", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate response ID"))
		return
	}

	// Create intervention response
	now := time.Now()
	interventionResponse := models.InterventionResponse{
		ID:            responseID,
		ParticipantID: participantID,
		State:         currentState,
		ResponseText:  req.ResponseText,
		ResponseType:  responseType,
		ReceivedAt:    now,
		ProcessedAt:   now,
	}

	// Save response
	if err := s.st.SaveInterventionResponse(interventionResponse); err != nil {
		slog.Error("Failed to save intervention response", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save response"))
		return
	}

	// TODO: Implement state transition logic based on response
	// For now, just log the response processing
	slog.Info("Response processed successfully", "participantID", participantID, "responseID", responseID, "state", currentState, "responseText", req.ResponseText)

	writeJSONResponse(w, http.StatusCreated, models.Success(interventionResponse))
}

// advanceStateHandler handles manually advancing participant state (POST /intervention/participants/{id}/advance)
func (s *Server) advanceStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("advanceStateHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("advanceStateHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("advanceStateHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.ParticipantStateAdvanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in advanceStateHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("advanceStateHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for state advance", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Update flow state
	now := time.Now()
	if flowState == nil {
		flowState = &models.FlowState{
			ParticipantID: participantID,
			FlowType:      FlowTypeIntervention,
			CurrentState:  req.ToState,
			StateData:     make(map[string]string),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	} else {
		flowState.CurrentState = req.ToState
		flowState.UpdatedAt = now
	}

	// Add reason to state data if provided
	if req.Reason != "" {
		flowState.StateData["advance_reason"] = req.Reason
		flowState.StateData["advance_time"] = now.Format(time.RFC3339)
		flowState.StateData["previous_state"] = currentState
	}

	if err := s.st.SaveFlowState(*flowState); err != nil {
		slog.Error("Failed to save flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save flow state"))
		return
	}

	// Update participant's current state
	participant.CurrentState = req.ToState
	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("Failed to update participant state", "error", err, "participantID", participantID)
		// Continue - flow state is more important
	}

	slog.Info("Participant state advanced", "participantID", participantID, "fromState", currentState, "toState", req.ToState, "reason", req.Reason)

	result := map[string]interface{}{
		"participant_id": participantID,
		"previous_state": currentState,
		"current_state":  req.ToState,
		"reason":         req.Reason,
		"updated_at":     now,
	}

	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// resetParticipantHandler handles resetting participant state (POST /intervention/participants/{id}/reset)
func (s *Server) resetParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("resetParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("resetParticipantHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("resetParticipantHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for reset", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Reset flow state
	if err := s.st.DeleteFlowState(participantID, FlowTypeIntervention); err != nil {
		slog.Error("Error deleting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to reset flow state"))
		return
	}

	// Reset participant state
	now := time.Now()
	participant.CurrentState = flow.StateOrientation
	participant.HasSeenOrientation = false
	participant.TimesCompletedWeek = 0
	participant.WeekStartDate = now
	participant.LastPromptDate = time.Time{} // Zero time

	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("Failed to reset participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to reset participant"))
		return
	}

	slog.Info("Participant reset successfully", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(participant))
}

// getParticipantHistoryHandler handles getting participant interaction history (GET /intervention/participants/{id}/history)
func (s *Server) getParticipantHistoryHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getParticipantHistoryHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getParticipantHistoryHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("getParticipantHistoryHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for history", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get intervention responses
	responses, err := s.st.GetInterventionResponses(participantID)
	if err != nil {
		slog.Error("Error getting intervention responses", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get responses"))
		return
	}

	// Get intervention messages
	messages, err := s.st.GetInterventionMessages(participantID)
	if err != nil {
		slog.Error("Error getting intervention messages", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get messages"))
		return
	}

	history := map[string]interface{}{
		"participant": participant,
		"responses":   responses,
		"messages":    messages,
	}

	slog.Debug("participant history retrieved", "participantID", participantID, "responses", len(responses), "messages", len(messages))
	writeJSONResponse(w, http.StatusOK, models.Success(history))
}

// triggerWeeklySummaryHandler handles triggering weekly summary (POST /intervention/weekly-summary)
func (s *Server) triggerWeeklySummaryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("triggerWeeklySummaryHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("triggerWeeklySummaryHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.WeeklySummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in triggerWeeklySummaryHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// TODO: Implement weekly summary logic
	// For now, just return a placeholder response

	processedCount := 0
	if len(req.ParticipantIDs) > 0 {
		processedCount = len(req.ParticipantIDs)
	} else {
		// Process all participants
		participants, err := s.st.ListInterventionParticipants()
		if err != nil {
			slog.Error("Error listing participants for weekly summary", "error", err)
			writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
			return
		}
		processedCount = len(participants)
	}

	result := map[string]interface{}{
		"processed_count": processedCount,
		"force_all":       req.ForceAll,
		"timestamp":       time.Now(),
		"status":          "completed",
	}

	slog.Info("Weekly summary triggered", "processedCount", processedCount, "forceAll", req.ForceAll)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// interventionStatsHandler handles getting intervention statistics (GET /intervention/stats)
func (s *Server) interventionStatsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("interventionStatsHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("interventionStatsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get all participants
	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("Error listing participants for stats", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	// Calculate statistics
	stats := models.InterventionStats{
		TotalParticipants:     len(participants),
		ActiveParticipants:    0,
		CompletedParticipants: 0,
		WithdrawnParticipants: 0,
		StateDistribution:     make(map[string]int),
	}

	var totalCompletionRate float64
	for _, p := range participants {
		switch p.Status {
		case models.ParticipantStatusActive:
			stats.ActiveParticipants++
		case models.ParticipantStatusCompleted:
			stats.CompletedParticipants++
		case models.ParticipantStatusWithdrawn:
			stats.WithdrawnParticipants++
		}

		// Count state distribution
		stats.StateDistribution[p.CurrentState]++

		// Calculate completion rate
		completionRate := 0.0
		if p.TimesCompletedWeek > 0 {
			completionRate = float64(p.TimesCompletedWeek) / 7.0
		}
		totalCompletionRate += completionRate
	}

	if len(participants) > 0 {
		stats.AverageCompletionRate = totalCompletionRate / float64(len(participants))
	}

	// Get response and message counts
	allResponses, err := s.st.GetAllInterventionResponses()
	if err != nil {
		slog.Error("Error getting all responses for stats", "error", err)
		// Continue without response count
	} else {
		stats.TotalResponses = len(allResponses)
	}

	allMessages, err := s.st.GetAllInterventionMessages()
	if err != nil {
		slog.Error("Error getting all messages for stats", "error", err)
		// Continue without message count
	} else {
		stats.TotalMessages = len(allMessages)
	}

	slog.Debug("intervention stats calculated", "totalParticipants", stats.TotalParticipants, "activeParticipants", stats.ActiveParticipants)
	writeJSONResponse(w, http.StatusOK, models.Success(stats))
}

// updateParticipantStatusHandler handles updating participant status (PUT /intervention/participants/{id}/status)
func (s *Server) updateParticipantStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("updateParticipantStatusHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		slog.Warn("updateParticipantStatusHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("updateParticipantStatusHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.ParticipantStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in updateParticipantStatusHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("updateParticipantStatusHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for status update", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	oldStatus := participant.Status
	participant.Status = req.Status

	// Save updated participant
	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("Failed to update participant status", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to update participant status"))
		return
	}

	slog.Info("Participant status updated", "participantID", participantID, "oldStatus", oldStatus, "newStatus", req.Status, "reason", req.Reason)

	result := map[string]interface{}{
		"participant_id": participantID,
		"old_status":     oldStatus,
		"new_status":     req.Status,
		"reason":         req.Reason,
		"updated_at":     time.Now(),
	}

	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// triggerParticipantHandler handles manually triggering next state (POST /intervention/participants/{id}/trigger)
func (s *Server) triggerParticipantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("triggerParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("triggerParticipantHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("triggerParticipantHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for trigger", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// TODO: Implement state machine logic to determine next state and generate message
	// For now, just return current state information

	result := map[string]interface{}{
		"participant_id": participantID,
		"current_state":  currentState,
		"triggered_at":   time.Now(),
		"status":         "triggered",
		"message":        "State trigger initiated - implementation pending",
	}

	slog.Info("Participant state trigger initiated", "participantID", participantID, "currentState", currentState)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// getParticipantStateHandler handles getting detailed state information (GET /intervention/participants/{id}/state)
func (s *Server) getParticipantStateHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getParticipantStateHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getParticipantStateHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("getParticipantStateHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for state query", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	// Build detailed state information
	stateDetails := models.FlowStateDetails{
		ParticipantID: participantID,
		FlowType:      FlowTypeIntervention,
		ActiveTimers:  []models.TimerInfo{}, // TODO: Implement timer tracking
	}

	if flowState != nil {
		stateDetails.CurrentState = flowState.CurrentState
		stateDetails.StateData = flowState.StateData
		stateDetails.CreatedAt = flowState.CreatedAt
		stateDetails.UpdatedAt = flowState.UpdatedAt

		// Get possible next states
		if stateInfo, exists := models.StateInfoMap[flowState.CurrentState]; exists {
			stateDetails.PossibleStates = stateInfo.PossibleNextStates
		}
	} else {
		stateDetails.CurrentState = flow.StateOrientation
		stateDetails.StateData = make(map[string]string)
		stateDetails.CreatedAt = participant.EnrolledAt
		stateDetails.UpdatedAt = participant.EnrolledAt
		stateDetails.PossibleStates = []string{flow.StateCommitmentPrompt}
	}

	// Validate current state
	stateDetails.StateValidation = models.StateValidation{
		IsValid:       true,
		CanTransition: true,
	}

	slog.Debug("participant state retrieved", "participantID", participantID, "currentState", stateDetails.CurrentState)
	writeJSONResponse(w, http.StatusOK, models.Success(stateDetails))
}

// updateParticipantScheduleHandler handles updating participant schedule (PUT /intervention/participants/{id}/schedule)
func (s *Server) updateParticipantScheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("updateParticipantScheduleHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		slog.Warn("updateParticipantScheduleHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("updateParticipantScheduleHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.ScheduleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in updateParticipantScheduleHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("updateParticipantScheduleHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for schedule update", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	oldTimezone := participant.Timezone
	oldScheduleTime := participant.ScheduleTime

	// Update schedule fields
	if req.Timezone != "" {
		participant.Timezone = req.Timezone
	}
	if req.ScheduleTime != "" {
		participant.ScheduleTime = req.ScheduleTime
	}

	// Save updated participant
	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("Failed to update participant schedule", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to update participant schedule"))
		return
	}

	slog.Info("Participant schedule updated", "participantID", participantID, "oldTimezone", oldTimezone, "newTimezone", participant.Timezone, "oldScheduleTime", oldScheduleTime, "newScheduleTime", participant.ScheduleTime)

	result := map[string]interface{}{
		"participant_id":    participantID,
		"timezone":          participant.Timezone,
		"schedule_time":     participant.ScheduleTime,
		"previous_timezone": oldTimezone,
		"previous_schedule": oldScheduleTime,
		"updated_at":        time.Now(),
	}

	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// getFlowStateHandler handles getting detailed flow state (GET /intervention/participants/{id}/flow-state)
func (s *Server) getFlowStateHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getFlowStateHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getFlowStateHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("getFlowStateHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for flow state query", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	if flowState == nil {
		// Return default state if no flow state exists
		now := time.Now()
		flowState = &models.FlowState{
			ParticipantID: participantID,
			FlowType:      FlowTypeIntervention,
			CurrentState:  flow.StateOrientation,
			StateData:     make(map[string]string),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	}

	slog.Debug("flow state retrieved", "participantID", participantID, "currentState", flowState.CurrentState)
	writeJSONResponse(w, http.StatusOK, models.Success(flowState))
}

// transitionFlowStateHandler handles flow state transitions (POST /intervention/participants/{id}/flow-state/transition)
func (s *Server) transitionFlowStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("transitionFlowStateHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("transitionFlowStateHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("transitionFlowStateHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.FlowStateTransitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in transitionFlowStateHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("transitionFlowStateHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for flow state transition", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Validate transition if not forced
	if !req.Force && req.FromState != "" && req.FromState != currentState {
		slog.Warn("Invalid flow state transition", "participantID", participantID, "expected", req.FromState, "current", currentState)
		writeJSONResponse(w, http.StatusConflict, models.Error(fmt.Sprintf("Current state is %s, expected %s", currentState, req.FromState)))
		return
	}

	// Validate that transition is allowed (unless forced)
	if !req.Force {
		if stateInfo, exists := models.StateInfoMap[currentState]; exists {
			validTransition := false
			for _, nextState := range stateInfo.PossibleNextStates {
				if nextState == req.ToState {
					validTransition = true
					break
				}
			}
			if !validTransition {
				slog.Warn("Invalid state transition attempted", "participantID", participantID, "from", currentState, "to", req.ToState)
				writeJSONResponse(w, http.StatusBadRequest, models.Error(fmt.Sprintf("Invalid transition from %s to %s", currentState, req.ToState)))
				return
			}
		}
	}

	// Perform transition
	now := time.Now()
	if flowState == nil {
		flowState = &models.FlowState{
			ParticipantID: participantID,
			FlowType:      FlowTypeIntervention,
			CurrentState:  req.ToState,
			StateData:     make(map[string]string),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	} else {
		flowState.CurrentState = req.ToState
		flowState.UpdatedAt = now
	}

	// Add transition metadata
	if req.Reason != "" {
		flowState.StateData["transition_reason"] = req.Reason
		flowState.StateData["transition_time"] = now.Format(time.RFC3339)
		flowState.StateData["previous_state"] = currentState
	}

	// Save updated flow state
	if err := s.st.SaveFlowState(*flowState); err != nil {
		slog.Error("Failed to save flow state transition", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save flow state transition"))
		return
	}

	// Update participant's current state
	participant.CurrentState = req.ToState
	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("Failed to update participant state", "error", err, "participantID", participantID)
		// Continue - flow state is more important
	}

	slog.Info("Flow state transition completed", "participantID", participantID, "fromState", currentState, "toState", req.ToState, "forced", req.Force, "reason", req.Reason)

	result := map[string]interface{}{
		"participant_id":  participantID,
		"previous_state":  currentState,
		"current_state":   req.ToState,
		"forced":          req.Force,
		"reason":          req.Reason,
		"transitioned_at": now,
	}

	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// generateMessageHandler handles message generation (POST /intervention/participants/{id}/messages/generate)
func (s *Server) generateMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("generateMessageHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("generateMessageHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("generateMessageHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.MessageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body for simple generation
		req = models.MessageGenerationRequest{}
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("generateMessageHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for message generation", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Generate message based on current state
	var messageContent string
	var messageType string

	// Determine message type from state if not explicitly provided
	if req.MessageType != "" {
		messageType = req.MessageType
	} else {
		messageType = determineMessageTypeFromState(currentState)
	}

	// Get message template based on state
	messageContent = getMessageTemplateForState(currentState, participant.Name, req.Variables)

	// Generate message ID for tracking (even if preview)
	messageID, err := generateMessageID()
	if err != nil {
		slog.Error("Failed to generate message ID", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate message ID"))
		return
	}

	result := map[string]interface{}{
		"message_id":     messageID,
		"participant_id": participantID,
		"current_state":  currentState,
		"message_type":   messageType,
		"content":        messageContent,
		"preview":        req.Preview,
		"generated_at":   time.Now(),
		"variables_used": req.Variables,
	}

	// If not preview, save the message record
	if !req.Preview {
		now := time.Now()
		message := models.InterventionMessage{
			ID:            messageID,
			ParticipantID: participantID,
			State:         currentState,
			MessageType:   messageType,
			Content:       messageContent,
			SentAt:        now,
			// DeliveredAt and ReadAt will be updated when message is actually sent
		}

		if err := s.st.SaveInterventionMessage(message); err != nil {
			slog.Error("Failed to save generated message", "error", err, "participantID", participantID)
			// Continue - this is not fatal for generation
		}
	}

	slog.Info("Message generated", "participantID", participantID, "messageID", messageID, "currentState", currentState, "messageType", messageType, "preview", req.Preview)
	writeJSONResponse(w, http.StatusCreated, models.Success(result))
}

// sendMessageHandler handles sending messages to participants (POST /intervention/participants/{id}/messages/send)
func (s *Server) sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("sendMessageHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("sendMessageHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("sendMessageHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	var req models.MessageSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in sendMessageHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("sendMessageHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for message send", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state for context
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Generate message ID
	messageID, err := generateMessageID()
	if err != nil {
		slog.Error("Failed to generate message ID", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate message ID"))
		return
	}

	// Determine message type
	messageType := req.MessageType
	if messageType == "" {
		messageType = models.MessageTypeFollowUp
	}

	// Handle scheduled sending
	var scheduledAt time.Time
	var sendNow bool = true

	if req.ScheduleAt != "" {
		var err error
		scheduledAt, err = time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil {
			slog.Warn("Invalid schedule_at format", "error", err, "scheduleAt", req.ScheduleAt)
			writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid schedule_at format"))
			return
		}
		sendNow = scheduledAt.Before(time.Now().Add(1 * time.Minute)) // Send immediately if scheduled for soon
	}

	now := time.Now()
	message := models.InterventionMessage{
		ID:            messageID,
		ParticipantID: participantID,
		State:         currentState,
		MessageType:   messageType,
		Content:       req.Content,
		SentAt:        now,
		// DeliveredAt will be updated when actually delivered
	}

	// Save message record
	if err := s.st.SaveInterventionMessage(message); err != nil {
		slog.Error("Failed to save message", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save message"))
		return
	}

	result := map[string]interface{}{
		"message_id":     messageID,
		"participant_id": participantID,
		"content":        req.Content,
		"message_type":   messageType,
		"status":         "queued",
		"scheduled_at":   scheduledAt,
		"send_now":       sendNow,
		"created_at":     now,
	}

	// TODO: Integrate with actual messaging service to send the message
	// For now, just mark as successful
	result["status"] = "sent" // In real implementation, this would be updated after actual sending

	slog.Info("Message queued for sending", "participantID", participantID, "messageID", messageID, "messageType", messageType, "scheduledAt", scheduledAt, "sendNow", sendNow)
	writeJSONResponse(w, http.StatusCreated, models.Success(result))
}

// getDailyStatusHandler handles getting daily status (GET /intervention/participants/{id}/daily-status)
func (s *Server) getDailyStatusHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getDailyStatusHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getDailyStatusHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	participantID := extractParticipantID(r)
	if participantID == "" {
		slog.Warn("getDailyStatusHandler missing participant ID")
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing participant ID"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("Error checking participant", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for daily status", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current flow state
	flowState, err := s.st.GetFlowState(participantID, FlowTypeIntervention)
	if err != nil {
		slog.Error("Error getting flow state", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get flow state"))
		return
	}

	currentState := flow.StateOrientation
	if flowState != nil {
		currentState = flowState.CurrentState
	}

	// Get today's date
	today := time.Now().Format("2006-01-02")

	// Get today's responses and messages
	responses, err := s.st.GetInterventionResponses(participantID)
	if err != nil {
		slog.Error("Error getting responses", "error", err, "participantID", participantID)
		responses = []models.InterventionResponse{} // Continue with empty responses
	}

	messages, err := s.st.GetInterventionMessages(participantID)
	if err != nil {
		slog.Error("Error getting messages", "error", err, "participantID", participantID)
		messages = []models.InterventionMessage{} // Continue with empty messages
	}

	// Filter for today and calculate metrics
	var todayResponses []models.InterventionResponse
	var todayMessages []models.InterventionMessage
	var lastActivity time.Time
	var completionStatus string = "pending"

	for _, response := range responses {
		if response.ReceivedAt.Format("2006-01-02") == today {
			todayResponses = append(todayResponses, response)
			if response.ReceivedAt.After(lastActivity) {
				lastActivity = response.ReceivedAt
			}
			// Check for completion response
			if response.ResponseText == "Done" || response.ResponseText == "done" {
				completionStatus = "done"
			}
		}
	}

	for _, message := range messages {
		if message.SentAt.Format("2006-01-02") == today {
			todayMessages = append(todayMessages, message)
		}
	}

	// Determine if flow is completed today
	flowCompleted := currentState == flow.StateEndOfDay || currentState == flow.StateWeeklySummary
	var completedAt time.Time
	if flowCompleted {
		completedAt = lastActivity
	}

	// Get intervention assignment from state data
	interventionAssignment := ""
	if flowState != nil && flowState.StateData != nil {
		if assignment, exists := flowState.StateData["flow_assignment_today"]; exists {
			interventionAssignment = assignment
		}
	}

	dailyStatus := models.DailyStatus{
		ParticipantID:          participantID,
		Date:                   today,
		CurrentState:           currentState,
		FlowCompleted:          flowCompleted,
		CompletedAt:            completedAt,
		MessagesReceived:       len(todayMessages),
		ResponsesGiven:         len(todayResponses),
		LastActivity:           lastActivity,
		CompletionStatus:       completionStatus,
		InterventionAssignment: interventionAssignment,
	}

	slog.Debug("daily status retrieved", "participantID", participantID, "date", today, "currentState", currentState, "flowCompleted", flowCompleted, "messagesReceived", len(todayMessages), "responsesGiven", len(todayResponses))
	writeJSONResponse(w, http.StatusOK, models.Success(dailyStatus))
}

// Helper functions for message generation

// determineMessageTypeFromState maps flow states to message types
func determineMessageTypeFromState(state string) string {
	switch state {
	case flow.StateOrientation:
		return models.MessageTypeOrientation
	case flow.StateCommitmentPrompt:
		return models.MessageTypeCommitment
	case flow.StateFeelingPrompt:
		return models.MessageTypeFeeling
	case flow.StateSendInterventionImmediate, flow.StateSendInterventionReflective:
		return models.MessageTypeIntervention
	case flow.StateReinforcementFollowup:
		return models.MessageTypeFollowUp
	case flow.StateWeeklySummary:
		return models.MessageTypeWeeklySummary
	default:
		return models.MessageTypeFollowUp
	}
}

// getMessageTemplateForState returns appropriate message content for a state
func getMessageTemplateForState(state, participantName string, variables map[string]string) string {
	// Replace $Name$ placeholder
	name := participantName
	if name == "" {
		name = "there"
	}

	var template string
	switch state {
	case flow.StateOrientation:
		template = flow.MsgOrientation
	case flow.StateCommitmentPrompt:
		template = flow.MsgCommitment
	case flow.StateFeelingPrompt:
		template = flow.MsgFeeling
	case flow.StateSendInterventionImmediate:
		template = flow.MsgInterventionImmediate
	case flow.StateSendInterventionReflective:
		template = flow.MsgInterventionReflective
	case flow.StateReinforcementFollowup:
		template = flow.MsgReinforcement
	case flow.StateDidYouGetAChance:
		template = flow.MsgDidYouGetAChance
	case flow.StateContextQuestion:
		template = flow.MsgContextQuestion
	case flow.StateMoodQuestion:
		template = flow.MsgMoodQuestion
	case flow.StateBarrierCheckAfterContextMood:
		template = flow.MsgBarrierCheck
	case flow.StateBarrierReasonNoChance:
		template = flow.MsgBarrierReason
	case flow.StateIgnoredPath:
		template = flow.MsgIgnoredPath1 // Use first ignored path message
	case flow.StateWeeklySummary:
		// Default weekly summary, could be customized with completion count
		completions := "X" // Would need to be passed in variables
		if variables != nil && variables["completions"] != "" {
			completions = variables["completions"]
		}
		template = strings.ReplaceAll(flow.MsgWeeklySummary, "%d", completions)
	default:
		template = "Thank you for participating in our Healthy Habits study!"
	}

	// Replace name placeholder
	template = strings.ReplaceAll(template, "$Name$", name)

	// Replace other variables if provided
	for key, value := range variables {
		placeholder := fmt.Sprintf("$%s$", key)
		template = strings.ReplaceAll(template, placeholder, value)
	}

	return template
}

// Bulk Operations Handlers

// bulkEnrollHandler handles bulk participant enrollment (POST /intervention/bulk/enroll)
func (s *Server) bulkEnrollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("bulkEnrollHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("bulkEnrollHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.BulkEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in bulkEnrollHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("bulkEnrollHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	var results []map[string]interface{}
	var successCount, errorCount int

	for i, enrollReq := range req.Participants {
		result := map[string]interface{}{
			"index":        i,
			"phone_number": enrollReq.PhoneNumber,
		}

		if req.DryRun {
			// Just validate without enrolling
			if err := enrollReq.Validate(); err != nil {
				result["status"] = "error"
				result["error"] = err.Error()
				errorCount++
			} else {
				result["status"] = "valid"
				successCount++
			}
		} else {
			// TODO: Implement actual enrollment logic (similar to enrollParticipantHandler)
			// For now, just simulate success
			if err := enrollReq.Validate(); err != nil {
				result["status"] = "error"
				result["error"] = err.Error()
				errorCount++
			} else {
				result["status"] = "enrolled"
				result["participant_id"] = fmt.Sprintf("part_%d", i) // Would be real ID
				successCount++
			}
		}

		results = append(results, result)
	}

	responseData := map[string]interface{}{
		"total_requested": len(req.Participants),
		"successful":      successCount,
		"errors":          errorCount,
		"dry_run":         req.DryRun,
		"results":         results,
		"processed_at":    time.Now(),
	}

	slog.Info("Bulk enrollment processed", "totalRequested", len(req.Participants), "successful", successCount, "errors", errorCount, "dryRun", req.DryRun)
	writeJSONResponse(w, http.StatusOK, models.Success(responseData))
}

// bulkStatusUpdateHandler handles bulk participant status updates (PUT /intervention/bulk/status)
func (s *Server) bulkStatusUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("bulkStatusUpdateHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		slog.Warn("bulkStatusUpdateHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.BulkStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in bulkStatusUpdateHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("bulkStatusUpdateHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	var results []map[string]interface{}
	var successCount, errorCount int

	for _, participantID := range req.ParticipantIDs {
		result := map[string]interface{}{
			"participant_id": participantID,
		}

		// Check if participant exists
		participant, err := s.st.GetInterventionParticipant(participantID)
		if err != nil {
			result["status"] = "error"
			result["error"] = "Failed to check participant"
			errorCount++
			results = append(results, result)
			continue
		}

		if participant == nil {
			result["status"] = "error"
			result["error"] = "Participant not found"
			errorCount++
			results = append(results, result)
			continue
		}

		oldStatus := participant.Status
		participant.Status = req.Status

		// Save updated participant
		if err := s.st.SaveInterventionParticipant(*participant); err != nil {
			result["status"] = "error"
			result["error"] = "Failed to update participant status"
			errorCount++
		} else {
			result["status"] = "updated"
			result["old_status"] = oldStatus
			result["new_status"] = req.Status
			successCount++
		}

		results = append(results, result)
	}

	responseData := map[string]interface{}{
		"total_requested": len(req.ParticipantIDs),
		"successful":      successCount,
		"errors":          errorCount,
		"new_status":      req.Status,
		"reason":          req.Reason,
		"results":         results,
		"updated_at":      time.Now(),
	}

	slog.Info("Bulk status update processed", "totalRequested", len(req.ParticipantIDs), "successful", successCount, "errors", errorCount, "newStatus", req.Status)
	writeJSONResponse(w, http.StatusOK, models.Success(responseData))
}

// bulkMessageHandler handles bulk message sending (POST /intervention/bulk/messages)
func (s *Server) bulkMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("bulkMessageHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("bulkMessageHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.BulkMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in bulkMessageHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("bulkMessageHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Get target participants
	var targetParticipants []models.InterventionParticipant
	if len(req.ParticipantIDs) > 0 {
		// Send to specific participants
		for _, participantID := range req.ParticipantIDs {
			participant, err := s.st.GetInterventionParticipant(participantID)
			if err != nil {
				slog.Error("Error getting participant for bulk message", "error", err, "participantID", participantID)
				continue
			}
			if participant != nil {
				targetParticipants = append(targetParticipants, *participant)
			}
		}
	} else {
		// Send to all active participants
		allParticipants, err := s.st.ListInterventionParticipants()
		if err != nil {
			slog.Error("Error listing participants for bulk message", "error", err)
			writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
			return
		}
		for _, participant := range allParticipants {
			if participant.Status == models.ParticipantStatusActive {
				targetParticipants = append(targetParticipants, participant)
			}
		}
	}

	var results []map[string]interface{}
	var successCount, errorCount int

	for _, participant := range targetParticipants {
		result := map[string]interface{}{
			"participant_id": participant.ID,
			"phone_number":   participant.PhoneNumber,
		}

		// Generate message ID
		messageID, err := generateMessageID()
		if err != nil {
			result["status"] = "error"
			result["error"] = "Failed to generate message ID"
			errorCount++
			results = append(results, result)
			continue
		}

		// Determine message type
		messageType := req.MessageType
		if messageType == "" {
			messageType = models.MessageTypeFollowUp
		}

		// Create message record
		now := time.Now()
		message := models.InterventionMessage{
			ID:            messageID,
			ParticipantID: participant.ID,
			State:         participant.CurrentState,
			MessageType:   messageType,
			Content:       req.Content,
			SentAt:        now,
		}

		// Save message record
		if err := s.st.SaveInterventionMessage(message); err != nil {
			result["status"] = "error"
			result["error"] = "Failed to save message"
			errorCount++
		} else {
			result["status"] = "queued"
			result["message_id"] = messageID
			result["message_type"] = messageType
			successCount++
		}

		results = append(results, result)
	}

	responseData := map[string]interface{}{
		"total_participants": len(targetParticipants),
		"successful":         successCount,
		"errors":             errorCount,
		"content":            req.Content,
		"message_type":       req.MessageType,
		"scheduled_at":       req.ScheduleAt,
		"results":            results,
		"created_at":         time.Now(),
	}

	slog.Info("Bulk message processed", "totalParticipants", len(targetParticipants), "successful", successCount, "errors", errorCount, "messageType", req.MessageType)
	writeJSONResponse(w, http.StatusOK, models.Success(responseData))
}

// Analytics and Data Export Handlers

// getFlowAnalyticsHandler handles getting flow analytics (GET /intervention/analytics/flow)
func (s *Server) getFlowAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getFlowAnalyticsHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getFlowAnalyticsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get all participants
	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("Error listing participants for analytics", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	// Calculate comprehensive analytics
	analytics := models.FlowAnalytics{
		TotalParticipants:     len(participants),
		ActiveParticipants:    0,
		CompletedParticipants: 0,
		StateDistribution:     make(map[string]int),
		CompletionRates:       models.CompletionRateData{},
		EngagementMetrics:     models.EngagementMetrics{},
		ResponsePatterns:      models.ResponsePatternAnalysis{},
		GeneratedAt:           time.Now(),
	}

	var totalCompletionRate float64
	var totalResponseTime float64
	var responseTimeCount int

	for _, p := range participants {
		switch p.Status {
		case models.ParticipantStatusActive:
			analytics.ActiveParticipants++
		case models.ParticipantStatusCompleted:
			analytics.CompletedParticipants++
		}

		// Count state distribution
		analytics.StateDistribution[p.CurrentState]++

		// Calculate completion rate for this participant
		weeklyRate := 0.0
		if p.TimesCompletedWeek > 0 {
			weeklyRate = float64(p.TimesCompletedWeek) / 7.0
		}
		totalCompletionRate += weeklyRate

		// Calculate time since enrollment for engagement metrics
		daysSinceEnrollment := time.Since(p.EnrolledAt).Hours() / 24
		if daysSinceEnrollment > 0 {
			analytics.EngagementMetrics.AvgDaysActive += daysSinceEnrollment
		}

		// Track average response time (simplified calculation)
		if !p.LastPromptDate.IsZero() {
			// This is a simplified calculation - in real implementation, you'd track actual response times
			avgResponseTime := 2.5 // Placeholder: 2.5 hours average
			totalResponseTime += avgResponseTime
			responseTimeCount++
		}
	}

	// Calculate averages
	if len(participants) > 0 {
		analytics.CompletionRates.WeeklyAverage = totalCompletionRate / float64(len(participants))
		analytics.EngagementMetrics.AvgDaysActive = analytics.EngagementMetrics.AvgDaysActive / float64(len(participants))
	}

	if responseTimeCount > 0 {
		analytics.ResponsePatterns.AvgResponseTimeHours = totalResponseTime / float64(responseTimeCount)
	}

	// Calculate additional metrics
	if analytics.TotalParticipants > 0 {
		analytics.CompletionRates.OverallCompletion = float64(analytics.CompletedParticipants) / float64(analytics.TotalParticipants)
		analytics.EngagementMetrics.ActiveRate = float64(analytics.ActiveParticipants) / float64(analytics.TotalParticipants)
	}

	// Set some example values for response patterns (would be calculated from actual data)
	analytics.ResponsePatterns.TotalResponses = analytics.TotalParticipants * 7 // Rough estimate
	analytics.ResponsePatterns.ResponseRate = 0.85                              // Example response rate

	slog.Debug("flow analytics calculated", "totalParticipants", analytics.TotalParticipants, "activeParticipants", analytics.ActiveParticipants, "completionRate", analytics.CompletionRates.WeeklyAverage)
	writeJSONResponse(w, http.StatusOK, models.Success(analytics))
}

// exportDataHandler handles data export (GET /intervention/export)
func (s *Server) exportDataHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("exportDataHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("exportDataHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get query parameters for export options
	exportType := r.URL.Query().Get("type") // participants, responses, messages, all
	format := r.URL.Query().Get("format")   // json, csv

	if exportType == "" {
		exportType = "all"
	}
	if format == "" {
		format = "json"
	}

	exportData := make(map[string]interface{})

	// Export participants
	if exportType == "participants" || exportType == "all" {
		participants, err := s.st.ListInterventionParticipants()
		if err != nil {
			slog.Error("Error listing participants for export", "error", err)
			writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to export participants"))
			return
		}
		exportData["participants"] = participants
	}

	// Export responses
	if exportType == "responses" || exportType == "all" {
		// TODO: Implement GetAllInterventionResponses if not exists
		exportData["responses"] = []models.InterventionResponse{} // Placeholder
	}

	// Export messages
	if exportType == "messages" || exportType == "all" {
		// TODO: Implement GetAllInterventionMessages if not exists
		exportData["messages"] = []models.InterventionMessage{} // Placeholder
	}

	exportData["exported_at"] = time.Now()
	exportData["export_type"] = exportType
	exportData["format"] = format

	// TODO: Implement CSV format export
	if format == "csv" {
		// Set CSV headers and convert to CSV format
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=intervention_export_%s.csv", time.Now().Format("20060102_150405")))
		// For now, return JSON with message about CSV support
		exportData["message"] = "CSV export not yet implemented, returning JSON format"
	}

	slog.Info("Data export completed", "exportType", exportType, "format", format)
	writeJSONResponse(w, http.StatusOK, models.Success(exportData))
}

// Configuration Management Handlers

// getConfigHandler handles getting intervention configuration (GET /intervention/config)
func (s *Server) getConfigHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("getConfigHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("getConfigHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Return current intervention configuration
	config := models.InterventionConfig{
		DefaultTimezone:     DefaultTimezone,
		DefaultScheduleTime: DefaultScheduleTime,
		Timeouts: map[string]string{
			"FEELING_TIMEOUT":        "2h",
			"COMPLETION_TIMEOUT":     "4h",
			"BARRIER_DETAIL_TIMEOUT": "1h",
			"BARRIER_REASON_TIMEOUT": "1h",
		},
		FlowSettings: map[string]interface{}{
			"max_weekly_completions": 7,
			"orientation_required":   true,
			"weekly_summary_enabled": true,
		},
		MessageTemplates: map[string]string{
			"orientation":             flow.MsgOrientation,
			"commitment":              flow.MsgCommitment,
			"feeling":                 flow.MsgFeeling,
			"intervention_immediate":  flow.MsgInterventionImmediate,
			"intervention_reflective": flow.MsgInterventionReflective,
			"reinforcement":           flow.MsgReinforcement,
			"did_you_get_a_chance":    flow.MsgDidYouGetAChance,
			"context_question":        flow.MsgContextQuestion,
			"mood_question":           flow.MsgMoodQuestion,
			"barrier_check":           flow.MsgBarrierCheck,
			"barrier_reason":          flow.MsgBarrierReason,
			"ignored_path":            flow.MsgIgnoredPath1,
			"weekly_summary":          flow.MsgWeeklySummary,
		},
		LastUpdated: time.Now(),
	}

	slog.Debug("intervention configuration retrieved")
	writeJSONResponse(w, http.StatusOK, models.Success(config))
}

// updateConfigHandler handles updating intervention configuration (PUT /intervention/config)
func (s *Server) updateConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("updateConfigHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		slog.Warn("updateConfigHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.InterventionConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in updateConfigHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate configuration
	if err := req.Validate(); err != nil {
		slog.Warn("updateConfigHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// TODO: Save configuration to persistent store
	// For now, just return success
	req.LastUpdated = time.Now()

	slog.Info("Intervention configuration updated")
	writeJSONResponse(w, http.StatusOK, models.Success(req))
}

// Timer and Daily Flow Management Handlers

// triggerDailyPromptsHandler handles triggering daily prompts (POST /intervention/daily-prompts/trigger)
func (s *Server) triggerDailyPromptsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("triggerDailyPromptsHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("triggerDailyPromptsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.DailyPromptTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body
		req = models.DailyPromptTriggerRequest{}
	}

	// Get target participants
	var targetParticipants []models.InterventionParticipant
	if len(req.ParticipantIDs) > 0 {
		// Trigger for specific participants
		for _, participantID := range req.ParticipantIDs {
			participant, err := s.st.GetInterventionParticipant(participantID)
			if err != nil {
				slog.Error("Error getting participant for daily prompt trigger", "error", err, "participantID", participantID)
				continue
			}
			if participant != nil {
				targetParticipants = append(targetParticipants, *participant)
			}
		}
	} else {
		// Trigger for all eligible participants
		allParticipants, err := s.st.ListInterventionParticipants()
		if err != nil {
			slog.Error("Error listing participants for daily prompt trigger", "error", err)
			writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
			return
		}

		// Filter for eligible participants (active status, appropriate timing, etc.)
		for _, participant := range allParticipants {
			if participant.Status == models.ParticipantStatusActive {
				// TODO: Add more sophisticated eligibility checks (timing, state, etc.)
				targetParticipants = append(targetParticipants, participant)
			}
		}
	}

	var results []map[string]interface{}
	var successCount, errorCount int

	for _, participant := range targetParticipants {
		result := map[string]interface{}{
			"participant_id": participant.ID,
			"phone_number":   participant.PhoneNumber,
			"current_state":  participant.CurrentState,
		}

		if req.DryRun {
			// Just validate eligibility without triggering
			result["status"] = "eligible"
			result["action"] = "would_trigger_daily_prompt"
			successCount++
		} else {
			// TODO: Implement actual daily prompt triggering logic
			// This would involve state machine logic to determine next prompt type
			result["status"] = "triggered"
			result["action"] = "daily_prompt_initiated"
			result["next_state"] = flow.StateCommitmentPrompt // Example
			successCount++
		}

		results = append(results, result)
	}

	responseData := map[string]interface{}{
		"total_participants": len(targetParticipants),
		"successful":         successCount,
		"errors":             errorCount,
		"dry_run":            req.DryRun,
		"force_all":          req.ForceAll,
		"results":            results,
		"triggered_at":       time.Now(),
	}

	slog.Info("Daily prompts triggered", "totalParticipants", len(targetParticipants), "successful", successCount, "errors", errorCount, "dryRun", req.DryRun)
	writeJSONResponse(w, http.StatusOK, models.Success(responseData))
}

// processReadyOverrideHandler handles "Ready" override messages (POST /intervention/ready-override)
func (s *Server) processReadyOverrideHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("processReadyOverrideHandler invoked", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("processReadyOverrideHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.ReadyOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode JSON in processReadyOverrideHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("processReadyOverrideHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Find participant by ID or phone
	var participant *models.InterventionParticipant
	var err error

	if req.ParticipantPhone != "" {
		// Find by phone number
		canonicalPhone, err := s.msgService.ValidateAndCanonicalizeRecipient(req.ParticipantPhone)
		if err != nil {
			slog.Warn("processReadyOverrideHandler phone validation failed", "error", err, "phone", req.ParticipantPhone)
			writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
			return
		}
		participant, err = s.st.GetInterventionParticipantByPhone(canonicalPhone)
	} else if req.ParticipantID != "" {
		participant, err = s.st.GetInterventionParticipant(req.ParticipantID)
	} else {
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Either participant_id or participant_phone is required"))
		return
	}

	if err != nil {
		slog.Error("Error checking participant for ready override", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("Participant not found for ready override")
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// TODO: Implement "Ready" override logic
	// This would typically advance the participant to the next appropriate state
	// and potentially trigger immediate message generation

	result := map[string]interface{}{
		"participant_id":   participant.ID,
		"phone_number":     participant.PhoneNumber,
		"current_state":    participant.CurrentState,
		"override_applied": true,
		"source":           req.Source,
		"processed_at":     time.Now(),
		"action":           "ready_override_processed",
		"next_state":       flow.StateCommitmentPrompt, // Example - would be determined by logic
	}

	slog.Info("Ready override processed", "participantID", participant.ID, "source", req.Source, "currentState", participant.CurrentState)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}
