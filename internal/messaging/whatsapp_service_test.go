package messaging

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// Ensure WhatsAppService implements Service interface
func TestWhatsAppService_ImplementsService(t *testing.T) {
	var _ Service = (*WhatsAppService)(nil)
}

// Test SendMessage emits a sent receipt
func TestWhatsAppService_SendMessage_Receipt(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)
	ctx := context.Background()
	to, body := "+1234567890", "hello"
	expectedCanonicalTo := "1234567890" // The service canonicalizes the phone number
	if err := svc.SendMessage(ctx, to, body); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	select {
	case receipt := <-svc.Receipts():
		if receipt.To != expectedCanonicalTo {
			t.Errorf("expected receipt.To %s, got %s", expectedCanonicalTo, receipt.To)
		}
		if receipt.Status != models.MessageStatusSent {
			t.Errorf("expected receipt.Status %s, got %s", models.MessageStatusSent, receipt.Status)
		}
	default:
		t.Fatal("expected receipt, got none")
	}
}

func TestWhatsAppService_SendTypingIndicator(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)
	ctx := context.Background()
	to := "+1234567890"

	if err := svc.SendTypingIndicator(ctx, to, true); err != nil {
		t.Fatalf("SendTypingIndicator returned error: %v", err)
	}
	if len(mockClient.TypingEvents) != 1 {
		t.Fatalf("expected 1 typing event, got %d", len(mockClient.TypingEvents))
	}
	event := mockClient.TypingEvents[0]
	if event.To != "1234567890" {
		t.Errorf("expected canonical recipient 1234567890, got %s", event.To)
	}
	if !event.Typing {
		t.Errorf("expected typing state true, got false")
	}

	// Ensure stop event works too
	if err := svc.SendTypingIndicator(ctx, to, false); err != nil {
		t.Fatalf("SendTypingIndicator stop returned error: %v", err)
	}
	if len(mockClient.TypingEvents) != 2 {
		t.Fatalf("expected 2 typing events, got %d", len(mockClient.TypingEvents))
	}
	if mockClient.TypingEvents[1].Typing {
		t.Errorf("expected typing state false for stop")
	}
}

// Test Start and Stop do not error and close channels
func TestWhatsAppService_StartStop(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	// After Stop, Receipts and Responses channels should be closed
	// Receiving from closed channels yields zero value immediately
	receipt, ok := <-svc.Receipts()
	if ok {
		t.Errorf("expected receipts channel closed, got value %v", receipt)
	}
	response, ok := <-svc.Responses()
	if ok {
		t.Errorf("expected responses channel closed, got value %v", response)
	}
}

func TestWhatsAppService_ValidateAndCanonicalizeRecipient(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "Valid phone number",
			input:       "1234567890",
			expected:    "1234567890",
			expectError: false,
		},
		{
			name:        "Valid international number",
			input:       "+1-234-567-8900",
			expected:    "12345678900",
			expectError: false,
		},
		{
			name:        "Phone number with formatting",
			input:       "(123) 456-7890",
			expected:    "1234567890",
			expectError: false,
		},
		{
			name:        "Phone number with dots and spaces",
			input:       "123.456 7890",
			expected:    "1234567890",
			expectError: false,
		},
		{
			name:        "Empty phone number",
			input:       "",
			expectError: true,
			errorSubstr: "recipient cannot be empty",
		},
		{
			name:        "Phone number with no digits",
			input:       "abc-def-ghij",
			expectError: true,
			errorSubstr: "no digits found",
		},
		{
			name:        "Phone number too short (5 digits)",
			input:       "12345",
			expectError: true,
			errorSubstr: "too short",
		},
		{
			name:        "Phone number too short with formatting",
			input:       "123-45",
			expectError: true,
			errorSubstr: "too short",
		},
		{
			name:        "Minimum valid length (6 digits)",
			input:       "123456",
			expected:    "123456",
			expectError: false,
		},
		{
			name:        "Minimum valid length with formatting",
			input:       "123-456",
			expected:    "123456",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.ValidateAndCanonicalizeRecipient(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input %q, but got nil", tt.input)
					return
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Expected error to contain %q, but got: %s", tt.errorSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input %q, but got: %s", tt.input, err.Error())
					return
				}
				if result != tt.expected {
					t.Errorf("Expected result %q for input %q, but got: %q", tt.expected, tt.input, result)
				}
			}
		})
	}
}
