package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

func TestSendHandler_NotImplemented(t *testing.T) {
	// Save and restore global variables
	oldStore := st
	oldWA := waClient
	defer func() {
		st = oldStore
		waClient = oldWA
	}()
	st = store.NewInMemoryStore()
	waClient, _ = whatsapp.NewClient()

	req, _ := http.NewRequest("POST", "/send", bytes.NewBuffer([]byte(`{"to":"+123","body":"hi"}`)))
	rr := httptest.NewRecorder()
	sendHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestScheduleHandler_NotImplemented(t *testing.T) {
	// Save and restore global variables
	oldSched := sched
	oldStore := st
	oldWA := waClient
	defer func() {
		sched = oldSched
		st = oldStore
		waClient = oldWA
	}()
	sched = scheduler.NewScheduler()
	st = store.NewInMemoryStore()
	waClient, _ = whatsapp.NewClient()

	req, _ := http.NewRequest("POST", "/schedule", bytes.NewBuffer([]byte(`{"to":"+123","cron":"* * * * *","body":"hi"}`)))
	rr := httptest.NewRecorder()
	scheduleHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReceiptsHandler_NotImplemented(t *testing.T) {
	// Save and restore global variables
	oldStore := st
	defer func() { st = oldStore }()
	st = store.NewInMemoryStore()

	req, _ := http.NewRequest("GET", "/receipts", nil)
	rr := httptest.NewRecorder()
	receiptsHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
