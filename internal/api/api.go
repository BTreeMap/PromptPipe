// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, timer, and store modules.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// ContextKeyParticipantID is the context key for participant ID
	ContextKeyParticipantID ContextKey = "participantID"
)

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
	respHandler *messaging.ResponseHandler
	st          store.Store
	timer       models.Timer
	defaultCron string
	gaClient    *genai.Client
}

// NewServer creates a new API server instance with the provided dependencies.
func NewServer(msgService messaging.Service, st store.Store, timer models.Timer, defaultCron string, gaClient *genai.Client) *Server {
	// Create response handler
	respHandler := messaging.NewResponseHandler(msgService)

	return &Server{
		msgService:  msgService,
		respHandler: respHandler,
		st:          st,
		timer:       timer,
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

	// Create response handler
	server.respHandler = messaging.NewResponseHandler(server.msgService)
	slog.Debug("Response handler initialized")

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

	// Initialize timer
	server.timer = flow.NewSimpleTimer()
	slog.Debug("Timer initialized")

	// Configure default schedule
	server.defaultCron = apiCfg.DefaultCron
	slog.Debug("Default cron schedule set", "defaultCron", server.defaultCron)

	// Initialize GenAI client if options provided
	if err := server.initializeGenAI(genaiOpts); err != nil {
		return nil, "", fmt.Errorf("failed to initialize GenAI: %w", err)
	}

	// Initialize conversation flow
	if err := server.initializeConversationFlow(); err != nil {
		return nil, "", fmt.Errorf("failed to initialize conversation flow: %w", err)
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

// startForwardingRoutines starts background goroutines to forward receipts and handle responses
func (s *Server) startForwardingRoutines() {
	go func() {
		slog.Debug("Starting receipt forwarding routine")
		defer slog.Debug("Receipt forwarding routine stopped")
		for r := range s.msgService.Receipts() {
			if err := s.st.AddReceipt(r); err != nil {
				slog.Error("Error storing receipt", "error", err, "to", r.To, "status", r.Status)
			} else {
				slog.Debug("Receipt stored successfully", "to", r.To, "status", r.Status)
			}
		}
	}()
	slog.Debug("Receipt forwarding routine started")

	go func() {
		slog.Debug("Starting response processing routine")
		defer slog.Debug("Response processing routine stopped")
		for resp := range s.msgService.Responses() {
			// Store the response first
			if err := s.st.AddResponse(resp); err != nil {
				slog.Error("Error storing response", "error", err, "from", resp.From)
			} else {
				slog.Debug("Response stored successfully", "from", resp.From)
			}

			// Process the response through the response handler
			ctx := context.Background()
			if err := s.respHandler.ProcessResponse(ctx, resp); err != nil {
				slog.Error("Error processing response through handler", "error", err, "from", resp.From)
			}
		}
	}()
	slog.Debug("Response processing routine started")
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

// initializeConversationFlow sets up the conversation flow with system prompt loading
func (s *Server) initializeConversationFlow() error {
	// Get system prompt file path
	systemPromptFile := flow.GetSystemPromptPath()

	// Create default system prompt file if it doesn't exist
	if err := flow.CreateDefaultSystemPromptFile(systemPromptFile); err != nil {
		slog.Warn("Failed to create default system prompt file", "error", err, "path", systemPromptFile)
	}

	// Create conversation flow with dependencies
	stateManager := flow.NewStoreBasedStateManager(s.st)
	conversationFlow := flow.NewConversationFlow(stateManager, s.gaClient, systemPromptFile)

	// Load system prompt
	if err := conversationFlow.LoadSystemPrompt(); err != nil {
		slog.Warn("Failed to load system prompt, using default", "error", err)
	}

	// Register conversation flow generator
	flow.Register(models.PromptTypeConversation, conversationFlow)
	slog.Debug("Conversation flow initialized and registered", "systemPromptFile", systemPromptFile)

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

	// Timer management endpoints (global scope)
	http.HandleFunc("/timers", server.timersHandler)

	// Intervention management endpoints with proper routing
	http.HandleFunc("/intervention/", server.interventionRouter)

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

	// Stop timer
	slog.Debug("Stopping timer")
	s.timer.Stop()
	slog.Debug("Timer stopped")

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

// interventionRouter handles all intervention-related endpoints with proper RESTful routing
func (s *Server) interventionRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/intervention")

	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	slog.Debug("Intervention router", "method", r.Method, "path", path, "fullPath", r.URL.Path)

	// Split path into segments
	segments := strings.Split(path, "/")
	if len(segments) == 0 || segments[0] == "" {
		http.Error(w, "Invalid intervention endpoint", http.StatusNotFound)
		return
	}

	switch segments[0] {
	case "participants":
		s.handleParticipantRoutes(w, r, segments[1:])
	case "weekly-summary":
		s.triggerWeeklySummaryHandler(w, r)
	case "stats":
		s.interventionStatsHandler(w, r)
	default:
		http.Error(w, "Unknown intervention endpoint", http.StatusNotFound)
	}
}

// handleParticipantRoutes handles all participant-related routes
func (s *Server) handleParticipantRoutes(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 || segments[0] == "" {
		// /intervention/participants
		switch r.Method {
		case http.MethodGet:
			s.listParticipantsHandler(w, r)
		case http.MethodPost:
			s.enrollParticipantHandler(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
		return
	}

	// Extract participant ID and add to request context for handlers to use
	participantID := segments[0]
	ctx := context.WithValue(r.Context(), ContextKeyParticipantID, participantID)
	r = r.WithContext(ctx)

	if len(segments) == 1 {
		// /intervention/participants/{id}
		switch r.Method {
		case http.MethodGet:
			s.getParticipantHandler(w, r)
		case http.MethodPut:
			s.updateParticipantHandler(w, r)
		case http.MethodDelete:
			s.deleteParticipantHandler(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
		return
	}

	// Handle sub-routes for specific participant
	switch segments[1] {
	case "responses":
		if r.Method == http.MethodPost {
			s.processResponseHandler(w, r)
		} else {
			w.Header().Set("Allow", "POST")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
	case "advance":
		if r.Method == http.MethodPost {
			s.advanceStateHandler(w, r)
		} else {
			w.Header().Set("Allow", "POST")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
	case "reset":
		if r.Method == http.MethodPost {
			s.resetParticipantHandler(w, r)
		} else {
			w.Header().Set("Allow", "POST")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
	case "history":
		if r.Method == http.MethodGet {
			s.getParticipantHistoryHandler(w, r)
		} else {
			w.Header().Set("Allow", "GET")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
	default:
		writeJSONResponse(w, http.StatusNotFound, models.Error("Unknown participant endpoint"))
	}
}
