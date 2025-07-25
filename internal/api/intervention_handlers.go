// Package api provides intervention management handlers for PromptPipe endpoints.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/util"
)

// enrollParticipantHandler handles POST /intervention/participants
func (s *Server) enrollParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("enrollParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	var req models.InterventionEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("enrollParticipantHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("enrollParticipantHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Validate and canonicalize phone number
	canonicalPhone, err := s.msgService.ValidateAndCanonicalizeRecipient(req.PhoneNumber)
	if err != nil {
		slog.Warn("enrollParticipantHandler phone validation failed", "error", err, "phone", req.PhoneNumber)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid phone number: "+err.Error()))
		return
	}

	// Check if participant already exists
	existing, err := s.st.GetInterventionParticipantByPhone(canonicalPhone)
	if err != nil {
		slog.Error("enrollParticipantHandler check existing failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check existing participant"))
		return
	}
	if existing != nil {
		slog.Warn("enrollParticipantHandler participant already exists", "phone", canonicalPhone, "id", existing.ID)
		writeJSONResponse(w, http.StatusConflict, models.Error("Participant with this phone number already enrolled"))
		return
	}

	// Generate participant ID
	participantID, err := generateParticipantID()
	if err != nil {
		slog.Error("enrollParticipantHandler ID generation failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate participant ID"))
		return
	}

	// Set defaults
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	dailyPromptTime := req.DailyPromptTime
	if dailyPromptTime == "" {
		dailyPromptTime = "10:00"
	}

	// Create participant
	now := time.Now()
	participant := models.InterventionParticipant{
		ID:              participantID,
		PhoneNumber:     canonicalPhone,
		Name:            req.Name,
		Timezone:        timezone,
		Status:          models.ParticipantStatusActive,
		EnrolledAt:      now,
		DailyPromptTime: dailyPromptTime,
		WeeklyReset:     now.AddDate(0, 0, 7), // 7 days from enrollment
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Save participant
	if err := s.st.SaveInterventionParticipant(participant); err != nil {
		slog.Error("enrollParticipantHandler save failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to enroll participant"))
		return
	}

	// Initialize flow state to ORIENTATION
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateOrientation); err != nil {
		slog.Error("enrollParticipantHandler state init failed", "error", err, "participantID", participantID)
		// Note: We don't fail the enrollment if state init fails, but we log it
	}

	// Register persistent response hook for this participant
	hookParams := map[string]string{
		"participant_id": participantID,
		"phone_number":   canonicalPhone,
	}
	if err := s.respHandler.RegisterPersistentHook(canonicalPhone, models.HookTypeIntervention, hookParams); err != nil {
		slog.Error("enrollParticipantHandler persistent hook registration failed", "error", err, "participantID", participantID)
		// Note: We don't fail the enrollment if hook registration fails, but we log it
	} else {
		slog.Debug("Persistent intervention hook registered", "participantID", participantID, "phone", canonicalPhone)
	}

	// Send immediate welcome message
	welcomeMsg := fmt.Sprintf("🌱 Welcome to our Healthy Habits study! You've been enrolled successfully. You'll receive your daily prompts at %s %s. You can also type 'Ready' anytime to get a prompt immediately. Your participation is valuable!", dailyPromptTime, timezone)
	if err := s.msgService.SendMessage(ctx, canonicalPhone, welcomeMsg); err != nil {
		slog.Error("enrollParticipantHandler welcome message failed", "error", err, "participantID", participantID)
		// Don't fail enrollment if welcome message fails
	} else {
		slog.Info("Welcome message sent", "participantID", participantID, "phone", canonicalPhone)
	}

	slog.Info("Participant enrolled successfully", "id", participantID, "phone", canonicalPhone)
	writeJSONResponse(w, http.StatusCreated, models.SuccessWithMessage("Participant enrolled successfully", participant))
}

// listParticipantsHandler handles GET /intervention/participants
func (s *Server) listParticipantsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("listParticipantsHandler invoked", "method", r.Method, "path", r.URL.Path)

	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("listParticipantsHandler failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	slog.Debug("listParticipantsHandler succeeded", "count", len(participants))
	writeJSONResponse(w, http.StatusOK, models.Success(participants))
}

// getParticipantHandler handles GET /intervention/participants/{id}
func (s *Server) getParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("getParticipantHandler invoked", "participantID", participantID)

	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("getParticipantHandler failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant"))
		return
	}

	if participant == nil {
		slog.Debug("getParticipantHandler not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	slog.Debug("getParticipantHandler succeeded", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(participant))
}

// updateParticipantHandler handles PUT /intervention/participants/{id}
func (s *Server) updateParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("updateParticipantHandler invoked", "participantID", participantID)

	var req models.InterventionParticipantUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("updateParticipantHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("updateParticipantHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("updateParticipantHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("updateParticipantHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Track what fields are being updated for notification message
	var updatedFields []string
	oldDailyPromptTime := participant.DailyPromptTime

	// Update fields if provided
	if req.Name != nil {
		participant.Name = *req.Name
		updatedFields = append(updatedFields, "name")
	}

	if req.Timezone != nil {
		participant.Timezone = *req.Timezone
		updatedFields = append(updatedFields, "timezone")
	}

	if req.DailyPromptTime != nil {
		participant.DailyPromptTime = *req.DailyPromptTime
		updatedFields = append(updatedFields, "daily_prompt_time")
	}

	if req.Status != nil {
		participant.Status = *req.Status
		updatedFields = append(updatedFields, "status")
	}

	// Update timestamp
	participant.UpdatedAt = time.Now()

	// Save updated participant
	if err := s.st.SaveInterventionParticipant(*participant); err != nil {
		slog.Error("updateParticipantHandler save failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to update participant"))
		return
	}

	// Send notification message if daily prompt time was updated
	if req.DailyPromptTime != nil && *req.DailyPromptTime != oldDailyPromptTime {
		ctx := context.Background()
		notificationMsg := fmt.Sprintf("📅 Your daily prompt time has been updated to %s. You'll receive your next prompt at this new time!", *req.DailyPromptTime)

		if err := s.msgService.SendMessage(ctx, participant.PhoneNumber, notificationMsg); err != nil {
			slog.Error("updateParticipantHandler notification failed", "error", err, "participantID", participantID)
			// Don't fail the update if notification fails
		} else {
			slog.Info("Update notification sent", "participantID", participantID, "newTime", *req.DailyPromptTime)
		}
	}

	slog.Info("Participant updated successfully", "participantID", participantID, "updatedFields", updatedFields)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Participant updated successfully", participant))
}

// deleteParticipantHandler handles DELETE /intervention/participants/{id}
func (s *Server) deleteParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("deleteParticipantHandler invoked", "participantID", participantID)

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("deleteParticipantHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("deleteParticipantHandler not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Send unregister notification to participant
	notificationCtx := context.Background()
	unregisterMsg := "You have been unregistered from the micro health intervention experiment by the organizer. If you have any questions, please contact the organizer for assistance. Thank you for your participation."
	if err := s.msgService.SendMessage(notificationCtx, participant.PhoneNumber, unregisterMsg); err != nil {
		slog.Error("deleteParticipantHandler notification failed", "error", err, "participantID", participantID, "phone", participant.PhoneNumber)
		// Continue with deletion even if notification fails
	} else {
		slog.Info("Unregister notification sent", "participantID", participantID, "phone", participant.PhoneNumber)
	}

	// Delete participant (this will cascade delete their responses via foreign key)
	if err := s.st.DeleteInterventionParticipant(participantID); err != nil {
		slog.Error("deleteParticipantHandler delete failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to delete participant"))
		return
	}

	// Also clean up their flow state
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	if err := stateManager.ResetState(ctx, participantID, models.FlowTypeMicroHealthIntervention); err != nil {
		slog.Error("deleteParticipantHandler state cleanup failed", "error", err, "participantID", participantID)
		// Note: We don't fail the delete if state cleanup fails
	}

	// Unregister persistent response hook
	if err := s.respHandler.UnregisterPersistentHook(participant.PhoneNumber); err != nil {
		slog.Error("deleteParticipantHandler persistent hook cleanup failed", "error", err, "participantID", participantID)
		// Note: We don't fail the delete if hook cleanup fails
	} else {
		slog.Debug("Persistent response hook unregistered", "participantID", participantID, "phone", participant.PhoneNumber)
	}

	slog.Info("Participant deleted successfully", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Participant deleted successfully", nil))
}

// processResponseHandler handles POST /intervention/participants/{id}/responses
func (s *Server) processResponseHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("processResponseHandler invoked", "participantID", participantID)

	var req models.InterventionResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("processResponseHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	if req.ResponseText == "" {
		slog.Warn("processResponseHandler empty response", "participantID", participantID)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("response_text is required"))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("processResponseHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("processResponseHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current state
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		slog.Error("processResponseHandler get state failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant state"))
		return
	}

	// Generate response ID
	responseID, err := generateResponseID()
	if err != nil {
		slog.Error("processResponseHandler ID generation failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate response ID"))
		return
	}

	// Determine response type based on current state
	responseType := determineResponseType(currentState)

	// Save the response
	response := models.InterventionResponse{
		ID:            responseID,
		ParticipantID: participantID,
		State:         string(currentState),
		ResponseText:  req.ResponseText,
		ResponseType:  responseType,
		Timestamp:     time.Now(),
	}

	if err := s.st.SaveInterventionResponse(response); err != nil {
		slog.Error("processResponseHandler save response failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to save response"))
		return
	}

	// TODO: Process the response and advance state based on the micro health intervention logic
	// This would involve calling the MicroHealthInterventionGenerator.ProcessResponse method
	// For now, we just record the response

	// Create micro health intervention generator with dependencies
	timer := flow.NewSimpleTimer()
	generator := flow.NewMicroHealthInterventionGenerator(stateManager, timer)

	// Process the response through the flow logic
	if err := generator.ProcessResponse(ctx, participantID, req.ResponseText); err != nil {
		slog.Error("processResponseHandler flow processing failed", "error", err, "participantID", participantID)
		// Don't fail the API call if flow processing fails, but log it
	} else {
		slog.Debug("Response processed through flow logic", "participantID", participantID)
	}

	slog.Info("Response processed successfully", "participantID", participantID, "responseID", responseID, "state", currentState)
	writeJSONResponse(w, http.StatusCreated, models.Success(response))
}

// advanceStateHandler handles POST /intervention/participants/{id}/advance
func (s *Server) advanceStateHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("advanceStateHandler invoked", "participantID", participantID)

	var req models.InterventionStateAdvanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("advanceStateHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	if req.ToState == "" {
		slog.Warn("advanceStateHandler empty to_state", "participantID", participantID)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("to_state is required"))
		return
	}

	// Validate that the target state is valid
	if !isValidInterventionState(req.ToState) {
		slog.Warn("advanceStateHandler invalid state", "participantID", participantID, "toState", req.ToState)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid state: "+req.ToState))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("advanceStateHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("advanceStateHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get current state
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		slog.Error("advanceStateHandler get state failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant state"))
		return
	}

	// Advance state
	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateType(req.ToState)); err != nil {
		slog.Error("advanceStateHandler set state failed", "error", err, "participantID", participantID, "toState", req.ToState)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to advance state"))
		return
	}

	result := map[string]interface{}{
		"participant_id": participantID,
		"from_state":     string(currentState),
		"to_state":       req.ToState,
		"reason":         req.Reason,
		"advanced_at":    time.Now(),
	}

	slog.Info("State advanced successfully", "participantID", participantID, "from", currentState, "to", req.ToState, "reason", req.Reason)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// resetParticipantHandler handles POST /intervention/participants/{id}/reset
func (s *Server) resetParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("resetParticipantHandler invoked", "participantID", participantID)

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("resetParticipantHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("resetParticipantHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Reset flow state to ORIENTATION
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	if err := stateManager.ResetState(ctx, participantID, models.FlowTypeMicroHealthIntervention); err != nil {
		slog.Error("resetParticipantHandler reset failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to reset participant state"))
		return
	}

	// Set back to ORIENTATION
	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateOrientation); err != nil {
		slog.Error("resetParticipantHandler set orientation failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to set orientation state"))
		return
	}

	result := map[string]interface{}{
		"participant_id": participantID,
		"reset_to":       models.StateOrientation,
		"reset_at":       time.Now(),
	}

	slog.Info("Participant reset successfully", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// getParticipantHistoryHandler handles GET /intervention/participants/{id}/history
func (s *Server) getParticipantHistoryHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("getParticipantHistoryHandler invoked", "participantID", participantID)

	// Check if participant exists
	participant, err := s.st.GetInterventionParticipant(participantID)
	if err != nil {
		slog.Error("getParticipantHistoryHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("getParticipantHistoryHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Get participant responses
	responses, err := s.st.GetInterventionResponses(participantID)
	if err != nil {
		slog.Error("getParticipantHistoryHandler get responses failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant history"))
		return
	}

	// Get current flow state
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		slog.Error("getParticipantHistoryHandler get state failed", "error", err, "participantID", participantID)
		// Don't fail if we can't get current state
		currentState = "unknown"
	}

	history := map[string]interface{}{
		"participant":    participant,
		"current_state":  currentState,
		"responses":      responses,
		"response_count": len(responses),
	}

	slog.Debug("getParticipantHistoryHandler succeeded", "participantID", participantID, "responseCount", len(responses))
	writeJSONResponse(w, http.StatusOK, models.Success(history))
}

// triggerWeeklySummaryHandler handles POST /intervention/weekly-summary
func (s *Server) triggerWeeklySummaryHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("triggerWeeklySummaryHandler invoked", "method", r.Method, "path", r.URL.Path)

	// Check HTTP method
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("triggerWeeklySummaryHandler method not allowed", "method", r.Method)
		writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		return
	}

	// Get all active participants
	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("triggerWeeklySummaryHandler list failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	processed := 0
	now := time.Now()

	for _, participant := range participants {
		if participant.Status != models.ParticipantStatusActive {
			continue
		}

		// Check if weekly reset is due
		if participant.WeeklyReset.After(now) {
			continue
		}

		// TODO: Generate and send weekly summary message
		// This would involve:
		// 1. Calculating participant's progress for the week
		// 2. Generating a summary message
		// 3. Sending the message via msgService
		// 4. Updating the participant's WeeklyReset time

		slog.Debug("Weekly summary would be sent", "participantID", participant.ID, "phone", participant.PhoneNumber)
		processed++
	}

	result := map[string]interface{}{
		"participants_processed": processed,
		"triggered_at":           now,
	}

	slog.Info("Weekly summary trigger completed", "processed", processed)
	writeJSONResponse(w, http.StatusOK, models.Success(result))
}

// interventionStatsHandler handles GET /intervention/stats
func (s *Server) interventionStatsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("interventionStatsHandler invoked", "method", r.Method, "path", r.URL.Path)

	// Check HTTP method
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("interventionStatsHandler method not allowed", "method", r.Method)
		writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		return
	}

	// Get all participants
	participants, err := s.st.ListInterventionParticipants()
	if err != nil {
		slog.Error("interventionStatsHandler list participants failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participants"))
		return
	}

	// Get all responses
	allResponses, err := s.st.ListAllInterventionResponses()
	if err != nil {
		slog.Error("interventionStatsHandler list responses failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get responses"))
		return
	}

	// Calculate statistics
	stats := calculateInterventionStats(participants, allResponses)

	slog.Debug("interventionStatsHandler succeeded", "totalParticipants", stats.TotalParticipants, "totalResponses", stats.TotalResponses)
	writeJSONResponse(w, http.StatusOK, models.Success(stats))
}

// Helper functions

// generateParticipantID generates a unique participant ID
func generateParticipantID() (string, error) {
	return util.GenerateParticipantID(), nil
}

// generateResponseID generates a unique response ID
func generateResponseID() (string, error) {
	return util.GenerateResponseID(), nil
}

// determineResponseType determines the response type based on the current state
func determineResponseType(state models.StateType) string {
	switch state {
	case models.StateCommitmentPrompt:
		return "commitment"
	case models.StateFeelingPrompt:
		return "feeling"
	case models.StateHabitReminder:
		return "completion"
	case models.StateFollowUp:
		return "followup"
	default:
		return "general"
	}
}

// isValidInterventionState checks if a state is valid for the micro health intervention
func isValidInterventionState(state string) bool {
	validStates := []models.StateType{
		models.StateOrientation,
		models.StateCommitmentPrompt,
		models.StateFeelingPrompt,
		models.StateRandomAssignment,
		models.StateHabitReminder,
		models.StateFollowUp,
		models.StateComplete,
	}

	for _, validState := range validStates {
		if state == string(validState) {
			return true
		}
	}
	return false
}

// calculateInterventionStats calculates statistics for the intervention
func calculateInterventionStats(participants []models.InterventionParticipant, responses []models.InterventionResponse) models.InterventionStats {
	stats := models.InterventionStats{
		TotalParticipants:    len(participants),
		ParticipantsByStatus: make(map[models.InterventionParticipantStatus]int),
		ParticipantsByState:  make(map[string]int),
		TotalResponses:       len(responses),
		ResponsesByType:      make(map[string]int),
		CompletionRate:       0.0,
		AverageResponseTime:  0.0,
	}

	// Count participants by status
	completedCount := 0
	for _, participant := range participants {
		stats.ParticipantsByStatus[participant.Status]++
		if participant.Status == models.ParticipantStatusCompleted {
			completedCount++
		}
	}

	// Calculate completion rate
	if stats.TotalParticipants > 0 {
		stats.CompletionRate = float64(completedCount) / float64(stats.TotalParticipants) * 100.0
	}

	// Count responses by type
	for _, response := range responses {
		stats.ResponsesByType[response.ResponseType]++
	}

	// TODO: Calculate ParticipantsByState by querying current states
	// TODO: Calculate AverageResponseTime based on message send/response timestamps

	return stats
}
