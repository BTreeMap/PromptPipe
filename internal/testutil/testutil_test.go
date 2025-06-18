package testutil

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestNewTestServer(t *testing.T) {
	server := NewTestServer()
	if server == nil {
		t.Fatal("NewTestServer returned nil")
	}

	// Test that the server has all required dependencies by verifying it's not nil
	// We can't directly access private fields, so this basic check is sufficient
	if server == nil {
		t.Error("Expected server to be created, got nil")
	}
}

func TestAssertHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		expected   int
		actual     int
		context    string
		shouldFail bool
	}{
		{
			name:       "matching status codes",
			expected:   200,
			actual:     200,
			context:    "test context",
			shouldFail: false,
		},
		{
			name:       "different status codes",
			expected:   200,
			actual:     404,
			context:    "test context",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock testing.T to capture failures
			mockT := &mockTestingT{}

			AssertHTTPStatus(mockT, tt.expected, tt.actual, tt.context)

			if tt.shouldFail && !mockT.failed {
				t.Error("Expected test to fail but it passed")
			}
			if !tt.shouldFail && mockT.failed {
				t.Error("Expected test to pass but it failed")
			}
		})
	}
}

func TestAssertJSONResponse(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		expectedStatus string
		shouldFail     bool
	}{
		{
			name:           "valid JSON with matching status",
			jsonBody:       `{"status":"ok","data":"test"}`,
			expectedStatus: "ok",
			shouldFail:     false,
		},
		{
			name:           "valid JSON with different status",
			jsonBody:       `{"status":"error","data":"test"}`,
			expectedStatus: "ok",
			shouldFail:     true,
		},
		{
			name:           "invalid JSON",
			jsonBody:       `{"status":}`,
			expectedStatus: "ok",
			shouldFail:     true,
		},
		{
			name:           "missing status field",
			jsonBody:       `{"data":"test"}`,
			expectedStatus: "ok",
			shouldFail:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockT := &mockTestingT{}
			rr := httptest.NewRecorder()
			rr.Body.WriteString(tt.jsonBody)

			var response map[string]interface{}

			// Handle potential panic from Fatalf calls
			defer func() {
				if r := recover(); r != nil {
					// Expected for invalid JSON cases
					if !tt.shouldFail {
						t.Errorf("Unexpected panic: %v", r)
					}
				}
			}()

			response = AssertJSONResponse(mockT, rr, tt.expectedStatus)

			if tt.shouldFail && !mockT.failed {
				t.Error("Expected test to fail but it passed")
			}
			if !tt.shouldFail && mockT.failed {
				t.Errorf("Expected test to pass but it failed: %s", mockT.errorMsg)
			}
			if !tt.shouldFail && response == nil {
				t.Error("Expected response map to be returned")
			}
		})
	}
}

func TestCreateHTTPRequest(t *testing.T) {
	tests := []struct {
		name   string
		method string
		url    string
		body   interface{}
	}{
		{
			name:   "GET request with no body",
			method: "GET",
			url:    "/test",
			body:   nil,
		},
		{
			name:   "POST request with JSON body",
			method: "POST",
			url:    "/test",
			body:   map[string]string{"key": "value"},
		},
		{
			name:   "PUT request with struct body",
			method: "PUT",
			url:    "/test",
			body:   models.Prompt{To: "+1234567890", Body: "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := CreateHTTPRequest(t, tt.method, tt.url, tt.body)

			if req == nil {
				t.Fatal("Expected request to be created, got nil")
			}
			if req.Method != tt.method {
				t.Errorf("Expected method %s, got %s", tt.method, req.Method)
			}
			if req.URL.Path != tt.url {
				t.Errorf("Expected URL %s, got %s", tt.url, req.URL.Path)
			}
		})
	}
}

func TestCreateJSONRequest(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		url      string
		jsonBody string
	}{
		{
			name:     "GET request with empty body",
			method:   "GET",
			url:      "/test",
			jsonBody: "",
		},
		{
			name:     "POST request with JSON body",
			method:   "POST",
			url:      "/test",
			jsonBody: `{"key":"value"}`,
		},
		{
			name:     "PUT request with complex JSON",
			method:   "PUT",
			url:      "/test",
			jsonBody: `{"to":"+1234567890","body":"test message","type":"static"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := CreateJSONRequest(t, tt.method, tt.url, tt.jsonBody)

			if req == nil {
				t.Fatal("Expected request to be created, got nil")
			}
			if req.Method != tt.method {
				t.Errorf("Expected method %s, got %s", tt.method, req.Method)
			}
			if req.URL.Path != tt.url {
				t.Errorf("Expected URL %s, got %s", tt.url, req.URL.Path)
			}
		})
	}
}

func TestAssertResponseCount(t *testing.T) {
	st := store.NewInMemoryStore()

	// Test with empty store
	mockT := &mockTestingT{}
	AssertResponseCount(mockT, st, 0, "empty store")
	if mockT.failed {
		t.Errorf("Expected test to pass for empty store, but got: %s", mockT.errorMsg)
	}

	// Add a response and test count
	testResponse := models.Response{From: "+1234567890", Body: "test", Time: 123}
	if err := st.AddResponse(testResponse); err != nil {
		t.Fatalf("Failed to add test response: %v", err)
	}

	mockT = &mockTestingT{}
	AssertResponseCount(mockT, st, 1, "one response")
	if mockT.failed {
		t.Errorf("Expected test to pass for one response, but got: %s", mockT.errorMsg)
	}

	// Test with wrong expected count
	mockT = &mockTestingT{}
	AssertResponseCount(mockT, st, 2, "wrong count")
	if !mockT.failed {
		t.Error("Expected test to fail for wrong count")
	}
}

func TestSeedTestData(t *testing.T) {
	st := store.NewInMemoryStore()

	SeedTestData(t, st)

	// Verify receipts were added
	receipts, err := st.GetReceipts()
	if err != nil {
		t.Fatalf("Failed to get receipts: %v", err)
	}
	if len(receipts) != 2 {
		t.Errorf("Expected 2 receipts, got %d", len(receipts))
	}

	// Verify responses were added
	responses, err := st.GetResponses()
	if err != nil {
		t.Fatalf("Failed to get responses: %v", err)
	}
	if len(responses) != 2 {
		t.Errorf("Expected 2 responses, got %d", len(responses))
	}
}

func TestAssertPromptEquals(t *testing.T) {
	prompt1 := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeStatic,
		Body: "test message",
		BranchOptions: []models.BranchOption{
			{Label: "A", Body: "Option A"},
			{Label: "B", Body: "Option B"},
		},
	}

	prompt2 := prompt1 // Same content
	prompt3 := models.Prompt{
		To:   "+456", // Different recipient
		Type: models.PromptTypeStatic,
		Body: "test message",
	}

	// Test equal prompts
	mockT := &mockTestingT{}
	AssertPromptEquals(mockT, prompt1, prompt2, "equal prompts")
	if mockT.failed {
		t.Errorf("Expected equal prompts test to pass, but got: %s", mockT.errorMsg)
	}

	// Test different prompts
	mockT = &mockTestingT{}
	AssertPromptEquals(mockT, prompt1, prompt3, "different prompts")
	if !mockT.failed {
		t.Error("Expected different prompts test to fail")
	}
}

func TestMustMarshalJSON(t *testing.T) {
	testData := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	result := MustMarshalJSON(t, testData)
	if result == nil {
		t.Error("Expected JSON data to be returned")
	}

	// Test with valid data
	if len(result) == 0 {
		t.Error("Expected non-empty JSON data")
	}
}

func TestMustUnmarshalJSON(t *testing.T) {
	jsonData := []byte(`{"key":"value","number":123}`)
	var target map[string]interface{}

	MustUnmarshalJSON(t, jsonData, &target)

	if target["key"] != "value" {
		t.Errorf("Expected key to be 'value', got %v", target["key"])
	}
	if target["number"].(float64) != 123 {
		t.Errorf("Expected number to be 123, got %v", target["number"])
	}
}

// mockTestingT implements a subset of testing.T for testing our test helpers
type mockTestingT struct {
	failed   bool
	errorMsg string
	helper   bool
}

func (m *mockTestingT) Helper() {
	m.helper = true
}

func (m *mockTestingT) Errorf(format string, args ...interface{}) {
	m.failed = true
	if len(args) > 0 {
		m.errorMsg = fmt.Sprintf(format, args...)
	} else {
		m.errorMsg = format
	}
}

func (m *mockTestingT) Error(args ...interface{}) {
	m.failed = true
	if len(args) > 0 {
		m.errorMsg = fmt.Sprintf("%v", args[0])
	}
}

func (m *mockTestingT) Fatalf(format string, args ...interface{}) {
	m.failed = true
	if len(args) > 0 {
		m.errorMsg = fmt.Sprintf(format, args...)
	} else {
		m.errorMsg = format
	}
	panic("test failed") // Simulate fatal error
}

func (m *mockTestingT) Fatal(args ...interface{}) {
	m.failed = true
	if len(args) > 0 {
		m.errorMsg = fmt.Sprintf("%v", args[0])
	}
	panic("test failed") // Simulate fatal error
}
