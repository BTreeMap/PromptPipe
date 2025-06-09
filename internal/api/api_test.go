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
	return NewServer(
		messaging.NewWhatsAppService(whatsapp.NewMockClient()),
		scheduler.NewScheduler(),
		store.NewInMemoryStore(),
		"",
		nil,
	)
}

// Test helper functions
func assertHTTPStatus(t *testing.T, expected, actual int, context string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected status %d, got %d", context, expected, actual)
	}
}

func assertJSONStatus(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus string) {
	t.Helper()
	var response map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if response["status"] != expectedStatus {
		t.Errorf("expected status '%s', got '%s'", expectedStatus, response["status"])
	}
}

func createJSONRequest(t *testing.T, method, url, jsonBody string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(jsonBody)))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	return req
}

func TestSendHandler_Success(t *testing.T) {
	server := newTestServer()

	req := createJSONRequest(t, "POST", "/send", `{"to":"+123","body":"hi"}`)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	assertHTTPStatus(t, http.StatusOK, rr.Code, "send handler success")
	assertJSONStatus(t, rr, "ok")
}

func TestSendHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/send", nil)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "send handler method not allowed")
}

func TestSendHandler_MissingRecipient(t *testing.T) {
	server := newTestServer()

	req := createJSONRequest(t, "POST", "/send", `{"body":"hi"}`)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	assertHTTPStatus(t, http.StatusBadRequest, rr.Code, "send handler missing recipient")
}

func TestScheduleHandler_Success(t *testing.T) {
	server := newTestServer()

	req := createJSONRequest(t, "POST", "/schedule", `{"to":"+123","cron":"* * * * *","body":"hi"}`)
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	assertHTTPStatus(t, http.StatusCreated, rr.Code, "schedule handler success")
	assertJSONStatus(t, rr, "scheduled")
}

func TestScheduleHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/schedule", nil)
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "schedule handler method not allowed")
}

func TestScheduleHandler_MissingRecipient(t *testing.T) {
	server := newTestServer()

	req := createJSONRequest(t, "POST", "/schedule", `{"cron":"* * * * *","body":"hi"}`)
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	assertHTTPStatus(t, http.StatusBadRequest, rr.Code, "schedule handler missing recipient")
}

func TestReceiptsHandler_Success(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/receipts", nil)
	rr := httptest.NewRecorder()
	server.receiptsHandler(rr, req)

	assertHTTPStatus(t, http.StatusOK, rr.Code, "receipts handler success")

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

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "receipts handler method not allowed")
}

func TestResponseHandler_Success(t *testing.T) {
	server := newTestServer()

	req := createJSONRequest(t, "POST", "/response", `{"from":"+123","body":"hello"}`)
	rr := httptest.NewRecorder()
	server.responseHandler(rr, req)

	assertHTTPStatus(t, http.StatusCreated, rr.Code, "response handler success")

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

	assertJSONStatus(t, rr, "recorded")
}

func TestResponseHandler_MethodNotAllowed(t *testing.T) {
	server := newTestServer()

	req, _ := http.NewRequest("GET", "/response", nil)
	rr := httptest.NewRecorder()
	server.responseHandler(rr, req)

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "response handler method not allowed")
}

func TestResponsesHandler_Success(t *testing.T) {
	server := newTestServer()

	// Seed one response
	server.st.AddResponse(models.Response{From: "+123", Body: "hi", Time: 1})

	req, _ := http.NewRequest("GET", "/responses", nil)
	rr := httptest.NewRecorder()
	server.responsesHandler(rr, req)

	assertHTTPStatus(t, http.StatusOK, rr.Code, "responses handler success")

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

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "responses handler method not allowed")
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

	assertHTTPStatus(t, http.StatusOK, rr.Code, "stats handler success")

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

	assertHTTPStatus(t, http.StatusMethodNotAllowed, rr.Code, "stats handler method not allowed")
}

// Test validation functionality
func TestSendHandler_ValidationErrors(t *testing.T) {
	server := newTestServer()

	tests := []struct {
		name     string
		jsonBody string
		wantCode int
	}{
		{
			name:     "empty recipient",
			jsonBody: `{"body":"test message"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "static prompt without body",
			jsonBody: `{"to":"+123","type":"static"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "genai prompt without system prompt",
			jsonBody: `{"to":"+123","type":"genai","user_prompt":"test"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "genai prompt without user prompt",
			jsonBody: `{"to":"+123","type":"genai","system_prompt":"test"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "branch prompt without options",
			jsonBody: `{"to":"+123","type":"branch"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "branch prompt with too few options",
			jsonBody: `{"to":"+123","type":"branch","branch_options":[{"label":"A","body":"Option A"}]}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createJSONRequest(t, "POST", "/send", tt.jsonBody)
			rr := httptest.NewRecorder()
			server.sendHandler(rr, req)
			assertHTTPStatus(t, tt.wantCode, rr.Code, tt.name)
		})
	}
}

func TestScheduleHandler_ValidationErrors(t *testing.T) {
	server := newTestServer()

	tests := []struct {
		name     string
		jsonBody string
		wantCode int
	}{
		{
			name:     "empty recipient",
			jsonBody: `{"body":"test message","cron":"* * * * *"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "static prompt without body",
			jsonBody: `{"to":"+123","type":"static","cron":"* * * * *"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "branch prompt with empty label",
			jsonBody: `{"to":"+123","type":"branch","cron":"* * * * *","branch_options":[{"label":"","body":"Option A"},{"label":"B","body":"Option B"}]}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createJSONRequest(t, "POST", "/schedule", tt.jsonBody)
			rr := httptest.NewRecorder()
			server.scheduleHandler(rr, req)
			assertHTTPStatus(t, tt.wantCode, rr.Code, tt.name)
		})
	}
}
