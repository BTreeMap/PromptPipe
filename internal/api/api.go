// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
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
	defaultCron string      // default cron schedule from env
	gaClient    *genai.Client
)

// Run starts the API server and initializes dependencies.
func Run() {
	var err error
	waClient, err = whatsapp.NewClient()
	if err != nil {
		log.Fatalf("Failed to create WhatsApp client: %v", err)
	}
	sched = scheduler.NewScheduler()
	// Load default schedule from environment
	defaultCron = os.Getenv("DEFAULT_SCHEDULE")
	// Choose storage backend based on DATABASE_URL
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		ps, err := store.NewPostgresStore(dbURL)
		if err != nil {
			log.Fatalf("Failed to connect to Postgres store: %v", err)
		}
		st = ps
	} else {
		st = store.NewInMemoryStore()
	}

	// Initialize GenAI client if API key provided
	gaClient, err = genai.NewClient()
	if err != nil {
		log.Fatalf("Failed to create GenAI client: %v", err)
	}

	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/receipts", receiptsHandler)
	// Endpoints for incoming message responses and statistics
	http.HandleFunc("/response", responseHandler)
	http.HandleFunc("/responses", responsesHandler)
	http.HandleFunc("/stats", statsHandler)
	log.Println("PromptPipe API running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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

	// If GenAI prompts provided, generate dynamic content
	if p.SystemPrompt != "" && p.UserPrompt != "" {
		generated, err := gaClient.GeneratePrompt(p.SystemPrompt, p.UserPrompt)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("GenAI generation error: %v", err)
			return
		}
		p.Body = generated
	}

	err := waClient.SendMessage(context.Background(), p.To, p.Body)
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
	// Apply default schedule if none provided
	if p.Cron == "" {
		if defaultCron == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p.Cron = defaultCron
	}
	if err := sched.AddJob(p.Cron, func() {
		msg := p.Body
		// If GenAI prompts provided, generate dynamic content per run
		if p.SystemPrompt != "" && p.UserPrompt != "" {
			gen, err := gaClient.GeneratePrompt(p.SystemPrompt, p.UserPrompt)
			if err != nil {
				log.Printf("GenAI scheduled generation error: %v", err)
				return
			}
			msg = gen
		}
		if err := waClient.SendMessage(context.Background(), p.To, msg); err != nil {
			log.Printf("Scheduled job send error: %v", err)
			return
		}
		if err := st.AddReceipt(models.Receipt{To: p.To, Status: "sent", Time: time.Now().Unix()}); err != nil {
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
