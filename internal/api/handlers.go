// Package api provides HTTP handlers for PromptPipe endpoints.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

func (s *Server) sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("Server.sendHandler: processing send request", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("Server.sendHandler: method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Server.sendHandler: failed to decode JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}
	slog.Debug("Server.sendHandler: parsed prompt", "to", p.To, "type", p.Type)

	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}

	// Validate and canonicalize recipient using the messaging service
	canonicalTo, err := s.msgService.ValidateAndCanonicalizeRecipient(p.To)
	if err != nil {
		slog.Warn("Server.sendHandler: recipient validation failed", "error", err, "original_to", p.To)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Update the prompt with the canonicalized recipient
	p.To = canonicalTo

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("Server.sendHandler: validation failed", "error", err, "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Generate message body via pluggable flow
	msg, err := flow.Generate(context.Background(), p)
	if err != nil {
		slog.Error("Server.sendHandler: failed to generate message content", "error", err)
		// Flow generation errors are generally internal server errors, not client errors
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate message content"))
		return
	}

	err = s.msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		slog.Error("Server.sendHandler: failed to send message", "error", err, "to", p.To)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to send message"))
		return
	}

	// Auto-register response handler for prompts that expect responses
	if s.respHandler.AutoRegisterResponseHandler(p) {
		slog.Debug("Server.sendHandler: response handler registered for prompt", "type", p.Type, "to", p.To)
	}

	slog.Info("Server.sendHandler: message sent successfully", "to", p.To)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Message sent successfully", nil))
}

func (s *Server) scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("Server.scheduleHandler: processing schedule request", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("Server.scheduleHandler: method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Server.scheduleHandler: failed to decode JSON", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}

	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}

	// Validate and canonicalize recipient using the messaging service
	canonicalTo, err := s.msgService.ValidateAndCanonicalizeRecipient(p.To)
	if err != nil {
		slog.Warn("Server.scheduleHandler: recipient validation failed", "error", err, "original_to", p.To)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Update the prompt with the canonicalized recipient
	p.To = canonicalTo

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("Server.scheduleHandler: validation failed", "error", err, "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Additional validation for GenAI client availability
	if p.Type == models.PromptTypeGenAI && s.gaClient == nil {
		slog.Warn("Server.scheduleHandler: GenAI client not configured", "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("GenAI client not configured"))
		return
	}
	// Apply default schedule if none provided
	if p.Schedule == nil {
		if s.defaultSchedule == nil {
			slog.Warn("Server.scheduleHandler: missing schedule and no default configured", "prompt", p)
			writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing required field: schedule"))
			return
		}
		p.Schedule = s.defaultSchedule
	}
	// Capture prompt locally for closure
	slog.Debug("Server.scheduleHandler: scheduling job", "to", p.To, "schedule", p.Schedule.ToCronString())
	job := p
	timerID, addErr := s.timer.ScheduleWithSchedule(p.Schedule, func() {
		slog.Debug("Server.scheduleHandler: scheduled job triggered", "to", job.To)
		// Create context with timeout for scheduled job operations
		ctx, cancel := context.WithTimeout(context.Background(), DefaultScheduledJobTimeout)
		defer cancel()

		// Generate message body via flow
		msg, genErr := flow.Generate(ctx, job)
		if genErr != nil {
			slog.Error("Server.scheduleHandler: failed to generate content in scheduled job", "error", genErr)
			return
		}
		// Send message
		if sendErr := s.msgService.SendMessage(ctx, job.To, msg); sendErr != nil {
			slog.Error("Server.scheduleHandler: failed to send scheduled message", "error", sendErr, "to", job.To)
			return
		}

		// Auto-register response handler for scheduled prompts that expect responses
		if s.respHandler.AutoRegisterResponseHandler(job) {
			slog.Debug("Server.scheduleHandler: response handler registered for scheduled prompt", "type", job.Type, "to", job.To)
		}

		// Add receipt
		recErr := s.st.AddReceipt(models.Receipt{To: job.To, Status: models.MessageStatusSent, Time: time.Now().Unix()})
		if recErr != nil {
			slog.Error("Server.scheduleHandler: failed to add scheduled receipt", "error", recErr)
		}
	})
	if addErr != nil {
		slog.Error("Server.scheduleHandler: failed to schedule job", "error", addErr)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to schedule job"))
		return
	}
	// Job scheduled successfully
	slog.Info("Server.scheduleHandler: job scheduled successfully", "to", p.To, "schedule", p.Schedule.ToCronString(), "timerID", timerID)
	writeJSONResponse(w, http.StatusCreated, models.SuccessWithMessage("Scheduled successfully", timerID))
}

func (s *Server) receiptsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Server.receiptsHandler: processing receipts request", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("Server.receiptsHandler: method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	receipts, err := s.st.GetReceipts()
	if err != nil {
		slog.Error("Error fetching receipts", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to fetch receipts"))
		return
	}
	slog.Debug("receipts fetched", "count", len(receipts))
	writeJSONResponse(w, http.StatusOK, models.Success(receipts))
}

// responseHandler handles incoming participant responses (POST /response).
func (s *Server) responseHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("responseHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("responseHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var resp models.Response
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		slog.Warn("Invalid JSON in responseHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}
	slog.Debug("responseHandler parsed response", "from", resp.From)
	resp.Time = time.Now().Unix()
	if err := s.st.AddResponse(resp); err != nil {
		slog.Error("Error adding response", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to store response"))
		return
	}
	slog.Info("Response recorded", "from", resp.From)
	writeJSONResponse(w, http.StatusCreated, models.SuccessWithMessage("Response recorded successfully", nil))
}

// responsesHandler returns all collected responses (GET /responses).
func (s *Server) responsesHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("responsesHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("responsesHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := s.st.GetResponses()
	if err != nil {
		slog.Error("Error fetching responses", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to fetch responses"))
		return
	}
	slog.Debug("responses fetched", "count", len(responses))
	writeJSONResponse(w, http.StatusOK, models.Success(responses))
}

// statsHandler returns statistics about collected responses (GET /stats).
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("statsHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("statsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := s.st.GetResponses()
	if err != nil {
		slog.Error("Error fetching responses in statsHandler", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to fetch responses"))
		return
	}
	total := len(responses)
	perSender := make(map[string]int)
	var sumLen int
	for _, resp := range responses {
		perSender[resp.From]++
		sumLen += len(resp.Body)
	}
	avgLen := 0.0
	if total > 0 {
		avgLen = float64(sumLen) / float64(total)
	}
	stats := map[string]interface{}{
		"total_responses":      total,
		"responses_per_sender": perSender,
		"avg_response_length":  avgLen,
	}
	slog.Debug("stats computed", "total_responses", total, "avg_response_length", avgLen)
	writeJSONResponse(w, http.StatusOK, models.Success(stats))
}

// timersHandler handles global timer operations (GET /timers)
func (s *Server) timersHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("timersHandler invoked", "method", r.Method, "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/timers")

	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	// Split path into segments
	segments := strings.Split(path, "/")

	if len(segments) == 0 || segments[0] == "" {
		// /intervention/timers
		switch r.Method {
		case http.MethodGet:
			s.listTimersHandler(w, r)
		default:
			w.Header().Set("Allow", "GET")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
		return
	}

	// Extract timer ID for specific timer operations
	timerID := segments[0]

	if len(segments) == 1 {
		// /intervention/timers/{id}
		switch r.Method {
		case http.MethodGet:
			s.getTimerHandler(w, r, timerID)
		case http.MethodDelete:
			s.cancelTimerHandler(w, r, timerID)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
		return
	}

	writeJSONResponse(w, http.StatusNotFound, models.Error("Unknown timer endpoint"))
}

// listTimersHandler handles GET /intervention/timers
func (s *Server) listTimersHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("listTimersHandler invoked", "method", r.Method, "path", r.URL.Path)

	timers := s.timer.ListActive()

	slog.Info("listTimersHandler returning timers", "count", len(timers))
	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"timers": timers,
		"count":  len(timers),
	})
}

// getTimerHandler handles GET /intervention/timers/{id}
func (s *Server) getTimerHandler(w http.ResponseWriter, r *http.Request, timerID string) {
	slog.Debug("getTimerHandler invoked", "method", r.Method, "path", r.URL.Path, "timerID", timerID)

	timerInfo, err := s.timer.GetTimer(timerID)
	if err != nil {
		slog.Warn("getTimerHandler timer not found", "timerID", timerID, "error", err)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Timer not found: "+err.Error()))
		return
	}

	slog.Info("getTimerHandler returning timer info", "timerID", timerID)
	writeJSONResponse(w, http.StatusOK, timerInfo)
}

// cancelTimerHandler handles DELETE /intervention/timers/{id}
func (s *Server) cancelTimerHandler(w http.ResponseWriter, r *http.Request, timerID string) {
	slog.Debug("cancelTimerHandler invoked", "method", r.Method, "path", r.URL.Path, "timerID", timerID)

	// Check if timer exists first
	_, err := s.timer.GetTimer(timerID)
	if err != nil {
		slog.Warn("cancelTimerHandler timer not found", "timerID", timerID, "error", err)
		writeJSONResponse(w, http.StatusNotFound, models.Error("Timer not found: "+err.Error()))
		return
	}

	// Cancel the timer
	err = s.timer.Cancel(timerID)
	if err != nil {
		slog.Error("cancelTimerHandler failed to cancel timer", "timerID", timerID, "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to cancel timer"))
		return
	}

	response := map[string]interface{}{
		"message":  "Timer cancelled successfully",
		"timerID":  timerID,
		"canceled": true,
	}

	slog.Info("cancelTimerHandler timer cancelled", "timerID", timerID)
	writeJSONResponse(w, http.StatusOK, response)
}

// healthHandler provides a health check endpoint for monitoring and load balancing
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthData := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Get active participant count as a health indicator
	if s.respHandler != nil {
		if count, err := s.respHandler.GetActiveParticipantCount(ctx); err != nil {
			slog.Warn("Health check: failed to get active participant count", "error", err)
			healthData["status"] = "degraded"
			healthData["error"] = "Failed to fetch participant metrics"
		} else {
			healthData["active_participants"] = count
		}
	}

	// Set appropriate status code based on health
	statusCode := http.StatusOK
	if healthData["status"] == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSONResponse(w, statusCode, healthData)
}
