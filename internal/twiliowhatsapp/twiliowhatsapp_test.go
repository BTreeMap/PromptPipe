package twiliowhatsapp

import (
	"context"
	"testing"
)

func TestMockClient_SendMessage(t *testing.T) {
	ctx := context.Background()
	mock := NewMockClient()

	err := mock.SendMessage(ctx, "12345", "Hello Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.SentMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.SentMessages))
	}

	if mock.SentMessages[0].Body != "Hello Test" {
		t.Errorf("expected body %q, got %q", "Hello Test", mock.SentMessages[0].Body)
	}
}
