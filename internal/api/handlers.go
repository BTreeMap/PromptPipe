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
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	slog.Debug("sendHandler parsed prompt", "to", p.To, "type", p.Type)

	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("sendHandler validation failed", "error", err, "prompt", p)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Generate message body via pluggable flow
	msg, err := flow.Generate(context.Background(), p)
	if err != nil {
		slog.Error("Flow generation error in sendHandler", "error", err)
		// Flow generation errors are generally internal server errors, not client errors
		http.Error(w, "Failed to generate message content", http.StatusInternalServerError)
		return
	}

	err = s.msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		slog.Error("Error sending message in sendHandler", "error", err, "to", p.To)
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}
	slog.Info("Message sent successfully", "to", p.To)
	writeJSONResponse(w, http.StatusOK, models.NewOKResponse())
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
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}

	// Validate prompt using the models validation
	if err := p.Validate(); err != nil {
		slog.Warn("scheduleHandler validation failed", "error", err, "prompt", p)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Additional validation for GenAI client availability
	if p.Type == models.PromptTypeGenAI && s.gaClient == nil {
		slog.Warn("scheduleHandler genai client not configured", "prompt", p)
		http.Error(w, "Invalid GenAI prompt or GenAI client not configured", http.StatusBadRequest)
		return
	}
	// Apply default schedule if none provided
	if p.Cron == "" {
		if s.defaultCron == "" {
			slog.Warn("scheduleHandler missing cron schedule and no default set", "prompt", p)
			http.Error(w, "Missing required field: cron schedule", http.StatusBadRequest)
			return
		}
		p.Cron = s.defaultCron
	}
	// Capture prompt locally for closure
	slog.Debug("scheduleHandler scheduling job", "to", p.To, "cron", p.Cron)
	job := p
	if addErr := s.sched.AddJob(p.Cron, func() {
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
		// Add receipt
		recErr := s.st.AddReceipt(models.Receipt{To: job.To, Status: models.StatusTypeSent, Time: time.Now().Unix()})
		if recErr != nil {
			slog.Error("Error adding scheduled receipt", "error", recErr)
		}
	}); addErr != nil {
		slog.Error("Error scheduling job", "error", addErr)
		http.Error(w, "Failed to schedule job", http.StatusInternalServerError)
		return
	}
	// Job scheduled successfully
	slog.Info("Job scheduled successfully", "to", p.To, "cron", p.Cron)
	writeJSONResponse(w, http.StatusCreated, models.NewScheduledResponse())
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
		http.Error(w, "Failed to fetch receipts", http.StatusInternalServerError)
		return
	}
	slog.Debug("receipts fetched", "count", len(receipts))
	writeJSONResponse(w, http.StatusOK, receipts)
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
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	slog.Debug("responseHandler parsed response", "from", resp.From)
	resp.Time = time.Now().Unix()
	if err := s.st.AddResponse(resp); err != nil {
		slog.Error("Error adding response", "error", err)
		http.Error(w, "Failed to store response", http.StatusInternalServerError)
		return
	}
	slog.Info("Response recorded", "from", resp.From)
	writeJSONResponse(w, http.StatusCreated, models.NewRecordedResponse())
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
		http.Error(w, "Failed to fetch responses", http.StatusInternalServerError)
		return
	}
	slog.Debug("responses fetched", "count", len(responses))
	writeJSONResponse(w, http.StatusOK, responses)
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
		http.Error(w, "Failed to fetch responses", http.StatusInternalServerError)
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
	writeJSONResponse(w, http.StatusOK, stats)
}
