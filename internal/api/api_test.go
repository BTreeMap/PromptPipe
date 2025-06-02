package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

func TestSendHandler_NotImplemented(t *testing.T) {
	// Save and restore global variables
	oldStore := st
	oldService := msgService
	defer func() {
		st = oldStore
		msgService = oldService
	}()
	st = store.NewInMemoryStore()
	msgService = messaging.NewWhatsAppService(whatsapp.NewMockClient())

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
	oldService := msgService
	defer func() {
		sched = oldSched
		st = oldStore
		msgService = oldService
	}()
	sched = scheduler.NewScheduler()
	st = store.NewInMemoryStore()
	msgService = messaging.NewWhatsAppService(whatsapp.NewMockClient())

	req, _ := http.NewRequest("POST", "/schedule", bytes.NewBuffer([]byte(`{"to":"+123","cron":"* * * * *","body":"hi"}`)))
	rr := httptest.NewRecorder()
	scheduleHandler(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
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

// Tests for response and statistics endpoints
func TestResponseHandler_MethodNotAllowed(t *testing.T) {
	oldStore := st
	defer func() { st = oldStore }()
	st = store.NewInMemoryStore()

	req, _ := http.NewRequest("GET", "/response", nil)
	rr := httptest.NewRecorder()
	responseHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /response, got %d", rr.Code)
	}
}

func TestResponseHandler_OK(t *testing.T) {
	oldStore := st
	defer func() { st = oldStore }()
	st = store.NewInMemoryStore()

	payload := `{"from":"+123","body":"hello"}`
	req, _ := http.NewRequest("POST", "/response", bytes.NewBuffer([]byte(payload)))
	rr := httptest.NewRecorder()
	responseHandler(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 for POST /response, got %d", rr.Code)
	}
	reps, err := st.GetResponses()
	if err != nil || len(reps) != 1 || reps[0].From != "+123" || reps[0].Body != "hello" {
		t.Errorf("response not stored correctly: %v, %v", reps, err)
	}
}

func TestResponsesHandler_MethodNotAllowed(t *testing.T) {
	oldStore := st
	defer func() { st = oldStore }()
	st = store.NewInMemoryStore()

	req, _ := http.NewRequest("POST", "/responses", nil)
	rr := httptest.NewRecorder()
	responsesHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /responses, got %d", rr.Code)
	}
}

func TestResponsesHandler_OK(t *testing.T) {
	oldStore := st
	defer func() { st = oldStore }()
	s := store.NewInMemoryStore()
	st = s
	// seed one response
	s.AddResponse(models.Response{From: "+123", Body: "hi", Time: 1})

	req, _ := http.NewRequest("GET", "/responses", nil)
	rr := httptest.NewRecorder()
	responsesHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET /responses, got %d", rr.Code)
	}
	var reps []models.Response
	if err := json.NewDecoder(rr.Body).Decode(&reps); err != nil || len(reps) != 1 {
		t.Errorf("unexpected responses output: %v, err: %v", rr.Body.String(), err)
	}
}

func TestStatsHandler_OK(t *testing.T) {
	oldStore := st
	defer func() { st = oldStore }()
	s := store.NewInMemoryStore()
	st = s
	// seed responses
	s.AddResponse(models.Response{From: "+1", Body: "a", Time: 1})
	s.AddResponse(models.Response{From: "+1", Body: "bb", Time: 2})
	s.AddResponse(models.Response{From: "+2", Body: "ccc", Time: 3})

	req, _ := http.NewRequest("GET", "/stats", nil)
	rr := httptest.NewRecorder()
	statsHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET /stats, got %d", rr.Code)
	}
	var stats map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&stats); err != nil {
		t.Errorf("invalid stats JSON: %v", err)
	}
	if stats["total_responses"].(float64) != 3 {
		t.Errorf("wrong total_responses: %v", stats["total_responses"])
	}
}
