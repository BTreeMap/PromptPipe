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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/recovery"
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
	msgService                     messaging.Service
	respHandler                    *messaging.ResponseHandler
	st                             store.Store
	timer                          models.Timer
	defaultSchedule                *models.Schedule
	gaClient                       *genai.Client
	intakeBotPromptFile            string // path to intake bot system prompt file
	promptGeneratorPromptFile      string // path to prompt generator system prompt file
	feedbackTrackerPromptFile      string // path to feedback tracker system prompt file
	chatHistoryLimit               int    // limit for number of history messages sent to bot tools
	feedbackInitialTimeout         string // timeout for initial feedback response
	feedbackFollowupDelay          string // delay before follow-up feedback session
	debugMode                      bool   // enable debug mode for user-facing debug messages
	schedulerPrepTimeMinutes       int    // preparation time in minutes before scheduled habit reminders
	autoFeedbackAfterPromptEnabled bool   // enable auto feedback session enforcement after scheduled prompt inactivity
	autoEnrollNewUsers             bool   // enable automatic enrollment of new users on first message
}

// NewServer creates a new API server instance with the provided dependencies.
func NewServer(msgService messaging.Service, st store.Store, timer models.Timer, defaultSchedule *models.Schedule, gaClient *genai.Client) *Server {
	// Create response handler with auto-enrollment disabled by default
	respHandler := messaging.NewResponseHandler(msgService, st, false)

	return &Server{
		msgService:      msgService,
		respHandler:     respHandler,
		st:              st,
		timer:           timer,
		defaultSchedule: defaultSchedule,
		gaClient:        gaClient,
	}
}

// Opts holds configuration options for the API server, such as HTTP address and default schedule.
type Opts struct {
	Addr                           string           // overrides API_ADDR
	DefaultSchedule                *models.Schedule // overrides DEFAULT_SCHEDULE
	IntakeBotPromptFile            string           // path to intake bot system prompt file
	PromptGeneratorPromptFile      string           // path to prompt generator system prompt file
	FeedbackTrackerPromptFile      string           // path to feedback tracker system prompt file
	ChatHistoryLimit               int              // limit for number of history messages sent to bot tools
	FeedbackInitialTimeout         string           // timeout for initial feedback response
	FeedbackFollowupDelay          string           // delay before follow-up feedback session
	DebugMode                      bool             // enable debug mode for user-facing debug messages
	SchedulerPrepTimeMinutes       int              // preparation time in minutes before scheduled habit reminders
	AutoFeedbackAfterPromptEnabled bool             // enable auto feedback session enforcement after scheduled prompt inactivity
	AutoEnrollNewUsers             bool             // enable automatic enrollment of new users on first message
}

// Option defines a configuration option for the API server.
type Option func(*Opts)

// WithAddr overrides the server address for the API.
func WithAddr(addr string) Option {
	return func(o *Opts) {
		o.Addr = addr
	}
}

// WithDefaultSchedule overrides the default schedule for prompts.
func WithDefaultSchedule(schedule *models.Schedule) Option {
	return func(o *Opts) {
		o.DefaultSchedule = schedule
	}
}

// WithDefaultCron overrides the default schedule for prompts using a cron-like format.
// This is a compatibility function that converts cron string to Schedule struct.
func WithDefaultCron(cron string) Option {
	// Parse cron string (format: "minute hour day month weekday")
	schedule, err := parseCronToSchedule(cron)
	if err != nil {
		slog.Warn("Server.WithDefaultCron: failed to parse cron string", "cron", cron, "error", err)
		schedule = nil
	}
	return func(o *Opts) {
		o.DefaultSchedule = schedule
	}
}

// WithIntakeBotPromptFile sets the path to the intake bot system prompt file.
func WithIntakeBotPromptFile(filePath string) Option {
	return func(o *Opts) {
		o.IntakeBotPromptFile = filePath
	}
}

// WithPromptGeneratorPromptFile sets the path to the prompt generator system prompt file.
func WithPromptGeneratorPromptFile(filePath string) Option {
	return func(o *Opts) {
		o.PromptGeneratorPromptFile = filePath
	}
}

// WithFeedbackTrackerPromptFile sets the path to the feedback tracker system prompt file.
func WithFeedbackTrackerPromptFile(filePath string) Option {
	return func(o *Opts) {
		o.FeedbackTrackerPromptFile = filePath
	}
}

// WithChatHistoryLimit sets the limit for number of history messages sent to bot tools.
// -1: no limit, 0: no history, positive: limit to last N messages
func WithChatHistoryLimit(limit int) Option {
	return func(o *Opts) {
		o.ChatHistoryLimit = limit
	}
}

// WithFeedbackInitialTimeout sets the timeout for initial feedback response.
func WithFeedbackInitialTimeout(timeout string) Option {
	return func(o *Opts) {
		o.FeedbackInitialTimeout = timeout
	}
}

// WithFeedbackFollowupDelay sets the delay before follow-up feedback session.
func WithFeedbackFollowupDelay(delay string) Option {
	return func(o *Opts) {
		o.FeedbackFollowupDelay = delay
	}
}

// WithDebugMode enables debug mode for user-facing debug messages.
func WithDebugMode(enabled bool) Option {
	return func(o *Opts) {
		o.DebugMode = enabled
	}
}

// WithSchedulerPrepTimeMinutes sets the preparation time in minutes before scheduled habit reminders.
func WithSchedulerPrepTimeMinutes(minutes int) Option {
	return func(o *Opts) {
		o.SchedulerPrepTimeMinutes = minutes
	}
}

// WithAutoFeedbackAfterPromptEnabled enables/disables auto feedback enforcement after scheduled prompts.
func WithAutoFeedbackAfterPromptEnabled(enabled bool) Option {
	return func(o *Opts) {
		o.AutoFeedbackAfterPromptEnabled = enabled
	}
}

// WithAutoEnrollNewUsers enables/disables automatic enrollment of new users on first message.
func WithAutoEnrollNewUsers(enabled bool) Option {
	return func(o *Opts) {
		o.AutoEnrollNewUsers = enabled
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

	// Store prompt file paths and configuration in server
	server.intakeBotPromptFile = apiCfg.IntakeBotPromptFile
	server.promptGeneratorPromptFile = apiCfg.PromptGeneratorPromptFile
	server.feedbackTrackerPromptFile = apiCfg.FeedbackTrackerPromptFile
	server.chatHistoryLimit = apiCfg.ChatHistoryLimit
	server.feedbackInitialTimeout = apiCfg.FeedbackInitialTimeout
	server.feedbackFollowupDelay = apiCfg.FeedbackFollowupDelay
	server.debugMode = apiCfg.DebugMode
	server.schedulerPrepTimeMinutes = apiCfg.SchedulerPrepTimeMinutes
	server.autoFeedbackAfterPromptEnabled = apiCfg.AutoFeedbackAfterPromptEnabled
	server.autoEnrollNewUsers = apiCfg.AutoEnrollNewUsers

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

	// Initialize store
	if err := server.initializeStore(storeOpts); err != nil {
		return nil, "", fmt.Errorf("failed to initialize store: %w", err)
	}

	// Create response handler (after store is initialized)
	server.respHandler = messaging.NewResponseHandler(server.msgService, server.st, server.autoEnrollNewUsers)
	slog.Debug("Response handler initialized", "autoEnrollNewUsers", server.autoEnrollNewUsers)

	// Start messaging service
	if err := server.msgService.Start(context.Background()); err != nil {
		slog.Error("Failed to start messaging service", "error", err)
		return nil, "", fmt.Errorf("failed to start messaging service: %w", err)
	}
	slog.Debug("Messaging service started")

	// Initialize timer
	server.timer = flow.NewSimpleTimer()
	slog.Debug("Timer initialized")

	// Configure default schedule
	server.defaultSchedule = apiCfg.DefaultSchedule
	slog.Debug("Default schedule set", "defaultSchedule", apiCfg.DefaultSchedule)

	// Initialize GenAI client if options provided
	if err := server.initializeGenAI(genaiOpts); err != nil {
		return nil, "", fmt.Errorf("failed to initialize GenAI: %w", err)
	}

	// Initialize conversation flow
	if err := server.initializeConversationFlow(); err != nil {
		return nil, "", fmt.Errorf("failed to initialize conversation flow: %w", err)
	}

	// Initialize application state recovery
	if err := server.initializeRecovery(); err != nil {
		return nil, "", fmt.Errorf("failed to initialize recovery: %w", err)
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

// initializeConversationFlow sets up the conversation flow with system prompt loading and scheduler tool
func (s *Server) initializeConversationFlow() error {
	// Create conversation flow with dependencies and scheduler tool
	stateManager := flow.NewStoreBasedStateManager(s.st)

	// Create conversation flow with all tools for the 3-bot architecture
	// Handle typed nil interface issue - if gaClient is a nil pointer, pass nil interface
	var genaiClientInterface genai.ClientInterface
	if s.gaClient != nil {
		genaiClientInterface = s.gaClient
	}

	// Use the new 3-bot system prompt
	systemPromptFile3Bot := "prompts/conversation_system_3bot.txt"
	conversationFlow := flow.NewConversationFlowWithAllTools(
		stateManager,
		genaiClientInterface,
		systemPromptFile3Bot,
		s.msgService,
		s.intakeBotPromptFile,
		s.promptGeneratorPromptFile,
		s.feedbackTrackerPromptFile,
		s.feedbackInitialTimeout,
		s.feedbackFollowupDelay,
		s.schedulerPrepTimeMinutes,
		s.autoFeedbackAfterPromptEnabled,
	)

	// Set the chat history limit
	conversationFlow.SetChatHistoryLimit(s.chatHistoryLimit)

	// Set debug mode
	conversationFlow.SetDebugMode(s.debugMode)

	// Load system prompt for the conversation flow (fallback to default if it doesn't exist)
	if err := conversationFlow.LoadSystemPrompt(); err != nil {
		slog.Warn("Failed to load 3-bot system prompt, using default", "error", err, "systemPromptFile", systemPromptFile3Bot)
		// Don't fail initialization, just log warning
	}

	// Load system prompts for all tools
	if err := conversationFlow.LoadToolSystemPrompts(); err != nil {
		slog.Warn("Failed to load some tool system prompts", "error", err)
		// Don't fail initialization, just log warning
	}

	// Register conversation flow generator
	flow.Register(models.PromptTypeConversation, conversationFlow)
	slog.Debug("Conversation flow initialized with 3-bot architecture", "systemPromptFile", systemPromptFile3Bot, "hasGenAI", s.gaClient != nil, "intakeBotPromptFile", s.intakeBotPromptFile, "promptGeneratorPromptFile", s.promptGeneratorPromptFile, "feedbackTrackerPromptFile", s.feedbackTrackerPromptFile, "chatHistoryLimit", s.chatHistoryLimit)

	return nil
}

// initializeRecovery sets up application state recovery system
func (s *Server) initializeRecovery() error {
	slog.Info("Initializing application state recovery system")

	// Create recovery manager
	recoveryManager := recovery.NewRecoveryManager(s.st, s.timer)

	// Create state manager for flow recoveries
	stateManager := flow.NewStoreBasedStateManager(s.st)

	// Register flow recoveries
	conversationRecovery := flow.NewConversationFlowRecovery()

	recoveryManager.RegisterRecoverable(conversationRecovery)

	// Register infrastructure recovery callbacks
	recoveryManager.RegisterTimerRecovery(recovery.TimerRecoveryHandler(s.timer))

	// Create response handler recovery callback that avoids import cycles
	handlerRecoveryCallback := func(info recovery.ResponseHandlerRecoveryInfo) error {
		slog.Debug("Processing response handler recovery",
			"phone", info.PhoneNumber, "flowType", info.FlowType)

		switch info.FlowType {
		case models.FlowTypeConversation:
			// Create conversation hook
			hook := messaging.CreateStaticHook(s.msgService)
			if err := s.respHandler.RegisterHook(info.PhoneNumber, hook); err != nil {
				return fmt.Errorf("failed to register conversation hook: %w", err)
			}

		default:
			return fmt.Errorf("unknown flow type for response handler recovery: %s", info.FlowType)
		}

		slog.Debug("Successfully registered response handler",
			"phone", info.PhoneNumber, "flowType", info.FlowType)
		return nil
	}

	recoveryManager.RegisterHandlerRecovery(recovery.CreateResponseHandlerRecoveryHandler(handlerRecoveryCallback))

	// Set dependencies for ResponseHandler to enable persistent hook creation
	s.respHandler.SetDependencies(stateManager, s.timer)

	// Perform recovery
	ctx := context.Background()
	if err := recoveryManager.RecoverAll(ctx); err != nil {
		slog.Warn("Recovery completed with errors", "error", err)
		// Don't fail startup for recovery errors, just log them
	}

	// Recover persistent hooks from database
	if err := s.respHandler.RecoverPersistentHooks(ctx); err != nil {
		slog.Warn("Failed to recover persistent hooks", "error", err)
		// Don't fail startup for hook recovery errors, just log them
	}

	// Validate and cleanup response handler hooks based on active participants
	// This ensures hooks only exist for currently active participants
	if err := s.respHandler.ValidateAndCleanupHooks(ctx); err != nil {
		slog.Warn("Response handler validation completed with errors", "error", err)
		// Don't fail startup for validation errors, just log them
	}

	// Clean up stale hooks from database
	if err := s.respHandler.CleanupStaleHooks(ctx); err != nil {
		slog.Warn("Failed to cleanup stale hooks", "error", err)
		// Don't fail startup for cleanup errors, just log them
	}

	// Recover pending daily prompt reminders through the SchedulerTool
	// This must happen after conversation flow is initialized
	if err := s.recoverSchedulerReminders(ctx); err != nil {
		slog.Warn("Failed to recover scheduler reminders", "error", err)
		// Don't fail startup for scheduler recovery errors, just log them
	}

	slog.Info("Application state recovery system initialized successfully")
	return nil
}

// recoverSchedulerReminders recovers pending daily prompt reminders after server restart
func (s *Server) recoverSchedulerReminders(ctx context.Context) error {
	slog.Info("Recovering scheduler reminders for active conversation participants")

	// Get the conversation flow
	conversationFlowInterface, exists := flow.Get(models.PromptTypeConversation)
	if !exists || conversationFlowInterface == nil {
		return fmt.Errorf("conversation flow not registered")
	}

	conversationFlow, ok := conversationFlowInterface.(*flow.ConversationFlow)
	if !ok {
		return fmt.Errorf("conversation flow has unexpected type")
	}

	// Get the scheduler tool
	schedulerTool := conversationFlow.GetSchedulerTool()
	if schedulerTool == nil {
		return fmt.Errorf("scheduler tool not available in conversation flow")
	}

	// Get all active conversation participants
	participants, err := s.st.ListConversationParticipants()
	if err != nil {
		return fmt.Errorf("failed to list conversation participants: %w", err)
	}

	// Filter to only active participants
	var activeParticipantIDs []string
	for _, participant := range participants {
		if participant.Status == models.ConversationStatusActive {
			activeParticipantIDs = append(activeParticipantIDs, participant.ID)
		}
	}

	// Recover reminders for all active participants
	if err := schedulerTool.RecoverPendingReminders(ctx, activeParticipantIDs); err != nil {
		return fmt.Errorf("scheduler reminder recovery completed with errors: %w", err)
	}

	slog.Info("Scheduler reminder recovery completed successfully",
		"activeParticipants", len(activeParticipantIDs),
		"totalParticipants", len(participants))
	return nil
}

// startHTTPServer registers handlers and starts the HTTP server
func startHTTPServer(addr string, server *Server) *http.Server {
	// Register HTTP handlers
	slog.Debug("Registering HTTP handlers")
	http.HandleFunc("/health", server.healthHandler)
	http.HandleFunc("/send", server.sendHandler)
	http.HandleFunc("/schedule", server.scheduleHandler)
	http.HandleFunc("/receipts", server.receiptsHandler)
	// Endpoints for incoming message responses and statistics
	http.HandleFunc("/response", server.responseHandler)
	http.HandleFunc("/responses", server.responsesHandler)
	http.HandleFunc("/stats", server.statsHandler)

	// Timer management endpoints (global scope)
	http.HandleFunc("/timers", server.timersHandler)

	// Conversation management endpoints with proper routing
	http.HandleFunc("/conversation/", server.conversationRouter)

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
		slog.Error("Failed to stop messaging service", "error", err)
		shutdownErrors = append(shutdownErrors, fmt.Errorf("messaging service shutdown failed: %w", err))
	}

	// Check if there were any shutdown errors
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

// parseCronToSchedule converts a cron string to a Schedule struct.
// Supports the basic cron format: "minute hour day month weekday"
func parseCronToSchedule(cron string) (*models.Schedule, error) {
	if cron == "" {
		return nil, errors.New("empty cron string")
	}

	// Split cron string into fields
	fields := strings.Fields(cron)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron format, expected 5 fields, got %d", len(fields))
	}

	schedule := &models.Schedule{}

	// Parse minute (field 0)
	if fields[0] != "*" {
		if minute, err := parseInt(fields[0], 0, 59); err == nil {
			schedule.Minute = &minute
		} else {
			return nil, fmt.Errorf("invalid minute: %s", fields[0])
		}
	}

	// Parse hour (field 1)
	if fields[1] != "*" {
		if hour, err := parseInt(fields[1], 0, 23); err == nil {
			schedule.Hour = &hour
		} else {
			return nil, fmt.Errorf("invalid hour: %s", fields[1])
		}
	}

	// Parse day (field 2)
	if fields[2] != "*" {
		if day, err := parseInt(fields[2], 1, 31); err == nil {
			schedule.Day = &day
		} else {
			return nil, fmt.Errorf("invalid day: %s", fields[2])
		}
	}

	// Parse month (field 3)
	if fields[3] != "*" {
		if month, err := parseInt(fields[3], 1, 12); err == nil {
			schedule.Month = &month
		} else {
			return nil, fmt.Errorf("invalid month: %s", fields[3])
		}
	}

	// Parse weekday (field 4)
	if fields[4] != "*" {
		if weekday, err := parseInt(fields[4], 0, 6); err == nil {
			schedule.Weekday = &weekday
		} else {
			return nil, fmt.Errorf("invalid weekday: %s", fields[4])
		}
	}

	return schedule, nil
}

// parseInt parses an integer with range validation.
func parseInt(s string, min, max int) (int, error) {
	// Handle simple range formats like "1-5" by taking the first value
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		s = parts[0]
	}

	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", val, min, max)
	}
	return val, nil
}

// conversationRouter handles all conversation-related endpoints with proper RESTful routing
func (s *Server) conversationRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/conversation")

	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	slog.Debug("Conversation router", "method", r.Method, "path", path, "fullPath", r.URL.Path)

	// Split path into segments
	segments := strings.Split(path, "/")
	if len(segments) == 0 || segments[0] == "" {
		http.Error(w, "Invalid conversation endpoint", http.StatusNotFound)
		return
	}

	switch segments[0] {
	case "participants":
		s.handleConversationParticipantRoutes(w, r, segments[1:])
	default:
		http.Error(w, "Unknown conversation endpoint", http.StatusNotFound)
	}
}

// handleConversationParticipantRoutes handles all conversation participant-related routes
func (s *Server) handleConversationParticipantRoutes(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 || segments[0] == "" {
		// /conversation/participants
		switch r.Method {
		case http.MethodGet:
			s.listConversationParticipantsHandler(w, r)
		case http.MethodPost:
			s.enrollConversationParticipantHandler(w, r)
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
		// /conversation/participants/{id}
		switch r.Method {
		case http.MethodGet:
			s.getConversationParticipantHandler(w, r)
		case http.MethodPut:
			s.updateConversationParticipantHandler(w, r)
		case http.MethodDelete:
			s.deleteConversationParticipantHandler(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			writeJSONResponse(w, http.StatusMethodNotAllowed, models.Error("Method not allowed"))
		}
		return
	}

	// For now, we don't have sub-routes like intervention has
	// But we can add them later (e.g., /conversation/participants/{id}/history)
	writeJSONResponse(w, http.StatusNotFound, models.Error("Unknown conversation participant endpoint"))
}
