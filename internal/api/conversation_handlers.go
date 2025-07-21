// Package api provides conversation participant management handlers for PromptPipe endpoints.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// enrollConversationParticipantHandler handles POST /conversation/participants
func (s *Server) enrollConversationParticipantHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Server.enrollConversationParticipantHandler: processing enrollment request", "method", r.Method, "path", r.URL.Path)

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

	// Store participant background information for conversation context
	if participant.Name != "" || participant.Gender != "" || participant.Ethnicity != "" || participant.Background != "" {
		backgroundInfo := buildParticipantBackgroundInfo(participant)
		slog.Debug("Storing participant background", "participantID", participantID, "backgroundInfo", backgroundInfo, "length", len(backgroundInfo))
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyParticipantBackground, backgroundInfo); err != nil {
			slog.Warn("Failed to store participant background", "error", err, "participantID", participantID)
			// Continue enrollment even if background storage fails
		} else {
			slog.Debug("Successfully stored participant background", "participantID", participantID)
		}
	} else {
		slog.Debug("No participant background information to store", "participantID", participantID)
	}

	// Register response hook for this participant using the actual participant ID
	conversationHook := messaging.CreateConversationHook(participantID, s.msgService)
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

	// For the first message, we simulate the user starting the conversation
	// This allows the AI to respond naturally with a greeting instead of following an instruction
	simulatedUserGreeting := "Hi!"

	response, err := conversationFlow.ProcessResponse(ctx, participantID, simulatedUserGreeting)
	if err != nil {
		slog.Error("generateFirstConversationMessage ProcessResponse failed", "error", err, "participantID", participantID)
		return "", err
	}

	return response, nil
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

	// Send unregister notification to participant
	notificationCtx := context.Background()
	unregisterMsg := "You have been unregistered from the conversation experiment by the organizer. If you have any questions, please contact the organizer for assistance. Thank you for your participation."
	if err := s.msgService.SendMessage(notificationCtx, participant.PhoneNumber, unregisterMsg); err != nil {
		slog.Error("deleteConversationParticipantHandler notification failed", "error", err, "participantID", participantID, "phone", participant.PhoneNumber)
		// Continue with deletion even if notification fails
	} else {
		slog.Info("Unregister notification sent", "participantID", participantID, "phone", participant.PhoneNumber)
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

// buildParticipantBackgroundInfo creates a formatted background string from participant information
func buildParticipantBackgroundInfo(participant models.ConversationParticipant) string {
	var backgroundBuilder strings.Builder

	if participant.Name != "" {
		backgroundBuilder.WriteString(fmt.Sprintf("Name: %s\n", participant.Name))
	}
	if participant.Gender != "" {
		backgroundBuilder.WriteString(fmt.Sprintf("Gender: %s\n", participant.Gender))
	}
	if participant.Ethnicity != "" {
		backgroundBuilder.WriteString(fmt.Sprintf("Ethnicity: %s\n", participant.Ethnicity))
	}
	if participant.Background != "" {
		backgroundBuilder.WriteString(fmt.Sprintf("Background: %s\n", participant.Background))
	}

	return strings.TrimSpace(backgroundBuilder.String())
}
