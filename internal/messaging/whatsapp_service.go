package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Constants for WhatsAppService configuration
const (
	// DefaultChannelBufferSize defines the default buffer size for receipt and response channels
	DefaultChannelBufferSize = 100
	// DefaultChannelTimeout defines the default timeout for non-blocking channel operations
	DefaultChannelTimeout = 1 * time.Second
)

// Error variables for better error handling
var (
	ErrServiceStopped = errors.New("messaging service has been stopped")
)

// phoneNumberRegex is a compiled regex for extracting only numeric characters from phone numbers
var phoneNumberRegex = regexp.MustCompile(`[^0-9]`)

// WhatsAppService implements Service using the Whatsmeow-based whatsapp client.
type WhatsAppService struct {
	client    whatsapp.WhatsAppSender
	waClient  *whatsapp.Client // Access to underlying client for event handling
	receipts  chan models.Receipt
	responses chan models.Response
	done      chan struct{}
	mu        sync.RWMutex
	stopped   bool
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

// ValidateAndCanonicalizeRecipient validates and canonicalizes a WhatsApp phone number.
// It removes all non-numeric characters and validates the result has at least 6 digits.
func (s *WhatsAppService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("recipient cannot be empty")
	}

	// Canonicalize by removing all non-numeric characters
	canonical := phoneNumberRegex.ReplaceAllString(recipient, "")
	wasModified := recipient != canonical
	
	// Validate canonicalized phone number
	if canonical == "" {
		return "", fmt.Errorf("invalid phone number: no digits found in recipient %q", recipient)
	}
	if len(canonical) < 6 {
		return "", fmt.Errorf("invalid phone number: %q is too short (minimum 6 digits required)", canonical)
	}

	// Log if canonicalization modified the recipient
	if wasModified {
		slog.Debug("WhatsAppService canonicalized recipient", "original", recipient, "canonical", canonical)
	}

	return canonical, nil
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		slog.Debug("WhatsAppService already stopped")
		return nil
	}

	slog.Info("WhatsAppService Stop invoked")
	s.stopped = true
	close(s.done)

	// Close channels after a brief delay to allow goroutines to finish
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(s.receipts)
		close(s.responses)
		slog.Info("WhatsAppService stopped and channels closed")
	}()

	return nil
}

// SendMessage sends a message and emits a sent receipt.
func (s *WhatsAppService) SendMessage(ctx context.Context, to string, body string) error {
	s.mu.RLock()
	if s.stopped {
		s.mu.RUnlock()
		return ErrServiceStopped
	}
	s.mu.RUnlock()

	// Validate and canonicalize recipient before sending
	canonicalTo, err := s.ValidateAndCanonicalizeRecipient(to)
	if err != nil {
		slog.Error("WhatsAppService SendMessage validation error", "error", err, "to", to)
		return err
	}

	slog.Debug("WhatsAppService SendMessage invoked", "to", canonicalTo, "body_length", len(body))
	err = s.client.SendMessage(ctx, canonicalTo, body)
	if err != nil {
		slog.Error("WhatsAppService SendMessage error", "error", err, "to", canonicalTo)
		return err
	}

	// Emit sent receipt (with safety check)
	s.safeEmitReceipt(models.Receipt{To: canonicalTo, Status: models.MessageStatusSent, Time: time.Now().Unix()})
	slog.Info("WhatsAppService message sent and receipt emitted", "to", canonicalTo)
	return nil
}

// safeEmitReceipt safely emits a receipt to the receipts channel, handling the case where the service is stopped
func (s *WhatsAppService) safeEmitReceipt(receipt models.Receipt) {
	s.mu.RLock()
	stopped := s.stopped
	s.mu.RUnlock()

	if stopped {
		slog.Debug("WhatsAppService receipt dropped, service stopped", "to", receipt.To, "status", receipt.Status)
		return
	}

	select {
	case s.receipts <- receipt:
		// Receipt sent successfully
	case <-time.After(DefaultChannelTimeout):
		slog.Warn("WhatsAppService receipts channel blocked, dropping receipt", "to", receipt.To, "timeout", DefaultChannelTimeout)
	}
}

// Receipts returns a channel of receipt events.
func (s *WhatsAppService) Receipts() <-chan models.Receipt {
	return s.receipts
}

// safeEmitResponse safely emits a response to the responses channel, handling the case where the service is stopped
func (s *WhatsAppService) safeEmitResponse(response models.Response) {
	s.mu.RLock()
	stopped := s.stopped
	s.mu.RUnlock()

	if stopped {
		slog.Debug("WhatsAppService response dropped, service stopped", "from", response.From)
		return
	}

	select {
	case s.responses <- response:
		slog.Info("WhatsAppService incoming message forwarded", "from", response.From)
	case <-time.After(DefaultChannelTimeout):
		slog.Warn("WhatsAppService responses channel blocked, dropping message", "from", response.From, "timeout", DefaultChannelTimeout)
	}
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
	s.safeEmitResponse(response)
}

// handleMessageReceipt processes delivery and read receipts
func (s *WhatsAppService) handleMessageReceipt(evt *events.Receipt) {
	// Convert JID to E.164 format
	toNumber := strings.TrimSuffix(evt.MessageSource.Sender.User, "")
	if !strings.HasPrefix(toNumber, "+") {
		toNumber = "+" + toNumber
	}

	var status models.MessageStatus
	switch evt.Type {
	case types.ReceiptTypeDelivered:
		status = models.MessageStatusDelivered
	case types.ReceiptTypeRead:
		status = models.MessageStatusRead
	case types.ReceiptTypeReadSelf:
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
	s.safeEmitReceipt(receipt)
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
