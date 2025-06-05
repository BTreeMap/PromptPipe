// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

var (
	msgService  messaging.Service
	sched       *scheduler.Scheduler
	st          store.Store // Use the interface for flexibility
	defaultCron string      // default cron schedule from opts
	gaClient    *genai.Client
)

// Opts holds configuration options for the API server, such as HTTP address and default cron schedule.
type Opts struct {
	Addr        string // overrides API_ADDR
	DefaultCron string // overrides DEFAULT_SCHEDULE
}

// Option defines a configuration option for the API server.
type Option func(*Opts)

// WithAddr overrides the server address for the API.
func WithAddr(addr string) Option {
	return func(o *Opts) {
		o.Addr = addr
	}
}

// WithDefaultCron overrides the default cron schedule for prompts.
func WithDefaultCron(cron string) Option {
	return func(o *Opts) {
		o.DefaultCron = cron
	}
}

// Run starts the API server and initializes dependencies, applying module options.
func Run(waOpts []whatsapp.Option, storeOpts []store.Option, genaiOpts []genai.Option, apiOpts []Option) {
	slog.Debug("API Run invoked", "whatsappOpts", len(waOpts), "storeOpts", len(storeOpts), "genaiOpts", len(genaiOpts), "apiOpts", len(apiOpts))
	var err error

	// Apply API server options
	var apiCfg Opts
	for _, opt := range apiOpts {
		opt(&apiCfg)
	}

	// Determine server address with priority: CLI options > default
	addr := apiCfg.Addr
	if addr == "" {
		addr = ":8080"
	}

	// Initialize WhatsApp client and wrap in messaging service
	whClient, err := whatsapp.NewClient(waOpts...)
	if err != nil {
		slog.Error("Failed to create WhatsApp client", "error", err)
		os.Exit(1)
	}
	slog.Debug("WhatsApp client created successfully")
	msgService = messaging.NewWhatsAppService(whClient)
	slog.Debug("Messaging service initialized")
	// Start messaging service
	if err := msgService.Start(context.Background()); err != nil {
		slog.Error("Failed to start messaging service", "error", err)
		os.Exit(1)
	}
	slog.Debug("Messaging service started")
	// Forward receipts and responses to store
	go func() {
		slog.Debug("Starting receipt forwarding routine")
		for r := range msgService.Receipts() {
			if err := st.AddReceipt(r); err != nil {
				slog.Error("Error storing receipt", "error", err)
			}
		}
	}()
	slog.Debug("Receipt forwarding routine started")
	go func() {
		slog.Debug("Starting response forwarding routine")
		for resp := range msgService.Responses() {
			if err := st.AddResponse(resp); err != nil {
				slog.Error("Error storing response", "error", err)
			}
		}
	}()
	slog.Debug("Response forwarding routine started")

	// Initialize scheduler
	sched = scheduler.NewScheduler()
	slog.Debug("Scheduler initialized")

	// Configure default schedule
	defaultCron = apiCfg.DefaultCron
	slog.Debug("Default cron schedule set", "defaultCron", defaultCron)

	// Choose storage backend: Postgres if DSN provided via options, else in-memory
	if len(storeOpts) > 0 {
		ps, err := store.NewPostgresStore(storeOpts...)
		if err != nil {
			slog.Error("Failed to connect to Postgres store", "error", err)
			os.Exit(1)
		}
		st = ps
		slog.Debug("Connected to Postgres store")
	} else {
		st = store.NewInMemoryStore()
		slog.Debug("Using in-memory store")
	}

	// Initialize GenAI client if API key provided via options
	if len(genaiOpts) > 0 {
		slog.Debug("Initializing GenAI client")
		gaClient, err = genai.NewClient(genaiOpts...)
		if err != nil {
			slog.Error("Failed to create GenAI client", "error", err)
			os.Exit(1)
		}
		// Register GenAI flow generator
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: gaClient})
		slog.Debug("GenAI client created and generator registered")
	} else {
		gaClient = nil
	}

	// Register flow generators
	// Static and branch are registered init(); register GenAI if available
	if gaClient != nil {
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: gaClient})
	}

	// Register HTTP handlers
	slog.Debug("Registering HTTP handlers")
	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/receipts", receiptsHandler)
	// Endpoints for incoming message responses and statistics
	http.HandleFunc("/response", responseHandler)
	http.HandleFunc("/responses", responsesHandler)
	http.HandleFunc("/stats", statsHandler)
	slog.Debug("HTTP handlers registered")
	// Start HTTP server with graceful shutdown
	srv := &http.Server{Addr: addr, Handler: nil}
	go func() {
		slog.Info("PromptPipe API running", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
			os.Exit(1)
		}
	}()
	slog.Debug("HTTP server started in background")
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutdown signal received, shutting down server")
	if err := srv.Shutdown(context.Background()); err != nil {
		slog.Error("Server Shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("API server shutdown complete")
	sched.Stop()
	slog.Debug("Scheduler stopped")
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("sendHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		slog.Warn("sendHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Failed to decode JSON in sendHandler", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slog.Debug("sendHandler parsed prompt", "to", p.To, "type", p.Type)
	// Validate required field: to
	if p.To == "" {
		slog.Warn("sendHandler missing recipient", "prompt", p)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Default to static type if not specified
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}
	// Generate message body via pluggable flow
	msg, err := flow.Generate(context.Background(), p)
	if err != nil {
		slog.Error("Flow generation error in sendHandler", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		slog.Error("Error sending message in sendHandler", "error", err, "to", p.To)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Info("Message sent successfully", "to", p.To)
	w.WriteHeader(http.StatusOK)
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("scheduleHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		slog.Warn("scheduleHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		slog.Warn("Failed to decode JSON in scheduleHandler", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Validate required fields: to
	if p.To == "" {
		slog.Warn("scheduleHandler missing recipient", "prompt", p)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if p.Type == "" {
		p.Type = models.PromptTypeStatic
	}
	// Additional validation based on type
	switch p.Type {
	case models.PromptTypeStatic:
		if p.To == "" || p.Body == "" {
			slog.Warn("scheduleHandler static prompt missing body", "prompt", p)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeGenAI:
		if p.To == "" || gaClient == nil || p.SystemPrompt == "" || p.UserPrompt == "" {
			slog.Warn("scheduleHandler genai prompt invalid or no genai client", "prompt", p)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeBranch:
		if p.To == "" || len(p.BranchOptions) == 0 {
			slog.Warn("scheduleHandler branch prompt missing options", "prompt", p)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		slog.Warn("scheduleHandler unsupported prompt type", "type", p.Type)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Apply default schedule if none provided
	if p.Cron == "" {
		if defaultCron == "" {
			slog.Warn("scheduleHandler missing cron schedule and no default set", "prompt", p)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.Cron = defaultCron
	}
	// Capture prompt locally for closure
	slog.Debug("scheduleHandler scheduling job", "to", p.To, "cron", p.Cron)
	job := p
	if err := sched.AddJob(p.Cron, func() {
		slog.Debug("scheduled job triggered", "to", job.To)
		// Generate message body via flow
		msg, err := flow.Generate(context.Background(), job)
		if err != nil {
			slog.Error("Flow generation error in scheduled job", "error", err)
			return
		}
		if err := msgService.SendMessage(context.Background(), job.To, msg); err != nil {
			slog.Error("Scheduled job send error", "error", err, "to", job.To)
			return
		}
		if err := st.AddReceipt(models.Receipt{To: job.To, Status: "sent", Time: time.Now().Unix()}); err != nil {
			slog.Error("Error adding scheduled receipt", "error", err)
		}
	}); err != nil {
		slog.Error("Error scheduling job", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Info("Job scheduled successfully", "to", p.To, "cron", p.Cron)
	w.WriteHeader(http.StatusCreated)
}

func receiptsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("receiptsHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		slog.Warn("receiptsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	receipts, err := st.GetReceipts()
	if err != nil {
		slog.Error("Error fetching receipts", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Debug("receipts fetched", "count", len(receipts))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(receipts); err != nil {
		slog.Error("Error encoding receipts response", "error", err)
	}
}

// responseHandler handles incoming participant responses (POST /response).
func responseHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("responseHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		slog.Warn("responseHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var resp models.Response
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		slog.Warn("Invalid JSON in responseHandler", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slog.Debug("responseHandler parsed response", "from", resp.From)
	resp.Time = time.Now().Unix()
	if err := st.AddResponse(resp); err != nil {
		slog.Error("Error adding response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Info("Response recorded", "from", resp.From)
	w.WriteHeader(http.StatusCreated)
}

// responsesHandler returns all collected responses (GET /responses).
func responsesHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("responsesHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		slog.Warn("responsesHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := st.GetResponses()
	if err != nil {
		slog.Error("Error fetching responses", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Debug("responses fetched", "count", len(responses))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		slog.Error("Error encoding responses response", "error", err)
	}
}

// statsHandler returns statistics about collected responses (GET /stats).
func statsHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("statsHandler invoked", "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodGet {
		slog.Warn("statsHandler method not allowed", "method", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := st.GetResponses()
	if err != nil {
		slog.Error("Error fetching responses in statsHandler", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
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
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		slog.Error("Error encoding stats response", "error", err)
	}
}
