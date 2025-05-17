// Package api provides HTTP handlers and the main API server logic for PromptPipe.
//
// It exposes RESTful endpoints for scheduling, sending, and tracking WhatsApp prompts.
// The API integrates with the WhatsApp, scheduler, and store modules.
package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

var (
	waClient *whatsapp.Client
	sched    *scheduler.Scheduler
	st       *store.InMemoryStore
)

func Run() {
	waClient, _ = whatsapp.NewClient() // TODO: handle error and config
	sched = scheduler.NewScheduler()
	st = store.NewInMemoryStore()

	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/schedule", scheduleHandler)
	http.HandleFunc("/receipts", receiptsHandler)
	go sched.Start()
	log.Println("PromptPipe API running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p models.Prompt
	body, _ := ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(body, &p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err := waClient.SendMessage(context.Background(), p.To, p.Body)
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
	body, _ := ioutil.ReadAll(r.Body)
	if err := json.Unmarshal(body, &p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sched.AddJob(p.Cron, func() {
		err := waClient.SendMessage(context.Background(), p.To, p.Body)
		if err == nil {
			st.AddReceipt(models.Receipt{To: p.To, Status: "sent", Time: time.Now().Unix()})
		}
	})
	w.WriteHeader(http.StatusOK)
}

func receiptsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if st == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	receipts := st.GetReceipts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(receipts)
}
