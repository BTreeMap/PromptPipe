package whatsapp

import "testing"

func TestNewClient(t *testing.T) {
	_, err := NewClient()
	if err != nil {
		t.Error("Failed to create WhatsApp client")
	}
}
