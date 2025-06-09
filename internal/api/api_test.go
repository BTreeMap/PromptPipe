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

// newTestServer creates a Server instance for testing with in-memory dependencies.
func newTestServer() *Server {
	return &Server{
		msgService:  messaging.NewWhatsAppService(whatsapp.NewMockClient()),
		sched:       scheduler.NewScheduler(),
		st:          store.NewInMemoryStore(),
		defaultCron: "",
		gaClient:    nil,
	}
}

func TestSendHandler_Success(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/send", bytes.NewBuffer([]byte(`{"to":"+123","body":"hi"}`)))
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response["status"])
	}
}

func TestSendHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/send", nil)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestSendHandler_MissingRecipient(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/send", bytes.NewBuffer([]byte(`{"body":"hi"}`)))
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestScheduleHandler_Success(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/schedule", bytes.NewBuffer([]byte(`{"to":"+123","cron":"* * * * *","body":"hi"}`)))
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if response["status"] != "scheduled" {
		t.Errorf("expected status 'scheduled', got '%s'", response["status"])
	}
}

func TestScheduleHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/schedule", nil)
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestScheduleHandler_MissingRecipient(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/schedule", bytes.NewBuffer([]byte(`{"cron":"* * * * *","body":"hi"}`)))
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestReceiptsHandler_Success(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/receipts", nil)
	rr := httptest.NewRecorder()
	server.receiptsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var receipts []models.Receipt
	if err := json.NewDecoder(rr.Body).Decode(&receipts); err != nil {
		t.Errorf("failed to decode receipts: %v", err)
	}
}

func TestReceiptsHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/receipts", nil)
	rr := httptest.NewRecorder()
	server.receiptsHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestResponseHandler_Success(t *testing.T) {
	server := newTestServer()

	payload := `{"from":"+123","body":"hello"}`
	req, _ := http.NewRequest("POST", "/response", bytes.NewBuffer([]byte(payload)))
	rr := httptest.NewRecorder()
	server.responseHandler(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 for POST /response, got %d", rr.Code)
	}

	// Verify the response was stored
	responses, err := server.st.GetResponses()
	if err != nil {
		t.Errorf("failed to get responses: %v", err)
	}

	if len(responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(responses))
	}

	if responses[0].From != "+123" || responses[0].Body != "hello" {
		t.Errorf("response not stored correctly: %+v", responses[0])
	}

	var response map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if response["status"] != "recorded" {
		t.Errorf("expected status 'recorded', got '%s'", response["status"])
	}
}

func TestResponseHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/response", nil)
	rr := httptest.NewRecorder()
	server.responseHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /response, got %d", rr.Code)
	}
}

func TestResponsesHandler_Success(t *testing.T) {
	server := newTestServer()

	// Seed one response
	server.st.AddResponse(models.Response{From: "+123", Body: "hi", Time: 1})

	req, _ := http.NewRequest("GET", "/responses", nil)
	rr := httptest.NewRecorder()
	server.responsesHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET /responses, got %d", rr.Code)
	}

	var responses []models.Response
	if err := json.NewDecoder(rr.Body).Decode(&responses); err != nil {
		t.Errorf("failed to decode responses: %v", err)
	}

	if len(responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(responses))
	}
}

func TestResponsesHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/responses", nil)
	rr := httptest.NewRecorder()
	server.responsesHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /responses, got %d", rr.Code)
	}
}

func TestStatsHandler_Success(t *testing.T) {
	server := newTestServer()

	// Seed responses
	server.st.AddResponse(models.Response{From: "+1", Body: "a", Time: 1})
	server.st.AddResponse(models.Response{From: "+1", Body: "bb", Time: 2})
	server.st.AddResponse(models.Response{From: "+2", Body: "ccc", Time: 3})

	req, _ := http.NewRequest("GET", "/stats", nil)
	rr := httptest.NewRecorder()
	server.statsHandler(rr, req)

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

	// Check responses per sender
	perSender := stats["responses_per_sender"].(map[string]interface{})
	if perSender["+1"].(float64) != 2 {
		t.Errorf("wrong count for +1: %v", perSender["+1"])
	}
	if perSender["+2"].(float64) != 1 {
		t.Errorf("wrong count for +2: %v", perSender["+2"])
	}

	// Check average length
	expectedAvg := float64(1+2+3) / 3.0 // "a", "bb", "ccc"
	if stats["avg_response_length"].(float64) != expectedAvg {
		t.Errorf("wrong avg_response_length: expected %v, got %v", expectedAvg, stats["avg_response_length"])
	}
}

func TestStatsHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("POST", "/stats", nil)
	rr := httptest.NewRecorder()
	server.statsHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /stats, got %d", rr.Code)
	}
}
