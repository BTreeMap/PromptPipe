package messaging

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"go.mau.fi/whatsmeow/types/events"
)

// Constants for WhatsAppService configuration
const (
	// DefaultChannelBufferSize defines the default buffer size for receipt and response channels
	DefaultChannelBufferSize = 100
	// DefaultChannelTimeout defines the default timeout for non-blocking channel operations
	DefaultChannelTimeout = 1 * time.Second
)

// WhatsAppService implements Service using the Whatsmeow-based whatsapp client.
type WhatsAppService struct {
	client    whatsapp.WhatsAppSender
	waClient  *whatsapp.Client // Access to underlying client for event handling
	receipts  chan models.Receipt
	responses chan models.Response
	done      chan struct{}
}

// NewWhatsAppService creates a new WhatsAppService wrapping the given WhatsAppSender.
func NewWhatsAppService(client whatsapp.WhatsAppSender) *WhatsAppService {
	service := &WhatsAppService{
		client:    client,
		receipts:  make(chan models.Receipt, DefaultChannelBufferSize),
		responses: make(chan models.Response, DefaultChannelBufferSize),
		done:      make(chan struct{}),
	}

	// If the client is a full Client (not just an interface), store it for event handling
	if waClient, ok := client.(*whatsapp.Client); ok {
		service.waClient = waClient
		slog.Debug("WhatsAppService created with full client for event handling")
	} else {
		slog.Debug("WhatsAppService created with interface client (likely mock)")
	}

	return service
}

// Start begins background processing (e.g., event polling).
func (s *WhatsAppService) Start(ctx context.Context) error {
	slog.Debug("WhatsAppService Start invoked")

	if s.waClient != nil {
		slog.Debug("WhatsAppService starting event handler")
		// Start goroutine to handle WhatsApp events
		go s.handleEvents(ctx)
		slog.Debug("WhatsAppService event handler started")
	} else {
		slog.Debug("WhatsAppService no full client available, skipping event handling (likely mock)")
	}

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
	s.receipts <- models.Receipt{To: to, Status: models.StatusTypeSent, Time: time.Now().Unix()}
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

// handleEvents processes WhatsApp events and feeds them into the appropriate channels
func (s *WhatsAppService) handleEvents(ctx context.Context) {
	slog.Debug("WhatsAppService handleEvents starting")

	if s.waClient == nil || s.waClient.GetClient() == nil {
		slog.Error("WhatsAppService handleEvents: no client available")
		return
	}

	// Add event handler for messages
	s.waClient.GetClient().AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			s.handleIncomingMessage(v)
		case *events.Receipt:
			s.handleMessageReceipt(v)
		default:
			// Ignore other event types
			slog.Debug("WhatsAppService ignoring event type", "type", getEventType(v))
		}
	})

	slog.Debug("WhatsAppService event handler registered")

	// Keep handler running until context is cancelled
	<-ctx.Done()
	slog.Debug("WhatsAppService handleEvents stopping due to context cancellation")
}

// handleIncomingMessage processes incoming text messages from participants
func (s *WhatsAppService) handleIncomingMessage(evt *events.Message) {
	if evt.Message == nil {
		return
	}

	// Extract text content
	var messageText string
	if evt.Message.Conversation != nil {
		messageText = *evt.Message.Conversation
	} else if evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.Text != nil {
		messageText = *evt.Message.ExtendedTextMessage.Text
	} else {
		// Skip non-text messages (images, audio, etc.)
		slog.Debug("WhatsAppService ignoring non-text message", "from", evt.Info.Sender.String())
		return
	}

	// Convert JID to E.164 format (remove @s.whatsapp.net suffix)
	fromNumber := strings.TrimSuffix(evt.Info.Sender.User, "")
	if !strings.HasPrefix(fromNumber, "+") {
		fromNumber = "+" + fromNumber
	}

	response := models.Response{
		From: fromNumber,
		Body: messageText,
		Time: evt.Info.Timestamp.Unix(),
	}

	slog.Debug("WhatsAppService processing incoming message", "from", response.From, "body_length", len(response.Body))

	// Send to responses channel (non-blocking)
	select {
	case s.responses <- response:
		slog.Info("WhatsAppService incoming message forwarded", "from", response.From)
	case <-time.After(DefaultChannelTimeout):
		slog.Warn("WhatsAppService responses channel blocked, dropping message", "from", response.From, "timeout", DefaultChannelTimeout)
	}
}

// handleMessageReceipt processes delivery and read receipts
func (s *WhatsAppService) handleMessageReceipt(evt *events.Receipt) {
	// Convert JID to E.164 format
	toNumber := strings.TrimSuffix(evt.MessageSource.Sender.User, "")
	if !strings.HasPrefix(toNumber, "+") {
		toNumber = "+" + toNumber
	}

	var status models.StatusType
	switch evt.Type {
	case events.ReceiptTypeDelivered:
		status = models.StatusTypeDelivered
	case events.ReceiptTypeRead:
		status = models.StatusTypeRead
	case events.ReceiptTypeReadSelf:
		// Skip self-read receipts
		return
	default:
		slog.Debug("WhatsAppService ignoring receipt type", "type", evt.Type, "to", toNumber)
		return
	}

	receipt := models.Receipt{
		To:     toNumber,
		Status: status,
		Time:   evt.Timestamp.Unix(),
	}

	slog.Debug("WhatsAppService processing receipt", "to", receipt.To, "status", receipt.Status)

	// Send to receipts channel (non-blocking)
	select {
	case s.receipts <- receipt:
		slog.Info("WhatsAppService receipt forwarded", "to", receipt.To, "status", receipt.Status)
	case <-time.After(DefaultChannelTimeout):
		slog.Warn("WhatsAppService receipts channel blocked, dropping receipt", "to", receipt.To, "timeout", DefaultChannelTimeout)
	}
}

// getEventType returns a string representation of the event type for logging
func getEventType(evt interface{}) string {
	switch evt.(type) {
	case *events.Message:
		return "Message"
	case *events.Receipt:
		return "Receipt"
	case *events.Presence:
		return "Presence"
	case *events.Connected:
		return "Connected"
	case *events.Disconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}
