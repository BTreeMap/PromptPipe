// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

var (
	waClient whatsapp.WhatsAppSender
	sched    *scheduler.Scheduler
	st       store.Store // Use the interface for flexibility
)

// Run starts the API server and initializes dependencies.
func Run() {
	var err error
	waClient, err = whatsapp.NewClient() // TODO: handle error and config
	if err != nil {
		log.Fatalf("Failed to create WhatsApp client: %v", err)
	}
	sched = scheduler.NewScheduler()
	st = store.NewInMemoryStore() // Or use store.NewPostgresStore(connStr) for production

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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error reading request body: %v", err)
		return
	}
	if err := json.Unmarshal(body, &p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = waClient.SendMessage(context.Background(), p.To, p.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	st.AddReceipt(models.Receipt{To: p.To, Status: "sent", Time: time.Now().Unix()})
	w.WriteHeader(http.StatusOK)
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error reading request body: %v", err)
		return
	}
	if err := json.Unmarshal(body, &p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = sched.AddJob(p.Cron, func() {
		err := waClient.SendMessage(context.Background(), p.To, p.Body)
		if err == nil {
			st.AddReceipt(models.Receipt{To: p.To, Status: "sent", Time: time.Now().Unix()})
		}
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(receipts)
}
