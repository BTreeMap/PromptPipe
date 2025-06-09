// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// writeJSONResponse writes a JSON response to the http.ResponseWriter with the given status code.
func writeJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
		// Fallback to a simple error message using proper structure
		w.WriteHeader(http.StatusInternalServerError)
		fallbackResponse := models.NewAPIResponse(models.APIStatusError)
		if fallbackErr := json.NewEncoder(w).Encode(fallbackResponse); fallbackErr != nil {
			// Last resort: write minimal JSON
			w.Write([]byte(`{"status":"error"}`))
		}
	}
}

// Default configuration constants
const (
	// DefaultServerAddress is the default HTTP server address
	DefaultServerAddress = ":8080"
	// DefaultShutdownTimeout is the default timeout for graceful server shutdown
	DefaultShutdownTimeout = 5 * time.Second
	// DefaultScheduledJobTimeout is the default timeout for scheduled job operations
	DefaultScheduledJobTimeout = 30 * time.Second
)

// Server holds all dependencies for the API server.
type Server struct {
	msgService  messaging.Service
	sched       *scheduler.Scheduler
	st          store.Store
	defaultCron string
	gaClient    *genai.Client
}

// NewServer creates a new API server instance with the provided dependencies.
func NewServer(msgService messaging.Service, sched *scheduler.Scheduler, st store.Store, defaultCron string, gaClient *genai.Client) *Server {
	return &Server{
		msgService:  msgService,
		sched:       sched,
		st:          st,
		defaultCron: defaultCron,
		gaClient:    gaClient,
	}
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

	// Create and configure server instance
	server, addr, err := createAndConfigureServer(waOpts, storeOpts, genaiOpts, apiOpts)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start the HTTP server
	srv := startHTTPServer(addr, server)

	// Wait for shutdown signal and perform graceful shutdown
	return waitForShutdownAndCleanup(server, srv)
}

// createAndConfigureServer creates and configures a Server instance with all dependencies
func createAndConfigureServer(waOpts []whatsapp.Option, storeOpts []store.Option, genaiOpts []genai.Option, apiOpts []Option) (*Server, string, error) {
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
		return nil, "", fmt.Errorf("failed to create WhatsApp client: %w", err)
	}
	slog.Debug("WhatsApp client created successfully")
	server.msgService = messaging.NewWhatsAppService(whClient)
	slog.Debug("Messaging service initialized")

	// Start messaging service
	if err := server.msgService.Start(context.Background()); err != nil {
		slog.Error("Failed to start messaging service", "error", err)
		return nil, "", fmt.Errorf("failed to start messaging service: %w", err)
	}
	slog.Debug("Messaging service started")

	// Initialize store
	if err := server.initializeStore(storeOpts); err != nil {
		return nil, "", fmt.Errorf("failed to initialize store: %w", err)
	}

	// Initialize scheduler
	server.sched = scheduler.NewScheduler()
	slog.Debug("Scheduler initialized")

	// Configure default schedule
	server.defaultCron = apiCfg.DefaultCron
	slog.Debug("Default cron schedule set", "defaultCron", server.defaultCron)

	// Initialize GenAI client if options provided
	if err := server.initializeGenAI(genaiOpts); err != nil {
		return nil, "", fmt.Errorf("failed to initialize GenAI: %w", err)
	}

	return server, addr, nil
}

// initializeStore sets up the store backend based on provided options
func (s *Server) initializeStore(storeOpts []store.Option) error {
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
				return fmt.Errorf("failed to connect to PostgreSQL store: %w", err)
			}
			s.st = ps
			slog.Info("Connected to PostgreSQL store successfully")
		} else {
			// Use SQLite store for file paths
			slog.Debug("Initializing SQLite store", "db_path", cfg.DSN, "dsn_type", "sqlite")
			ss, err := store.NewSQLiteStore(storeOpts...)
			if err != nil {
				slog.Error("Failed to connect to SQLite store", "error", err, "db_path", cfg.DSN)
				return fmt.Errorf("failed to connect to SQLite store: %w", err)
			}
			s.st = ss
			slog.Info("Connected to SQLite store successfully", "db_path", cfg.DSN)
		}
	} else {
		slog.Debug("No store options provided, using in-memory store")
		s.st = store.NewInMemoryStore()
		slog.Info("Using in-memory store - data will not persist across restarts")
	}

	// Start forwarding routines for receipts and responses
	s.startForwardingRoutines()

	return nil
}

// startForwardingRoutines starts background goroutines to forward receipts and responses to store
func (s *Server) startForwardingRoutines() {
	go func() {
		slog.Debug("Starting receipt forwarding routine")
		for r := range s.msgService.Receipts() {
			if err := s.st.AddReceipt(r); err != nil {
				slog.Error("Error storing receipt", "error", err)
			}
		}
	}()
	slog.Debug("Receipt forwarding routine started")

	go func() {
		slog.Debug("Starting response forwarding routine")
		for resp := range s.msgService.Responses() {
			if err := s.st.AddResponse(resp); err != nil {
				slog.Error("Error storing response", "error", err)
			}
		}
	}()
	slog.Debug("Response forwarding routine started")
}

// initializeGenAI sets up the GenAI client if options are provided
func (s *Server) initializeGenAI(genaiOpts []genai.Option) error {
	if len(genaiOpts) > 0 {
		slog.Debug("Initializing GenAI client")
		var err error
		s.gaClient, err = genai.NewClient(genaiOpts...)
		if err != nil {
			slog.Error("Failed to create GenAI client", "error", err)
			return fmt.Errorf("failed to create GenAI client: %w", err)
		}
		// Register GenAI flow generator
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: s.gaClient})
		slog.Debug("GenAI client created and generator registered")
	} else {
		s.gaClient = nil
	}
	return nil
}

// startHTTPServer registers handlers and starts the HTTP server
func startHTTPServer(addr string, server *Server) *http.Server {
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

	return srv
}

// waitForShutdownAndCleanup waits for shutdown signal and handles cleanup
func waitForShutdownAndCleanup(server *Server, srv *http.Server) error {
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutdown signal received, shutting down server")

	// Perform graceful shutdown with proper error handling
	return server.gracefulShutdown(srv)
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
		http.Error(w, "Failed to generate message content", http.StatusBadRequest)
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

// gracefulShutdown handles the proper shutdown sequence for all services
func (s *Server) gracefulShutdown(srv *http.Server) error {
	var shutdownErrors []error

	// Create a timeout context for the shutdown process
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancelShutdown()

	// Shutdown HTTP server
	slog.Debug("Shutting down HTTP server")
	if err := srv.Shutdown(ctxShutdown); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
		shutdownErrors = append(shutdownErrors, fmt.Errorf("HTTP server shutdown: %w", err))
	} else {
		slog.Info("HTTP server shutdown complete")
	}

	// Stop scheduler
	slog.Debug("Stopping scheduler")
	s.sched.Stop()
	slog.Debug("Scheduler stopped")

	// Close store to clean up database connections
	slog.Debug("Closing store")
	if err := s.st.Close(); err != nil {
		slog.Error("Store cleanup failed", "error", err)
		shutdownErrors = append(shutdownErrors, fmt.Errorf("store cleanup: %w", err))
	} else {
		slog.Debug("Store cleanup completed")
	}

	// Stop messaging service
	slog.Debug("Stopping messaging service")
	if err := s.msgService.Stop(); err != nil {
		slog.Error("Messaging service stop failed", "error", err)
		shutdownErrors = append(shutdownErrors, fmt.Errorf("messaging service stop: %w", err))
	} else {
		slog.Debug("Messaging service stopped")
	}

	// Return any accumulated errors
	if len(shutdownErrors) > 0 {
		slog.Error("Shutdown completed with errors", "error_count", len(shutdownErrors))
		// Return the first error, but log all of them
		for i, err := range shutdownErrors {
			slog.Error("Shutdown error", "index", i, "error", err)
		}
		return shutdownErrors[0]
	}

	slog.Info("Graceful shutdown completed successfully")
	return nil
}
