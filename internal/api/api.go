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
	msgService = messaging.NewWhatsAppService(whClient)
	// Start messaging service
	if err := msgService.Start(context.Background()); err != nil {
		slog.Error("Failed to start messaging service", "error", err)
		os.Exit(1)
	}
	// Forward receipts and responses to store
	go func() {
		for r := range msgService.Receipts() {
			if err := st.AddReceipt(r); err != nil {
				slog.Error("Error storing receipt", "error", err)
			}
		}
	}()
	go func() {
		for resp := range msgService.Responses() {
			if err := st.AddResponse(resp); err != nil {
				slog.Error("Error storing response", "error", err)
			}
		}
	}()

	// Initialize scheduler
	sched = scheduler.NewScheduler()

	// Configure default schedule
	defaultCron = apiCfg.DefaultCron

	// Choose storage backend: Postgres if DSN provided via options, else in-memory
	if len(storeOpts) > 0 {
		ps, err := store.NewPostgresStore(storeOpts...)
		if err != nil {
			slog.Error("Failed to connect to Postgres store", "error", err)
			os.Exit(1)
		}
		st = ps
	} else {
		st = store.NewInMemoryStore()
	}

	// Initialize GenAI client if API key provided via options
	if len(genaiOpts) > 0 {
		gaClient, err = genai.NewClient(genaiOpts...)
		if err != nil {
			slog.Error("Failed to create GenAI client", "error", err)
			os.Exit(1)
		}
		// Register GenAI flow generator
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: gaClient})
	} else {
		gaClient = nil
	}

	// Register flow generators
	// Static and branch are registered init(); register GenAI if available
	if gaClient != nil {
		flow.Register(models.PromptTypeGenAI, &flow.GenAIGenerator{Client: gaClient})
	}

	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/receipts", receiptsHandler)
	// Endpoints for incoming message responses and statistics
	http.HandleFunc("/response", responseHandler)
	http.HandleFunc("/responses", responsesHandler)
	http.HandleFunc("/stats", statsHandler)
	// Start HTTP server with graceful shutdown
	srv := &http.Server{Addr: addr, Handler: nil}
	go func() {
		slog.Info("PromptPipe API running", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
			os.Exit(1)
		}
	}()
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server")
	if err := srv.Shutdown(context.Background()); err != nil {
		slog.Error("Server Shutdown failed", "error", err)
		os.Exit(1)
	}
	sched.Stop()
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		slog.Warn("Invalid JSON in sendHandler", "error", err)
		return
	}

	// Validate required field: to
	if p.To == "" {
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
		w.WriteHeader(http.StatusBadRequest)
		slog.Error("Flow generation error in sendHandler", "error", err)
		return
	}

	err = msgService.SendMessage(context.Background(), p.To, msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error("Error sending message", "error", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		slog.Warn("Invalid JSON in scheduleHandler", "error", err)
		return
	}
	// Validate required fields: to
	if p.To == "" {
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeGenAI:
		if p.To == "" || gaClient == nil || p.SystemPrompt == "" || p.UserPrompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeBranch:
		if p.To == "" || len(p.BranchOptions) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Apply default schedule if none provided
	if p.Cron == "" {
		if defaultCron == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.Cron = defaultCron
	}
	// Capture prompt locally for closure
	job := p
	if err := sched.AddJob(p.Cron, func() {
		// Generate message body via flow
		msg, err := flow.Generate(context.Background(), job)
		if err != nil {
			slog.Error("Flow generation error in scheduled job", "error", err)
			return
		}
		if err := msgService.SendMessage(context.Background(), job.To, msg); err != nil {
			slog.Error("Scheduled job send error", "error", err)
			return
		}
		if err := st.AddReceipt(models.Receipt{To: job.To, Status: "sent", Time: time.Now().Unix()}); err != nil {
			slog.Error("Error adding scheduled receipt", "error", err)
		}
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error("Error scheduling job", "error", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func receiptsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	receipts, err := st.GetReceipts()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(receipts); err != nil {
		slog.Error("Error encoding receipts response", "error", err)
	}
}

// responseHandler handles incoming participant responses (POST /response).
func responseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var resp models.Response
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		slog.Warn("Invalid JSON in responseHandler", "error", err)
		return
	}
	resp.Time = time.Now().Unix()
	if err := st.AddResponse(resp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		slog.Error("Error adding response", "error", err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// responsesHandler returns all collected responses (GET /responses).
func responsesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := st.GetResponses()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		slog.Error("Error encoding responses response", "error", err)
	}
}

// statsHandler returns statistics about collected responses (GET /stats).
func statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responses, err := st.GetResponses()
	if err != nil {
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
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		slog.Error("Error encoding stats response", "error", err)
	}
}
