package genai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openai/openai-go"
)

// Test the debug logging functionality
func TestDebugLogging(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "genai_debug_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a mock client with debug mode enabled
	mockResp := openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: "Test response"}},
		},
	}

	client := &Client{
		chat:        &mockChatService{resp: mockResp},
		model:       "test-model",
		temperature: 0.7,
		maxTokens:   100,
		debugMode:   true,
		stateDir:    tempDir,
	}

	// Make an API call
	_, err = client.GeneratePromptWithContext(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("GeneratePromptWithContext failed: %v", err)
	}

	// Wait a moment for the file to be written
	time.Sleep(100 * time.Millisecond)

	// Check if debug directory was created
	debugDir := filepath.Join(tempDir, "debug")
	if _, err := os.Stat(debugDir); os.IsNotExist(err) {
		t.Fatalf("Debug directory was not created: %s", debugDir)
	}

	// Check if debug file was created
	files, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatalf("Failed to read debug directory: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("No debug files were created")
	}

	// Read and verify the debug file content
	debugFile := filepath.Join(debugDir, files[0].Name())
	content, err := os.ReadFile(debugFile)
	if err != nil {
		t.Fatalf("Failed to read debug file: %v", err)
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Fatalf("Failed to unmarshal debug log: %v", err)
	}

	// Verify required fields are present
	requiredFields := []string{"timestamp", "method", "model", "params", "response"}
	for _, field := range requiredFields {
		if _, exists := logEntry[field]; !exists {
			t.Errorf("Required field '%s' missing from debug log", field)
		}
	}

	// Verify method and model
	if logEntry["method"] != "GeneratePromptWithContext" {
		t.Errorf("Expected method 'GeneratePromptWithContext', got %v", logEntry["method"])
	}

	if logEntry["model"] != "test-model" {
		t.Errorf("Expected model 'test-model', got %v", logEntry["model"])
	}
}

// Test that debug logging is disabled when debug mode is false
func TestDebugLoggingDisabled(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "genai_debug_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a mock client with debug mode disabled
	mockResp := openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: "Test response"}},
		},
	}

	client := &Client{
		chat:        &mockChatService{resp: mockResp},
		model:       "test-model",
		temperature: 0.7,
		maxTokens:   100,
		debugMode:   false, // Debug mode disabled
		stateDir:    tempDir,
	}

	// Make an API call
	_, err = client.GeneratePromptWithContext(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("GeneratePromptWithContext failed: %v", err)
	}

	// Wait a moment
	time.Sleep(100 * time.Millisecond)

	// Check that no debug directory was created
	debugDir := filepath.Join(tempDir, "debug")
	if _, err := os.Stat(debugDir); !os.IsNotExist(err) {
		t.Errorf("Debug directory should not be created when debug mode is disabled")
	}
}
