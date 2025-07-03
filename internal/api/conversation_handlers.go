// Package api provides conversation participant management handlers for PromptPipe endpoints.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// enrollConversationParticipantHandler handles POST /conversation/participants
func (s *Server) enrollConversationParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("enrollConversationParticipantHandler invoked", "method", r.Method, "path", r.URL.Path)

	var req models.ConversationEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("enrollConversationParticipantHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("enrollConversationParticipantHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Validate and canonicalize phone number
	canonicalPhone, err := s.msgService.ValidateAndCanonicalizeRecipient(req.PhoneNumber)
	if err != nil {
		slog.Warn("enrollConversationParticipantHandler phone validation failed", "error", err, "phone", req.PhoneNumber)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid phone number: "+err.Error()))
		return
	}

	// Check if participant already exists
	existing, err := s.st.GetConversationParticipantByPhone(canonicalPhone)
	if err != nil {
		slog.Error("enrollConversationParticipantHandler check existing failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check existing participant"))
		return
	}
	if existing != nil {
		slog.Warn("enrollConversationParticipantHandler participant already exists", "phone", canonicalPhone, "id", existing.ID)
		writeJSONResponse(w, http.StatusConflict, models.Error("Participant with this phone number already enrolled"))
		return
	}

	// Generate participant ID
	participantID, err := generateParticipantID()
	if err != nil {
		slog.Error("enrollConversationParticipantHandler ID generation failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate participant ID"))
		return
	}

	// Create participant
	now := time.Now()
	participant := models.ConversationParticipant{
		ID:          participantID,
		PhoneNumber: canonicalPhone,
		Name:        req.Name,
		Gender:      req.Gender,
		Ethnicity:   req.Ethnicity,
		Background:  req.Background,
		Status:      models.ConversationStatusActive,
		EnrolledAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Save participant
	if err := s.st.SaveConversationParticipant(participant); err != nil {
		slog.Error("enrollConversationParticipantHandler save failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to enroll participant"))
		return
	}

	// Initialize flow state to CONVERSATION_ACTIVE
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, models.StateConversationActive); err != nil {
		slog.Error("enrollConversationParticipantHandler state init failed", "error", err, "participantID", participantID)
		// Note: We don't fail the enrollment if state init fails, but we log it
	}

	// Register response hook for this participant
	conversationPrompt := models.Prompt{
		To:         canonicalPhone,
		Type:       models.PromptTypeConversation,
		UserPrompt: "", // Will be set when processing responses
		State:      models.StateConversationActive,
	}
	conversationHook := messaging.CreateConversationHook(conversationPrompt, s.msgService)
	if err := s.respHandler.RegisterHook(canonicalPhone, conversationHook); err != nil {
		slog.Error("enrollConversationParticipantHandler hook registration failed", "error", err, "participantID", participantID)
		// Note: We don't fail the enrollment if hook registration fails, but we log it
	} else {
		slog.Debug("Conversation hook registered", "participantID", participantID, "phone", canonicalPhone)
	}

	// Generate and send the first LLM message using the conversation flow
	firstMessage, err := s.generateFirstConversationMessage(ctx, participantID, participant)
	if err != nil {
		slog.Error("enrollConversationParticipantHandler first message generation failed", "error", err, "participantID", participantID)
		// Send a fallback welcome message
		firstMessage = "Hello! Welcome to our conversation. I'm here to chat with you. Feel free to share anything on your mind!"
	}

	if err := s.msgService.SendMessage(ctx, canonicalPhone, firstMessage); err != nil {
		slog.Error("enrollConversationParticipantHandler first message send failed", "error", err, "participantID", participantID)
		// Don't fail enrollment if first message fails to send
	} else {
		slog.Info("First conversation message sent", "participantID", participantID, "phone", canonicalPhone)
	}

	slog.Info("Conversation participant enrolled successfully", "id", participantID, "phone", canonicalPhone)
	writeJSONResponse(w, http.StatusCreated, models.SuccessWithMessage("Conversation participant enrolled successfully", participant))
}

// generateFirstConversationMessage generates the first LLM message for a new conversation participant
func (s *Server) generateFirstConversationMessage(ctx context.Context, participantID string, participant models.ConversationParticipant) (string, error) {
	// Get the conversation flow generator from the flow registry
	generator, exists := flow.Get(models.PromptTypeConversation)
	if !exists {
		slog.Error("generateFirstConversationMessage: conversation flow generator not registered")
		return "", fmt.Errorf("conversation flow generator not registered")
	}

	// Cast to conversation flow
	conversationFlow, ok := generator.(*flow.ConversationFlow)
	if !ok {
		slog.Error("generateFirstConversationMessage: invalid generator type for conversation flow")
		return "", fmt.Errorf("invalid generator type for conversation flow")
	}

	// Build a context-aware system prompt that incorporates participant background
	contextPrompt := buildWelcomePrompt(participant)

	// For the very first message, we can use the conversation flow's ProcessResponse with a special initialization message
	// This ensures the conversation history is properly initialized and the LLM generates an appropriate response
	response, err := conversationFlow.ProcessResponse(ctx, participantID, contextPrompt)
	if err != nil {
		slog.Error("generateFirstConversationMessage ProcessResponse failed", "error", err, "participantID", participantID)
		return "", err
	}

	return response, nil
}

// buildWelcomePrompt creates a contextual prompt for the first conversation message
func buildWelcomePrompt(participant models.ConversationParticipant) string {
	prompt := "SYSTEM_CONTEXT: This is the first interaction with a new conversation participant."

	if participant.Name != "" {
		prompt += fmt.Sprintf(" The participant's name is %s.", participant.Name)
	}

	if participant.Gender != "" {
		prompt += fmt.Sprintf(" Gender: %s.", participant.Gender)
	}

	if participant.Ethnicity != "" {
		prompt += fmt.Sprintf(" Ethnicity: %s.", participant.Ethnicity)
	}

	if participant.Background != "" {
		prompt += fmt.Sprintf(" Background info: %s.", participant.Background)
	}

	prompt += " Please generate a warm, personalized welcome message to start our conversation. Keep it natural and engaging."

	return prompt
}

// listConversationParticipantsHandler handles GET /conversation/participants
func (s *Server) listConversationParticipantsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("listConversationParticipantsHandler invoked", "method", r.Method, "path", r.URL.Path)

	participants, err := s.st.ListConversationParticipants()
	if err != nil {
		slog.Error("listConversationParticipantsHandler failed", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to list participants"))
		return
	}

	slog.Debug("listConversationParticipantsHandler succeeded", "count", len(participants))
	writeJSONResponse(w, http.StatusOK, models.Success(participants))
}

// getConversationParticipantHandler handles GET /conversation/participants/{id}
func (s *Server) getConversationParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("getConversationParticipantHandler invoked", "participantID", participantID)

	participant, err := s.st.GetConversationParticipant(participantID)
	if err != nil {
		slog.Error("getConversationParticipantHandler failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to get participant"))
		return
	}

	if participant == nil {
		slog.Debug("getConversationParticipantHandler not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	slog.Debug("getConversationParticipantHandler succeeded", "participantID", participantID)
	writeJSONResponse(w, http.StatusOK, models.Success(participant))
}

// updateConversationParticipantHandler handles PUT /conversation/participants/{id}
func (s *Server) updateConversationParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("updateConversationParticipantHandler invoked", "participantID", participantID)

	var req models.ConversationParticipantUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("updateConversationParticipantHandler invalid JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		slog.Warn("updateConversationParticipantHandler validation failed", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Check if participant exists
	participant, err := s.st.GetConversationParticipant(participantID)
	if err != nil {
		slog.Error("updateConversationParticipantHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("updateConversationParticipantHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Apply updates
	if req.Name != nil {
		participant.Name = *req.Name
	}
	if req.Gender != nil {
		participant.Gender = *req.Gender
	}
	if req.Ethnicity != nil {
		participant.Ethnicity = *req.Ethnicity
	}
	if req.Background != nil {
		participant.Background = *req.Background
	}
	if req.Status != nil {
		participant.Status = *req.Status
	}

	participant.UpdatedAt = time.Now()

	// Save updated participant
	if err := s.st.SaveConversationParticipant(*participant); err != nil {
		slog.Error("updateConversationParticipantHandler save failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to update participant"))
		return
	}

	slog.Info("Conversation participant updated successfully", "id", participantID)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Participant updated successfully", participant))
}

// deleteConversationParticipantHandler handles DELETE /conversation/participants/{id}
func (s *Server) deleteConversationParticipantHandler(w http.ResponseWriter, r *http.Request) {
	participantID := r.Context().Value(ContextKeyParticipantID).(string)
	slog.Debug("deleteConversationParticipantHandler invoked", "participantID", participantID)

	// Check if participant exists
	participant, err := s.st.GetConversationParticipant(participantID)
	if err != nil {
		slog.Error("deleteConversationParticipantHandler check failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to check participant"))
		return
	}

	if participant == nil {
		slog.Debug("deleteConversationParticipantHandler participant not found", "participantID", participantID)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Participant not found"))
		return
	}

	// Unregister response hook
	if err := s.respHandler.UnregisterHook(participant.PhoneNumber); err != nil {
		slog.Warn("deleteConversationParticipantHandler hook unregistration failed", "error", err, "participantID", participantID)
		// Continue with deletion even if hook unregistration fails
	}

	// Delete flow state
	ctx := context.Background()
	stateManager := flow.NewStoreBasedStateManager(s.st)
	if err := stateManager.ResetState(ctx, participantID, models.FlowTypeConversation); err != nil {
		slog.Warn("deleteConversationParticipantHandler state deletion failed", "error", err, "participantID", participantID)
		// Continue with deletion even if state deletion fails
	}

	// Delete participant
	if err := s.st.DeleteConversationParticipant(participantID); err != nil {
		slog.Error("deleteConversationParticipantHandler delete failed", "error", err, "participantID", participantID)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to delete participant"))
		return
	}

	slog.Info("Conversation participant deleted successfully", "id", participantID)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Participant deleted successfully", nil))
}
