package messaging

import (
	"context"
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
	// TODO: subscribe to WhatsApp events and feed receipts/responses channels.
	return nil
}

// Stop stops background processing.
func (s *WhatsAppService) Stop() error {
	close(s.done)
	close(s.receipts)
	close(s.responses)
	return nil
}

// SendMessage sends a message and emits a sent receipt.
func (s *WhatsAppService) SendMessage(ctx context.Context, to string, body string) error {
	err := s.client.SendMessage(ctx, to, body)
	if err != nil {
		return err
	}
	// Emit sent receipt
	s.receipts <- models.Receipt{To: to, Status: "sent", Time: time.Now().Unix()}
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
