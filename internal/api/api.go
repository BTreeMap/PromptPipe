// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"errors"
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

// Default configuration constants
const (
	// DefaultServerAddress is the default HTTP server address
	DefaultServerAddress = ":8080"
	// DefaultShutdownTimeout is the default timeout for graceful server shutdown
	DefaultShutdownTimeout = 5 * time.Second
)

// Server holds all dependencies for the API server.
type Server struct {
	msgService  messaging.Service
	sched       *scheduler.Scheduler
	st          store.Store
	defaultCron string
	gaClient    *genai.Client
}

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
// It returns an error if initialization fails.
func Run(waOpts []whatsapp.Option, storeOpts []store.Option, genaiOpts []genai.Option, apiOpts []Option) error {
	slog.Debug("API Run invoked", "whatsappOpts", len(waOpts), "storeOpts", len(storeOpts), "genaiOpts", len(genaiOpts), "apiOpts", len(apiOpts))

	// Create server instance
	server := &Server{}

	// Apply API server options
	var apiCfg Opts
	for _, opt := range apiOpts {
		opt(&apiCfg)
	}

	// Determine server address with priority: CLI options > default
	addr := apiCfg.Addr
	if addr == "" {
		addr = DefaultServerAddress
	}

	// Initialize WhatsApp client and wrap in messaging service
	whClient, err := whatsapp.NewClient(waOpts...)
	if err != nil {
		slog.Error("Failed to create WhatsApp client", "error", err)
		return err
	}
	slog.Debug("WhatsApp client created successfully")
	server.msgService = messaging.NewWhatsAppService(whClient)
	slog.Debug("Messaging service initialized")
	// Start messaging service
	if err := server.msgService.Start(context.Background()); err != nil {
		slog.Error("Failed to start messaging service", "error", err)
		return err
	}
	slog.Debug("Messaging service started")

	// Choose storage backend based on DSN type in options
	if len(storeOpts) > 0 {
		// Apply options to determine DSN type
		var cfg store.Opts
		for _, opt := range storeOpts {
			opt(&cfg)
		}

		// Check if it's a PostgreSQL DSN using the shared detection function
		if cfg.DSN != "" && store.DetectDSNType(cfg.DSN) == "postgres" {
			slog.Debug("Initializing PostgreSQL store", "dsn_set", cfg.DSN != "", "dsn_type", "postgresql")
			ps, err := store.NewPostgresStore(storeOpts...)
			if err != nil {
				slog.Error("Failed to connect to PostgreSQL store", "error", err, "dsn_length", len(cfg.DSN))
				return err
			}
			server.st = ps
			slog.Info("Connected to PostgreSQL store successfully")
		} else {
			// Use SQLite store for file paths
			slog.Debug("Initializing SQLite store", "db_path", cfg.DSN, "dsn_type", "sqlite")
			ss, err := store.NewSQLiteStore(storeOpts...)
			if err != nil {
				slog.Error("Failed to connect to SQLite store", "error", err, "db_path", cfg.DSN)
				return err
			}
			server.st = ss
			slog.Info("Connected to SQLite store successfully", "db_path", cfg.DSN)
		}
	} else {
		slog.Debug("No store options provided, using in-memory store")
		server.st = store.NewInMemoryStore()
		slog.Info("Using in-memory store - data will not persist across restarts")
	}

	// After store initialization, forward receipts and responses regardless of store type
	go func() {
		slog.Debug("Starting receipt forwarding routine")
		for r := range server.msgService.Receipts() {
			if err := server.st.AddReceipt(r); err != nil {
				slog.Error("Error storing receipt", "error", err)
			}
		}
	}()
	slog.Debug("Receipt forwarding routine started")
	go func() {
		slog.Debug("Starting response forwarding routine")
		for resp := range server.msgService.Responses() {
			if err := server.st.AddResponse(resp); err != nil {
				slog.Error("Error storing response", "error", err)
			}
		}
	}()
	slog.Debug("Response forwarding routine started")

	// Initialize scheduler
	server.sched = scheduler.NewScheduler()
	slog.Debug("Scheduler initialized")

	// Configure default schedule
	server.defaultCron = apiCfg.DefaultCron
	slog.Debug("Default cron schedule set", "defaultCron", server.defaultCron)

	// Initialize GenAI client if API key provided via options
	if len(genaiOpts) > 0 {
		slog.Debug("Initializing GenAI client")
		var err error
		server.gaClient, err = genai.NewClient(genaiOpts...)
		if err != nil {
			slog.Error("Failed to create GenAI client", "error", err)
			return err
		}
		// Register GenAI flow generator
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: server.gaClient})
		slog.Debug("GenAI client created and generator registered")
	} else {
		server.gaClient = nil
	}

	// Register HTTP handlers
	slog.Debug("Registering HTTP handlers")
	http.HandleFunc("/send", server.sendHandler)
	http.HandleFunc("/schedule", server.scheduleHandler)
	http.HandleFunc("/receipts", server.receiptsHandler)
	// Endpoints for incoming message responses and statistics
	http.HandleFunc("/response", server.responseHandler)
	http.HandleFunc("/responses", server.responsesHandler)
	http.HandleFunc("/stats", server.statsHandler)
	slog.Debug("HTTP handlers registered")
	// Start HTTP server with graceful shutdown
	srv := &http.Server{Addr: addr, Handler: nil}
	go func() {
		slog.Info("PromptPipe API running", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("API server error", "error", err)
		}
	}()
	slog.Debug("HTTP server started in background")
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutdown signal received, shutting down server")

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(ctxShutdown); err != nil {
		slog.Error("Server Shutdown failed", "error", err)
	}
	slog.Info("API server shutdown complete")

	// Stop scheduler
	server.sched.Stop()
	slog.Debug("Scheduler stopped")

	// Close store to clean up database connections
	if err := server.st.Close(); err != nil {
		slog.Error("Store cleanup failed", "error", err)
	} else {
		slog.Debug("Store cleanup completed")
	}

	// Stop messaging service
	if err := server.msgService.Stop(); err != nil {
		slog.Error("Messaging service stop failed", "error", err)
	} else {
		slog.Debug("Messaging service stopped")
	}

	return nil
}

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

	err = s.msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		slog.Error("Error sending message in sendHandler", "error", err, "to", p.To)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Info("Message sent successfully", "to", p.To)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Respond with empty JSON
	w.Write([]byte(`{"status":"ok"}`))
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
		if p.To == "" || s.gaClient == nil || p.SystemPrompt == "" || p.UserPrompt == "" {
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
		if s.defaultCron == "" {
			slog.Warn("scheduleHandler missing cron schedule and no default set", "prompt", p)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.Cron = s.defaultCron
	}
	// Capture prompt locally for closure
	slog.Debug("scheduleHandler scheduling job", "to", p.To, "cron", p.Cron)
	job := p
	if addErr := s.sched.AddJob(p.Cron, func() {
		slog.Debug("scheduled job triggered", "to", job.To)
		// Generate message body via flow
		msg, genErr := flow.Generate(context.Background(), job)
		if genErr != nil {
			slog.Error("Flow generation error in scheduled job", "error", genErr)
			return
		}
		// Send message
		if sendErr := s.msgService.SendMessage(context.Background(), job.To, msg); sendErr != nil {
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Job scheduled successfully
	slog.Info("Job scheduled successfully", "to", p.To, "cron", p.Cron)
	// Respond with JSON status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"scheduled"}`))
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Debug("receipts fetched", "count", len(receipts))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(receipts); err != nil {
		slog.Error("Error encoding receipts response", "error", err)
	}
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
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slog.Debug("responseHandler parsed response", "from", resp.From)
	resp.Time = time.Now().Unix()
	if err := s.st.AddResponse(resp); err != nil {
		slog.Error("Error adding response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Info("Response recorded", "from", resp.From)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"recorded"}`))
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	slog.Debug("responses fetched", "count", len(responses))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		slog.Error("Error encoding responses response", "error", err)
	}
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
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		slog.Error("Error encoding stats response", "error", err)
	}
}
