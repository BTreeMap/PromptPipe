package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/twiliowhatsapp"
)

// TestAPIIntegration tests the complete integration of response handlers with the API
func TestTwilioAPIIntegration_BranchPromptResponseFlow(t *testing.T) {
	// Setup mock messaging service
	mockClient := twiliowhatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	// Create response handler
	respHandler := NewResponseHandler(msgService, store.NewInMemoryStore(), false)

	// Create a branch prompt
	branchPrompt := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeBranch,
		Body: "What would you like to do?",
		BranchOptions: []models.BranchOption{
			{Label: "Continue", Body: "Let's continue with the next step"},
			{Label: "Stop", Body: "We'll stop here for now"},
		},
	}

	// Simulate the API flow: auto-register response handler
	registered := respHandler.AutoRegisterResponseHandler(branchPrompt)
	if !registered {
		t.Fatal("Response handler should have been registered for branch prompt")
	}

	// Verify handler is registered for canonicalized number
	if !respHandler.IsHookRegistered("1234567890") {
		t.Error("Response handler should be registered for canonicalized number")
	}

	// Simulate user response "1" (selecting first option)
	ctx := context.Background()
	userResponse := models.Response{
		From: "1234567890",
		Body: "1",
		Time: time.Now().Unix(),
	}

	// Process the response
	err := respHandler.ProcessResponse(ctx, userResponse)
	if err != nil {
		t.Fatalf("Failed to process user response: %v", err)
	}

	// Verify confirmation message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 confirmation message, got %d", len(mockClient.SentMessages))
	}

	confirmMsg := mockClient.SentMessages[0]
	if confirmMsg.To != "1234567890" {
		t.Errorf("Expected message to 1234567890, got %s", confirmMsg.To)
	}
	if !contains(confirmMsg.Body, "Continue") || !contains(confirmMsg.Body, "Let's continue") {
		t.Errorf("Expected confirmation with selected option content, got: %s", confirmMsg.Body)
	}
}

func TestTwilioAPIIntegration_GenAIPromptResponseFlow(t *testing.T) {
	// Setup mock messaging service
	mockClient := twiliowhatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	// Create response handler
	respHandler := NewResponseHandler(msgService, store.NewInMemoryStore(), false)

	// Create a GenAI prompt
	genaiPrompt := models.Prompt{
		To:           "+1234567890",
		Type:         models.PromptTypeGenAI,
		SystemPrompt: "You are a helpful assistant",
		UserPrompt:   "Ask the user how they're feeling",
	}

	// Simulate the API flow: auto-register response handler
	registered := respHandler.AutoRegisterResponseHandler(genaiPrompt)
	if !registered {
		t.Fatal("Response handler should have been registered for GenAI prompt")
	}

	// Simulate user response
	ctx := context.Background()
	userResponse := models.Response{
		From: "1234567890",
		Body: "I'm feeling great today! Thanks for asking.",
		Time: time.Now().Unix(),
	}

	// Process the response
	err := respHandler.ProcessResponse(ctx, userResponse)
	if err != nil {
		t.Fatalf("Failed to process user response: %v", err)
	}

	// Verify acknowledgment message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 acknowledgment message, got %d", len(mockClient.SentMessages))
	}

	ackMsg := mockClient.SentMessages[0]
	if ackMsg.To != "1234567890" {
		t.Errorf("Expected message to 1234567890, got %s", ackMsg.To)
	}
	if !contains(ackMsg.Body, "Thank") && !contains(ackMsg.Body, "thanks") {
		t.Errorf("Expected acknowledgment message, got: %s", ackMsg.Body)
	}
}

func TestTwilioAPIIntegration_StaticPromptNoAutoHandler(t *testing.T) {
	// Setup mock messaging service
	mockClient := twiliowhatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	// Create response handler
	respHandler := NewResponseHandler(msgService, store.NewInMemoryStore(), false)

	// Create a static prompt that doesn't expect responses
	staticPrompt := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeStatic,
		Body: "This is just an informational message. No response needed.",
	}

	// Simulate the API flow: try to auto-register response handler
	registered := respHandler.AutoRegisterResponseHandler(staticPrompt)
	if registered {
		t.Error("Response handler should NOT have been registered for non-interactive static prompt")
	}

	// Verify no handler is registered
	if respHandler.IsHookRegistered("1234567890") {
		t.Error("No response handler should be registered for non-interactive static prompt")
	}

	// Simulate user response anyway
	ctx := context.Background()
	userResponse := models.Response{
		From: "1234567890",
		Body: "I got your message",
		Time: time.Now().Unix(),
	}

	// Process the response - should use default handler
	err := respHandler.ProcessResponse(ctx, userResponse)
	if err != nil {
		t.Fatalf("Failed to process user response: %v", err)
	}

	// Verify default message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 default message, got %d", len(mockClient.SentMessages))
	}

	defaultMsg := mockClient.SentMessages[0]
	if defaultMsg.To != "1234567890" {
		t.Errorf("Expected message to 1234567890, got %s", defaultMsg.To)
	}
	if !contains(defaultMsg.Body, "recorded") {
		t.Errorf("Expected default recorded message, got: %s", defaultMsg.Body)
	}
}

func TestTwilioAPIIntegration_InteractiveStaticPromptWithHandler(t *testing.T) {
	// Setup mock messaging service
	mockClient := twiliowhatsapp.NewMockClient()
	msgService := NewWhatsAppService(mockClient)

	// Create response handler
	respHandler := NewResponseHandler(msgService, store.NewInMemoryStore(), false)

	// Create a static prompt that expects responses (has question)
	staticPrompt := models.Prompt{
		To:   "+1234567890",
		Type: models.PromptTypeStatic,
		Body: "How are you feeling today? Please reply with your answer.",
	}

	// Simulate the API flow: auto-register response handler
	registered := respHandler.AutoRegisterResponseHandler(staticPrompt)
	if !registered {
		t.Fatal("Response handler should have been registered for interactive static prompt")
	}

	// Verify handler is registered
	if !respHandler.IsHookRegistered("1234567890") {
		t.Error("Response handler should be registered for interactive static prompt")
	}

	// Simulate user response
	ctx := context.Background()
	userResponse := models.Response{
		From: "1234567890",
		Body: "I'm doing well, thanks!",
		Time: time.Now().Unix(),
	}

	// Process the response
	err := respHandler.ProcessResponse(ctx, userResponse)
	if err != nil {
		t.Fatalf("Failed to process user response: %v", err)
	}

	// Verify acknowledgment message was sent
	if len(mockClient.SentMessages) != 1 {
		t.Errorf("Expected 1 acknowledgment message, got %d", len(mockClient.SentMessages))
	}

	ackMsg := mockClient.SentMessages[0]
	if ackMsg.To != "1234567890" {
		t.Errorf("Expected message to 1234567890, got %s", ackMsg.To)
	}
	if !contains(ackMsg.Body, "Thanks") && !contains(ackMsg.Body, "recorded") {
		t.Errorf("Expected acknowledgment message, got: %s", ackMsg.Body)
	}
}

// // Helper function for case-insensitive string contains check
// func contains(s, substr string) bool {
// 	return len(s) >= len(substr) && (s == substr ||
// 		len(s) > len(substr) &&
// 			(indexOf(s, substr) >= 0))
// }

// func indexOf(s, substr string) int {
// 	for i := 0; i <= len(s)-len(substr); i++ {
// 		if s[i:i+len(substr)] == substr {
// 			return i
// 		}
// 	}
// 	return -1
// }
