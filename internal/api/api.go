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

	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/receipts", receiptsHandler)
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
		if err := waClient.SendMessage(context.Background(), p.To, p.Body); err != nil {
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
