package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// WhatsAppService implements Service using the Whatsmeow-based whatsapp client.
type WhatsAppService struct {
	client    whatsapp.WhatsAppSender
	receipts  chan models.Receipt
	responses chan models.Response
	done      chan struct{}
}

// NewWhatsAppService creates a new WhatsAppService wrapping the given WhatsAppSender.
func NewWhatsAppService(client whatsapp.WhatsAppSender) *WhatsAppService {
	return &WhatsAppService{
		client:    client,
		receipts:  make(chan models.Receipt, 100),
		responses: make(chan models.Response, 100),
		done:      make(chan struct{}),
	}
}

// Start begins background processing (e.g., event polling). Currently a no-op.
func (s *WhatsAppService) Start(ctx context.Context) error {
	slog.Debug("WhatsAppService Start invoked")
	// TODO: subscribe to WhatsApp events and feed receipts/responses channels.
	slog.Debug("WhatsAppService Start no-op implementation")
	return nil
}

// Stop stops background processing.
func (s *WhatsAppService) Stop() error {
	slog.Info("WhatsAppService Stop invoked")
	close(s.done)
	close(s.receipts)
	close(s.responses)
	slog.Info("WhatsAppService stopped and channels closed")
	return nil
}

// SendMessage sends a message and emits a sent receipt.
func (s *WhatsAppService) SendMessage(ctx context.Context, to string, body string) error {
	slog.Debug("WhatsAppService SendMessage invoked", "to", to, "body_length", len(body))
	err := s.client.SendMessage(ctx, to, body)
	if err != nil {
		slog.Error("WhatsAppService SendMessage error", "error", err, "to", to)
		return err
	}
	// Emit sent receipt
	s.receipts <- models.Receipt{To: to, Status: "sent", Time: time.Now().Unix()}
	slog.Info("WhatsAppService message sent and receipt emitted", "to", to)
	return nil
}

// Receipts returns a channel of receipt events.
func (s *WhatsAppService) Receipts() <-chan models.Receipt {
	return s.receipts
}

// Responses returns a channel of incoming response events.
func (s *WhatsAppService) Responses() <-chan models.Response {
	return s.responses
}
