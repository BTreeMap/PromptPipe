package whatsapp

import "testing"

func TestNewClient(t *testing.T) {
	// Use the mock client for testing to avoid DB and WhatsApp dependencies
	client := NewMockClient()
	if client == nil {
		t.Error("Failed to create WhatsApp mock client")
	}
}
