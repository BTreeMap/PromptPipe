// Package api provides HTTP handlers for PromptPipe endpoints.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

func (s *Server) sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("sendHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("sendHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Failed to decode JSON in sendHandler", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("Invalid JSON format"))
		return
	}
	slog.Debug("sendHandler parsed prompt", "to", p.To, "type", p.Type)

	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}

	// Validate and canonicalize recipient using the messaging service
	canonicalTo, err := s.msgService.ValidateAndCanonicalizeRecipient(p.To)
	if err != nil {
		slog.Warn("sendHandler recipient validation failed", "error", err, "original_to", p.To)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Update the prompt with the canonicalized recipient
	p.To = canonicalTo

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("sendHandler validation failed", "error", err, "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Generate message body via pluggable flow
	msg, err := flow.Generate(context.Background(), p)
	if err != nil {
		slog.Error("Flow generation error in sendHandler", "error", err)
		// Flow generation errors are generally internal server errors, not client errors
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to generate message content"))
		return
	}

	err = s.msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		slog.Error("Error sending message in sendHandler", "error", err, "to", p.To)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to send message"))
		return
	}

	// Auto-register response handler for prompts that expect responses
	// Set a reasonable timeout for response handlers (24 hours for most prompts)
	defaultTimeout := 24 * time.Hour
	if s.respHandler.AutoRegisterResponseHandler(p, defaultTimeout) {
		// Set auto-cleanup after 48 hours to prevent memory leaks
		s.respHandler.SetAutoCleanupTimeout(p.To, 48*time.Hour)
		slog.Debug("Response handler registered for prompt", "type", p.Type, "to", p.To)
	}

	slog.Info("Message sent successfully", "to", p.To)
	writeJSONResponse(w, http.StatusOK, models.SuccessWithMessage("Message sent successfully", nil))
}

func (s *Server) scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	slog.Debug("scheduleHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		slog.Warn("scheduleHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Failed to decode JSON in scheduleHandler", "error", err)
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
		slog.Warn("scheduleHandler recipient validation failed", "error", err, "original_to", p.To)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}
	// Update the prompt with the canonicalized recipient
	p.To = canonicalTo

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("scheduleHandler validation failed", "error", err, "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error(err.Error()))
		return
	}

	// Additional validation for GenAI client availability
	if p.Type == models.PromptTypeGenAI && s.gaClient == nil {
		slog.Warn("scheduleHandler genai client not configured", "prompt", p)
		writeJSONResponse(w, http.StatusBadRequest, models.Error("GenAI client not configured"))
		return
	}
	// Apply default schedule if none provided
	if p.Cron == "" {
		if s.defaultCron == "" {
			slog.Warn("scheduleHandler missing cron schedule and no default set", "prompt", p)
			writeJSONResponse(w, http.StatusBadRequest, models.Error("Missing required field: cron schedule"))
			return
		}
		p.Cron = s.defaultCron
	}
	// Capture prompt locally for closure
	slog.Debug("scheduleHandler scheduling job", "to", p.To, "cron", p.Cron)
	job := p
	timerID, addErr := s.timer.ScheduleCron(p.Cron, func() {
		slog.Debug("scheduled job triggered", "to", job.To)
		// Create context with timeout for scheduled job operations
		ctx, cancel := context.WithTimeout(context.Background(), DefaultScheduledJobTimeout)
		defer cancel()

		// Generate message body via flow
		msg, genErr := flow.Generate(ctx, job)
		if genErr != nil {
			slog.Error("Flow generation error in scheduled job", "error", genErr)
			return
		}
		// Send message
		if sendErr := s.msgService.SendMessage(ctx, job.To, msg); sendErr != nil {
			slog.Error("Scheduled job send error", "error", sendErr, "to", job.To)
			return
		}

		// Auto-register response handler for scheduled prompts that expect responses
		defaultTimeout := 24 * time.Hour
		if s.respHandler.AutoRegisterResponseHandler(job, defaultTimeout) {
			// Set auto-cleanup after 48 hours to prevent memory leaks
			s.respHandler.SetAutoCleanupTimeout(job.To, 48*time.Hour)
			slog.Debug("Response handler registered for scheduled prompt", "type", job.Type, "to", job.To)
		}

		// Add receipt
		recErr := s.st.AddReceipt(models.Receipt{To: job.To, Status: models.MessageStatusSent, Time: time.Now().Unix()})
		if recErr != nil {
			slog.Error("Error adding scheduled receipt", "error", recErr)
		}
	})
	if addErr != nil {
		slog.Error("Error scheduling job", "error", addErr)
		writeJSONResponse(w, http.StatusInternalServerError, models.Error("Failed to schedule job"))
		return
	}
	// Job scheduled successfully
	slog.Info("Job scheduled successfully", "to", p.To, "cron", p.Cron, "timerID", timerID)
	writeJSONResponse(w, http.StatusCreated, models.SuccessWithMessage("Scheduled successfully", timerID))
}

func (s *Server) receiptsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("receiptsHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		slog.Warn("receiptsHandler method not allowed", "method", r.Method)
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

	switch r.Method {
	case http.MethodGet:
		s.listAllTimersHandler(w, r)
	default:
		w.Header().Set("Allow", "GET")
		writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
	}
}

// listAllTimersHandler returns all active timers in the system
func (s *Server) listAllTimersHandler(w http.ResponseWriter, r *http.Request) {
	timers := s.timer.ListActive()

	slog.Debug("Listed all timers", "count", len(timers))
	writeJSONResponse(w, http.StatusOK, models.Success(map[string]interface{}{
		"timers": timers,
		"count":  len(timers),
	}))
}
