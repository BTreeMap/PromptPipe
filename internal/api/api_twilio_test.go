package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/twiliowhatsapp"
)

// newTestServerTwilio creates a Server instance using the Twilio mock client.
func newTestServerTwilio() *Server {
	os.Setenv("USE_TWILIO", "true")

	defaultSchedule := &models.Schedule{}
	twilioClient := twiliowhatsapp.NewMockClient() // mock, not the live API
	twilioService := messaging.NewTwilioService(twilioClient)

	return NewServer(
		twilioService,
		store.NewInMemoryStore(),
		flow.NewSimpleTimer(),
		defaultSchedule,
		nil,
	)
}

func TestTwilioSendHandler_Success(t *testing.T) {
	server := newTestServerTwilio()

	req := createJSONRequest(t, "POST", "/send", `{"to":"+15551234567","body":"Hi from Twilio!"}`)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	assertHTTPStatus(t, http.StatusOK, rr.Code, "Twilio send handler success")
	assertJSONStatus(t, rr, "ok")
}

func TestTwilioSendHandler_BadRequest(t *testing.T) {
	server := newTestServerTwilio()

	req := createJSONRequest(t, "POST", "/send", `{"body":""}`)
	rr := httptest.NewRecorder()
	server.sendHandler(rr, req)

	assertHTTPStatus(t, http.StatusBadRequest, rr.Code, "Twilio send missing recipient")
	assertJSONStatus(t, rr, "error")
}

func TestTwilioScheduleHandler_Success(t *testing.T) {
	server := newTestServerTwilio()

	req := createJSONRequest(t, "POST", "/schedule", `{"to":"+15551234567","cron":"daily","body":"Scheduled Twilio message"}`)
	rr := httptest.NewRecorder()
	server.scheduleHandler(rr, req)

	assertHTTPStatus(t, http.StatusCreated, rr.Code, "Twilio schedule handler success")
	assertJSONStatus(t, rr, "ok")
}

func TestTwilioResponseHandler_Success(t *testing.T) {
	server := newTestServerTwilio()

	req := createJSONRequest(t, "POST", "/response", `{"from":"+15551234567","body":"Reply from Twilio user"}`)
	rr := httptest.NewRecorder()
	server.responseHandler(rr, req)

	assertHTTPStatus(t, http.StatusCreated, rr.Code, "Twilio response handler success")
	assertJSONStatus(t, rr, "ok")

	responses, err := server.st.GetResponses()
	if err != nil {
		t.Fatalf("failed to get responses: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if responses[0].From != "+15551234567" || responses[0].Body != "Reply from Twilio user" {
		t.Errorf("stored response mismatch: %+v", responses[0])
	}
}

func TestTwilioStatsHandler_Success(t *testing.T) {
	server := newTestServerTwilio()
	server.st.AddResponse(models.Response{From: "+1", Body: "hi", Time: 1})
	server.st.AddResponse(models.Response{From: "+1", Body: "hey", Time: 2})
	server.st.AddResponse(models.Response{From: "+2", Body: "yo", Time: 3})

	req, _ := http.NewRequest("GET", "/stats", nil)
	rr := httptest.NewRecorder()
	server.statsHandler(rr, req)

	assertHTTPStatus(t, http.StatusOK, rr.Code, "Twilio stats handler success")

	var response models.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status ok, got %s", response.Status)
	}

	stats := response.Result.(map[string]interface{})
	if stats["total_responses"].(float64) != 3 {
		t.Errorf("expected 3 total responses, got %v", stats["total_responses"])
	}
}
