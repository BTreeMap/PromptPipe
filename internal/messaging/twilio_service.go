package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	//TODO: both of these need to be imported differently
	"github.com/BTreeMap/PromptPipe/internal/twiliowhatsapp"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// TwilioService implements the Service interface using Twilio API
type TwilioService struct {
	client    twiliowhatsapp.TwilioWhatsAppSender // Could be real Twilio client or MockClient
	receipts  chan models.Receipt
	responses chan models.Response
	done      chan struct{}
	mu        sync.RWMutex
	stopped   bool
}

// NewTwilioService creates a new TwilioService with a real Twilio client
func NewTwilioService(client twiliowhatsapp.TwilioWhatsAppSender) *TwilioService {
	service := &TwilioService{
		client:    client,
		receipts:  make(chan models.Receipt, DefaultChannelBufferSize),
		responses: make(chan models.Response, DefaultChannelBufferSize),
		done:      make(chan struct{}),
	}

	return service
}

// ValidateAndCanonicalizeRecipient validates and canonicalizes a WhatsApp phone number.
// It removes all non-numeric characters and validates the result has at least 6 digits.
func (s *TwilioService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
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
		slog.Debug("TwilioService canonicalized recipient", "original", recipient, "canonical", canonical)
	}

	return canonical, nil
}

// Start is a no-op for Twilio (no live client)
func (s *TwilioService) Start(ctx context.Context) error {
	return nil
}

// Stop closes channels and stops the service
func (s *TwilioService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil
	}
	s.stopped = true
	close(s.done)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(s.receipts)
		close(s.responses)
	}()

	return nil
}

// SendMessage sends a message via Twilio and emits a receipt
func (s *TwilioService) SendMessage(ctx context.Context, to string, body string) error {
	s.mu.RLock()
	if s.stopped {
		s.mu.RUnlock()
		return ErrServiceStopped
	}
	s.mu.RUnlock()

	canonicalTo, err := s.ValidateAndCanonicalizeRecipient(to)
	if err != nil {
		slog.Error("WhatsAppService SendIntensityAdjustmentPoll validation error", "error", err, "to", to)
		return err
	}

	err = s.client.SendMessage(ctx, canonicalTo, body)
	if err != nil {
		return err
	}

	s.safeEmitReceipt(models.Receipt{To: canonicalTo, Status: models.MessageStatusSent, Time: time.Now().Unix()})
	return nil
}

func (s *TwilioService) SendPromptWithButtons(ctx context.Context, to string, body string) error {
	s.mu.RLock()
	if s.stopped {
		s.mu.RUnlock()
		return ErrServiceStopped
	}
	s.mu.RUnlock()

	err := s.client.SendPromptWithButtons(ctx, to, body)
	if err != nil {
		return err
	}

	s.safeEmitReceipt(models.Receipt{To: to, Status: models.MessageStatusSent, Time: time.Now().Unix()})
	return nil
}

// SendTypingIndicator updates typing state (no-op in real Twilio)
func (s *TwilioService) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	s.mu.RLock()
	if s.stopped {
		s.mu.RUnlock()
		return ErrServiceStopped
	}
	s.mu.RUnlock()

	return s.client.SendTypingIndicator(ctx, to, typing)
}

// Receipts returns the channel for sent message receipts
func (s *TwilioService) Receipts() <-chan models.Receipt {
	return s.receipts
}

// Responses returns the channel for incoming messages (unused for Twilio)
func (s *TwilioService) Responses() <-chan models.Response {
	return s.responses
}

func (s *TwilioService) safeEmitReceipt(receipt models.Receipt) {
	s.mu.RLock()
	stopped := s.stopped
	s.mu.RUnlock()
	if stopped {
		return
	}

	select {
	case s.receipts <- receipt:
	case <-time.After(DefaultChannelTimeout):
	}
}

// TwilioWebhookHandler handles inbound Twilio webhook requests.
// It parses incoming messages and emits them as models.Response into the Responses() channel.
func (s *TwilioService) TwilioWebhookHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Twilio webhook received")

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse Twilio webhook form", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	from := r.FormValue("From")
	body := r.FormValue("Body")

	if from == "" || body == "" {
		slog.Warn("Twilio webhook missing fields", "from", from, "body", body)
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	slog.Info("Inbound WhatsApp message from Twilio", "from", from, "body", body)

	response := models.Response{
		From: from,
		Body: body,
		Time: time.Now().Unix(), // ✅ matches struct definition
	}

	s.safeEmitResponse(response)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// safeEmitResponse safely pushes responses into the responses channel.
func (s *TwilioService) safeEmitResponse(response models.Response) {
	s.mu.RLock()
	stopped := s.stopped
	s.mu.RUnlock()
	if stopped {
		slog.Warn("TwilioService dropping inbound response (service stopped)", "from", response.From)
		return
	}

	select {
	case s.responses <- response:
		slog.Debug("TwilioService emitted inbound response", "from", response.From)
	case <-time.After(DefaultChannelTimeout):
		slog.Warn("TwilioService responses channel blocked, dropping message", "from", response.From)
	}
}
