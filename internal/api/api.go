// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

var (
	waClient    whatsapp.WhatsAppSender
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

	// Initialize WhatsApp client
	waClient, err = whatsapp.NewClient(waOpts...)
	if err != nil {
		log.Fatalf("Failed to create WhatsApp client: %v", err)
	}

	// Initialize scheduler
	sched = scheduler.NewScheduler()

	// Configure default schedule
	defaultCron = apiCfg.DefaultCron

	// Choose storage backend: Postgres if DSN provided via options, else in-memory
	if len(storeOpts) > 0 {
		ps, err := store.NewPostgresStore(storeOpts...)
		if err != nil {
			log.Fatalf("Failed to connect to Postgres store: %v", err)
		}
		st = ps
	} else {
		st = store.NewInMemoryStore()
	}

	// Initialize GenAI client if API key provided via options
	if len(genaiOpts) > 0 {
		gaClient, err = genai.NewClient(genaiOpts...)
		if err != nil {
			log.Fatalf("Failed to create GenAI client: %v", err)
		}
	} else {
		gaClient = nil
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
		log.Printf("PromptPipe API running on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API server error: %v", err)
		}
	}()
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server Shutdown failed: %v", err)
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
		log.Printf("Invalid JSON in sendHandler: %v", err)
		return
	}

	// Determine prompt type if not specified
	if p.Type == "" {
		switch {
		case p.SystemPrompt != "" && p.UserPrompt != "" && gaClient != nil:
			p.Type = models.PromptTypeGenAI
		case len(p.BranchOptions) > 0:
			p.Type = models.PromptTypeBranch
		default:
			p.Type = models.PromptTypeStatic
		}
	}
	// Validate prompt based on type
	switch p.Type {
	case models.PromptTypeStatic:
		if p.Body == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeGenAI:
		if gaClient == nil || p.SystemPrompt == "" || p.UserPrompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	case models.PromptTypeBranch:
		if len(p.BranchOptions) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Construct message body based on type
	var msgBody string
	switch p.Type {
	case models.PromptTypeStatic:
		msgBody = p.Body
	case models.PromptTypeGenAI:
		generated, err := gaClient.GeneratePrompt(p.SystemPrompt, p.UserPrompt)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("GenAI generation error: %v", err)
			return
		}
		msgBody = generated
		p.Body = generated
	case models.PromptTypeBranch:
		// Format branch options
		sb := fmt.Sprintf("%s\n", p.Body)
		for i, opt := range p.BranchOptions {
			sb += fmt.Sprintf("%d. %s: %s\n", i+1, opt.Label, opt.Body)
		}
		msgBody = sb
	}

	err := waClient.SendMessage(context.Background(), p.To, msgBody)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error sending message: %v", err)
		return
	}
	if err := st.AddReceipt(models.Receipt{To: p.To, Status: "sent", Time: time.Now().Unix()}); err != nil {
		log.Printf("Error adding receipt: %v", err)
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
		log.Printf("Invalid JSON in scheduleHandler: %v", err)
		return
	}
	// Determine prompt type if not specified
	if p.Type == "" {
		switch {
		case p.SystemPrompt != "" && p.UserPrompt != "" && gaClient != nil:
			p.Type = models.PromptTypeGenAI
		case len(p.BranchOptions) > 0:
			p.Type = models.PromptTypeBranch
		default:
			p.Type = models.PromptTypeStatic
		}
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
		// Construct message based on prompt type
		var msg string
		switch job.Type {
		case models.PromptTypeStatic:
			msg = job.Body
		case models.PromptTypeGenAI:
			if gaClient != nil {
				gen, err := gaClient.GeneratePrompt(job.SystemPrompt, job.UserPrompt)
				if err != nil {
					log.Printf("GenAI generation error: %v", err)
					return
				}
				msg = gen
			} else {
				msg = job.Body
			}
		case models.PromptTypeBranch:
			sb := fmt.Sprintf("%s\n", job.Body)
			for i, opt := range job.BranchOptions {
				sb += fmt.Sprintf("%d. %s: %s\n", i+1, opt.Label, opt.Body)
			}
			msg = sb
		}
		if err := waClient.SendMessage(context.Background(), job.To, msg); err != nil {
			log.Printf("Error sending scheduled message: %v", err)
		}
		if err := st.AddReceipt(models.Receipt{To: job.To, Status: "sent", Time: time.Now().Unix()}); err != nil {
			log.Printf("Error adding scheduled receipt: %v", err)
		}
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error scheduling job: %v", err)
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
		log.Printf("Error encoding receipts response: %v", err)
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
		log.Printf("Invalid JSON in responseHandler: %v", err)
		return
	}
	resp.Time = time.Now().Unix()
	if err := st.AddResponse(resp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error adding response: %v", err)
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
		log.Printf("Error encoding responses response: %v", err)
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
		log.Printf("Error encoding stats response: %v", err)
	}
}
