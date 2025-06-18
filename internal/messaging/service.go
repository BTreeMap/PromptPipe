package messaging

import (
	"context"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Service defines a pluggable message delivery abstraction.
// It supports sending messages, and provides channels for receipt and response events.
type Service interface {
	// ValidateAndCanonicalizeRecipient validates and canonicalizes a recipient identifier.
	// Returns the canonicalized recipient and an error if validation fails.
	// This allows each service to implement its own recipient validation rules.
	ValidateAndCanonicalizeRecipient(recipient string) (string, error)

	// SendMessage sends a message to a recipient.
	SendMessage(ctx context.Context, to string, body string) error

	// Start begins any background processing (e.g., polling for events).
	Start(ctx context.Context) error

	// Stop stops background processing and cleans up resources.
	Stop() error

	// Receipts returns a channel of receipt events (sent, delivered, read).
	Receipts() <-chan models.Receipt

	// Responses returns a channel of incoming participant responses.
	Responses() <-chan models.Response
}
