// Package testutil provides common test utilities and helpers for PromptPipe tests.
package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/messaging"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/scheduler"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// TestingT defines the interface that testing types must implement.
// This allows both *testing.T and mock implementations to be used.
type TestingT interface {
	Helper()
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

// NewTestServer creates a test API server with in-memory dependencies.
// This centralizes the test server creation logic used across multiple test files.
func NewTestServer() *api.Server {
	msgService := messaging.NewWhatsAppService(whatsapp.NewMockClient())
	sched := scheduler.NewScheduler()
	st := store.NewInMemoryStore()
	timer := flow.NewSimpleTimer()

	return api.NewServer(msgService, sched, st, timer, "", nil)
}

// AssertHTTPStatus checks the HTTP status code and fails the test if it doesn't match.
func AssertHTTPStatus(t TestingT, expected, actual int, context string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected status %d, got %d", context, expected, actual)
	}
}

// AssertJSONResponse decodes JSON response and validates the status field.
func AssertJSONResponse(t TestingT, rr *httptest.ResponseRecorder, expectedStatus string) map[string]interface{} {
	t.Helper()
	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if status, ok := response["status"].(string); ok {
		if status != expectedStatus {
			t.Errorf("expected status '%s', got '%s'", expectedStatus, status)
		}
	} else {
		t.Error("response missing or invalid 'status' field")
	}

	return response
}

// CreateHTTPRequest creates an HTTP request with optional JSON body for testing.
func CreateHTTPRequest(t TestingT, method, url string, body interface{}) *http.Request {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("failed to create HTTP request: %v", err)
	}
	return req
}

// CreateJSONRequest creates an HTTP request with a JSON string body for testing.
// This is a convenience wrapper around CreateHTTPRequest for JSON string bodies.
func CreateJSONRequest(t TestingT, method, url, jsonBody string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(jsonBody)))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	return req
}

// AssertResponseCount validates the number of responses in store matches expected.
func AssertResponseCount(t TestingT, store store.Store, expected int, context string) {
	t.Helper()
	responses, err := store.GetResponses()
	if err != nil {
		t.Fatalf("%s: failed to get responses: %v", context, err)
	}
	if len(responses) != expected {
		t.Errorf("%s: expected %d responses, got %d", context, expected, len(responses))
	}
}

// SeedTestData adds sample data to the store for testing.
func SeedTestData(t TestingT, store store.Store) {
	t.Helper()

	// Add test receipts
	testReceipts := []models.Receipt{
		{To: "+123", Status: models.MessageStatusSent, Time: 1},
		{To: "+456", Status: models.MessageStatusDelivered, Time: 2},
	}

	for _, receipt := range testReceipts {
		if err := store.AddReceipt(receipt); err != nil {
			t.Fatalf("failed to add test receipt: %v", err)
		}
	}

	// Add test responses
	testResponses := []models.Response{
		{From: "+123", Body: "test response 1", Time: 10},
		{From: "+456", Body: "test response 2", Time: 20},
	}

	for _, response := range testResponses {
		if err := store.AddResponse(response); err != nil {
			t.Fatalf("failed to add test response: %v", err)
		}
	}
}

// AssertPromptEquals compares two Prompt structs for equality in tests.
func AssertPromptEquals(t TestingT, expected, actual models.Prompt, context string) {
	t.Helper()
	if actual.To != expected.To ||
		actual.Cron != expected.Cron ||
		actual.Type != expected.Type ||
		actual.State != expected.State ||
		actual.Body != expected.Body ||
		actual.SystemPrompt != expected.SystemPrompt ||
		actual.UserPrompt != expected.UserPrompt {
		t.Errorf("%s: prompts don't match\nexpected: %+v\nactual: %+v", context, expected, actual)
	}

	if len(actual.BranchOptions) != len(expected.BranchOptions) {
		t.Errorf("%s: branch options length mismatch: expected %d, got %d",
			context, len(expected.BranchOptions), len(actual.BranchOptions))
		return
	}

	for i, expectedOpt := range expected.BranchOptions {
		actualOpt := actual.BranchOptions[i]
		if actualOpt.Label != expectedOpt.Label || actualOpt.Body != expectedOpt.Body {
			t.Errorf("%s: branch option %d mismatch\nexpected: %+v\nactual: %+v",
				context, i, expectedOpt, actualOpt)
		}
	}
}

// MustMarshalJSON marshals an object to JSON and fails test on error.
func MustMarshalJSON(t TestingT, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return data
}

// MustUnmarshalJSON unmarshals JSON data into target and fails test on error.
func MustUnmarshalJSON(t TestingT, data []byte, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
}
