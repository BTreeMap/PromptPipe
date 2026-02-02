// Package whatsapp wraps the Whatsmeow client for WhatsApp integration in PromptPipe.
//
// It provides methods for sending messages and handling WhatsApp events.
package whatsapp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Constants for WhatsApp client configuration
const (
	// DefaultSQLitePath is the default path for WhatsApp/whatsmeow SQLite database
	DefaultSQLitePath = "/var/lib/promptpipe/whatsmeow.db"
	// JIDSuffix is the WhatsApp JID suffix for regular users
	JIDSuffix = "s.whatsapp.net"
)

// Poll configuration constants
// These define the engagement poll sent with prompts and must match exactly
// between the sending (SendPromptButtons) and receiving (poll response handler) logic.
const (
	// PollQuestion is the question text for the engagement poll
	PollQuestion = "Did you do it?"

	// IntensityPollQuestion is the question text for the intensity adjustment poll
	IntensityPollQuestion = "How's the intensity?"
)

// PollOptions defines the available response options for the engagement poll.
// The order matters - keep consistent between sending and receiving.
var PollOptions = []string{"Done", "Next time"}

// IntensityPollOptions defines all possible intensity adjustment options
var IntensityPollOptions = map[string][]string{
	"low":    {"Keep current", "Increase"},
	"normal": {"Decrease", "Keep current", "Increase"},
	"high":   {"Decrease", "Keep current"},
}

// FormatPollResponse formats a poll response into the standardized "Q: [question] A: [answer]" format.
// This ensures consistent formatting across all parts of the codebase that handle poll responses.
func FormatPollResponse(question, answer string) string {
	return fmt.Sprintf("Q: %s A: %s", question, answer)
}

// GetSuccessPollResponse returns the formatted poll response string for a successful habit completion.
// Use this for detecting when a user has clicked the "Done" button.
func GetSuccessPollResponse() string {
	return FormatPollResponse(PollQuestion, PollOptions[0]) // "Done" is the first option
}

// ParseIntensityPollResponse parses an intensity poll response and returns the new intensity level.
// Returns empty string if the response is not a valid intensity adjustment response.
// Valid responses: "Decrease", "Keep current", "Increase"
func ParseIntensityPollResponse(response string, currentIntensity string) string {
	// Check if this is an intensity adjustment poll response
	if !strings.Contains(response, IntensityPollQuestion) {
		return ""
	}

	// Extract the answer part
	if strings.Contains(response, "A: Decrease") {
		switch currentIntensity {
		case "normal":
			return "low"
		case "high":
			return "normal"
		default:
			return currentIntensity // Can't decrease from low
		}
	}

	if strings.Contains(response, "A: Keep current") {
		return currentIntensity
	}

	if strings.Contains(response, "A: Increase") {
		switch currentIntensity {
		case "low":
			return "normal"
		case "normal":
			return "high"
		default:
			return currentIntensity // Can't increase from high
		}
	}

	return "" // Not a valid intensity response
}

// WhatsAppSender is an interface for sending WhatsApp messages (for production and testing)
type WhatsAppSender interface {
	SendMessage(ctx context.Context, to string, body string) error
	SendTypingIndicator(ctx context.Context, to string, typing bool) error
}

// Opts holds configuration options for the WhatsApp client.
// This focuses solely on WhatsApp/whatsmeow database configuration and login settings.
type Opts struct {
	DBDSN       string // WhatsApp/whatsmeow database connection string
	QRPath      string // path to write login QR code
	NumericCode bool   // use numeric login code instead of QR code
}

// Option defines a configuration option for the WhatsApp client.
type Option func(*Opts)

// WithDBDSN sets the WhatsApp/whatsmeow database connection string.
func WithDBDSN(dsn string) Option {
	return func(o *Opts) {
		o.DBDSN = dsn
	}
}

// WithQRCodeOutput instructs the WhatsApp client to write the login QR code to the specified path.
func WithQRCodeOutput(path string) Option {
	return func(o *Opts) {
		o.QRPath = path
	}
}

// WithNumericCode instructs the WhatsApp client to use numeric login code instead of QR code.
func WithNumericCode() Option {
	return func(o *Opts) {
		o.NumericCode = true
	}
}

// Client wraps the Whatsmeow client for modular use
type Client struct {
	waClient *whatsmeow.Client
}

// NewClient creates a new WhatsApp client, applying any provided options for customization.
// This handles WhatsApp/whatsmeow database configuration with proper validation and warnings.
func NewClient(opts ...Option) (*Client, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	slog.Debug("Client.NewClient: setting options", "DBDSN_set", cfg.DBDSN != "", "QRPath_set", cfg.QRPath != "", "NumericCode", cfg.NumericCode)

	// Determine database DSN
	dbDSN := cfg.DBDSN
	if dbDSN == "" {
		dbDSN = DefaultSQLitePath
		slog.Debug("No WhatsApp database DSN provided, using default SQLite path", "default_path", dbDSN)
	}

	// Auto-detect database driver based on DSN
	var dbDriver string
	if store.DetectDSNType(dbDSN) == "postgres" {
		dbDriver = "postgres"
		slog.Debug("WhatsApp client auto-detected PostgreSQL driver", "dsn_type", "postgresql")
	} else {
		dbDriver = "sqlite3"
		slog.Debug("WhatsApp client auto-detected SQLite driver", "dsn_type", "sqlite")

		// Check if SQLite DSN has foreign keys enabled (whatsmeow recommends this)
		if !strings.Contains(dbDSN, "_foreign_keys") && !strings.Contains(dbDSN, "foreign_keys") {
			slog.Warn("SQLite database for WhatsApp does not appear to have foreign keys enabled. "+
				"The whatsmeow library strongly recommends enabling foreign keys for data integrity. "+
				"Consider adding '?_foreign_keys=on' to your connection string.",
				"dsn_example", "file:"+dbDSN+"?_foreign_keys=on")
		}
	}

	slog.Debug("WhatsApp NewClient initializing DB store", "driver", dbDriver, "dsn_set", dbDSN != "")
	logger := waLog.Stdout("Database", "INFO", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, dbDriver, dbDSN, logger)
	if err != nil {
		slog.Error("Failed to initialize WhatsApp DB store", "error", err)
		return nil, fmt.Errorf("failed to initialize WhatsApp database store: %w", err)
	}
	slog.Debug("WhatsApp DB store initialized")

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		slog.Error("Failed to get first device from store", "error", err)
		return nil, fmt.Errorf("failed to get device from WhatsApp store: %w", err)
	}
	slog.Debug("WhatsApp device store retrieved")

	clientLog := waLog.Stdout("Client", "INFO", true)
	waClient := whatsmeow.NewClient(deviceStore, clientLog)

	if waClient.Store.ID == nil {
		slog.Info("WhatsApp login required; starting QR code flow")
		qrChan, _ := waClient.GetQRChannel(context.Background())
		err = waClient.Connect()
		if err != nil {
			slog.Error("Failed to connect to WhatsApp during login", "error", err)
			return nil, fmt.Errorf("failed to connect to WhatsApp during login: %w", err)
		}
		// Determine output writer for QR or code
		writer := io.Writer(os.Stdout)
		if cfg.QRPath != "" {
			f, ferr := os.Create(cfg.QRPath)
			if ferr != nil {
				slog.Error("Failed to create QR file", "error", ferr)
				return nil, fmt.Errorf("failed to create QR file: %w", ferr)
			}
			defer f.Close()
			writer = f
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				slog.Debug("WhatsApp login event code received", "code", evt.Code)
				if cfg.NumericCode {
					fmt.Fprintln(writer, evt.Code)
				} else {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, writer)
				}
			} else {
				slog.Debug("WhatsApp login event", "event", evt.Event)
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		slog.Debug("WhatsApp already logged in, connecting to server")
		if err := waClient.Connect(); err != nil {
			slog.Error("Failed to connect to WhatsApp server", "error", err)
			return nil, fmt.Errorf("failed to connect to WhatsApp server: %w", err)
		}
	}
	slog.Info("WhatsApp client connected successfully")
	return &Client{waClient: waClient}, nil
}

// SendMessage sends a WhatsApp message to the specified recipient.
// It performs basic validation and relies on the service layer for phone number validation.
func (c *Client) SendMessage(ctx context.Context, to string, body string) error {
	if c.waClient == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}
	if c.waClient.Store == nil {
		return fmt.Errorf("whatsapp client store not available")
	}
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if body == "" {
		return fmt.Errorf("message body cannot be empty")
	}

	// Note: Phone number validation and canonicalization should be handled by the service layer.
	// The recipient should already be validated and canonicalized when this method is called.

	jid, err := parseRecipientJID(to)
	if err != nil {
		return err
	}

	slog.Debug("Sending WhatsApp message", "to", jid.String(), "body_length", len(body))
	msg := &waE2E.Message{Conversation: &body}

	_, err = c.waClient.SendMessage(ctx, jid, msg)
	if err != nil {
		slog.Error("Failed to send WhatsApp message", "error", err, "to", to)
		return fmt.Errorf("failed to send message to %s: %w", to, err)
	}

	slog.Debug("WhatsApp message sent successfully", "to", to)
	return nil
}

// SendPromptButtons sends a prompt message followed by a poll for "Did you do it?" with two options.
// This method maintains the original interface but now sends a poll instead of deprecated button messages.
func (c *Client) SendPromptButtons(ctx context.Context, to string, body string) error {
	if c.waClient == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}
	if c.waClient.Store == nil {
		return fmt.Errorf("whatsapp client store not available")
	}
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("message body cannot be empty")
	}

	jid, err := parseRecipientJID(to)
	if err != nil {
		return err
	}

	slog.Debug("Sending WhatsApp prompt with poll", "to", jid.String(), "body_length", len(body))

	// First send the main prompt message
	mainMsg := &waE2E.Message{Conversation: &body}
	_, err = c.waClient.SendMessage(ctx, jid, mainMsg)
	if err != nil {
		slog.Error("Failed to send WhatsApp prompt message", "error", err, "to", to)
		return fmt.Errorf("failed to send prompt message to %s: %w", to, err)
	}

	// Then send a poll as a follow-up using Whatsmeow's built-in helper
	pollMsg := c.waClient.BuildPollCreation(
		PollQuestion,
		PollOptions,
		1, // single-select
	)

	_, err = c.waClient.SendMessage(ctx, jid, pollMsg)
	if err != nil {
		slog.Error("Failed to send WhatsApp poll", "error", err, "to", to)
		return fmt.Errorf("failed to send poll to %s: %w", to, err)
	}

	slog.Debug("WhatsApp prompt with poll sent successfully", "to", to)
	return nil
}

// SendIntensityAdjustmentPoll sends a poll asking the user to adjust their intervention intensity.
// The poll options are determined by the current intensity level (low/normal/high).
func (c *Client) SendIntensityAdjustmentPoll(ctx context.Context, to string, currentIntensity string) error {
	if c.waClient == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}
	if c.waClient.Store == nil {
		return fmt.Errorf("whatsapp client store not available")
	}
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}

	// Get the appropriate poll options for the current intensity
	options, exists := IntensityPollOptions[currentIntensity]
	if !exists {
		slog.Error("Invalid intensity level", "intensity", currentIntensity)
		return fmt.Errorf("invalid intensity level: %s", currentIntensity)
	}

	jid, err := parseRecipientJID(to)
	if err != nil {
		return err
	}

	slog.Debug("Sending intensity adjustment poll", "to", jid.String(), "currentIntensity", currentIntensity, "options", options)

	// Send poll using Whatsmeow's built-in helper
	pollMsg := c.waClient.BuildPollCreation(
		IntensityPollQuestion,
		options,
		1, // single-select
	)

	_, err = c.waClient.SendMessage(ctx, jid, pollMsg)
	if err != nil {
		slog.Error("Failed to send intensity adjustment poll", "error", err, "to", to)
		return fmt.Errorf("failed to send intensity poll to %s: %w", to, err)
	}

	slog.Debug("Intensity adjustment poll sent successfully", "to", to)
	return nil
}

// SendTypingIndicator updates the chat presence state (typing indicator) for a conversation.
func (c *Client) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	if c.waClient == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	jid, err := parseRecipientJID(to)
	if err != nil {
		return err
	}
	presence := types.ChatPresencePaused
	if typing {
		presence = types.ChatPresenceComposing
	}

	if err := c.waClient.SendChatPresence(jid, presence, types.ChatPresenceMediaText); err != nil {
		slog.Error("Failed to send WhatsApp typing indicator", "to", to, "error", err, "typing", typing)
		return fmt.Errorf("failed to send typing indicator: %w", err)
	}

	slog.Debug("WhatsApp typing indicator sent", "to", to, "typing", typing)
	return nil
}

func parseRecipientJID(to string) (types.JID, error) {
	if strings.Contains(to, "@") {
		jid, err := types.ParseJID(to)
		if err != nil {
			return types.JID{}, fmt.Errorf("invalid recipient JID %q: %w", to, err)
		}
		return jid.ToNonAD(), nil
	}

	return types.NewJID(to, JIDSuffix), nil
}

// GetClient returns the underlying whatsmeow client for event handling
func (c *Client) GetClient() *whatsmeow.Client {
	return c.waClient
}

// MockClient implements the same interface as Client but does nothing (for tests)
// In tests, use whatsapp.NewMockClient() instead of NewClient to avoid real WhatsApp connections.
// Update api_test.go to use MockClient for waClient.
type MockClient struct {
	SentMessages         []SentMessage
	PromptButtonMessages []SentMessage
	TypingEvents         []TypingEvent
}

// SentMessage represents a message sent via MockClient for testing
type SentMessage struct {
	To   string
	Body string
}

func NewMockClient() *MockClient {
	return &MockClient{
		SentMessages:         make([]SentMessage, 0),
		PromptButtonMessages: make([]SentMessage, 0),
		TypingEvents:         make([]TypingEvent, 0),
	}
}

func (m *MockClient) SendMessage(ctx context.Context, to string, body string) error {
	// Basic validation only - detailed validation should be done at service level
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if body == "" {
		return fmt.Errorf("message body cannot be empty")
	}

	// Track sent message for testing
	m.SentMessages = append(m.SentMessages, SentMessage{
		To:   to,
		Body: body,
	})

	return nil
}

// SendPromptButtons records prompt button messages for testing purposes.
// Note: This now sends a poll instead of buttons, but maintains the interface name.
func (m *MockClient) SendPromptButtons(ctx context.Context, to string, body string) error {
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if body == "" {
		return fmt.Errorf("message body cannot be empty")
	}

	m.PromptButtonMessages = append(m.PromptButtonMessages, SentMessage{To: to, Body: body})
	// Simulate sending main message
	if err := m.SendMessage(ctx, to, body); err != nil {
		return err
	}
	// Simulate sending poll (just record it, no separate tracking needed for mock)
	return m.SendMessage(ctx, to, PollQuestion)
}

// SendIntensityAdjustmentPoll records intensity adjustment poll messages for testing purposes.
func (m *MockClient) SendIntensityAdjustmentPoll(ctx context.Context, to string, currentIntensity string) error {
	if to == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if currentIntensity == "" {
		currentIntensity = "normal" // Default if not set
	}

	// Just send a simple message to track this - in real implementation, it would send a poll
	return m.SendMessage(ctx, to, IntensityPollQuestion)
}

// SendTypingIndicator records typing indicator state changes for testing.
func (m *MockClient) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	m.TypingEvents = append(m.TypingEvents, TypingEvent{To: to, Typing: typing})
	return nil
}

// TypingEvent captures mock typing indicator invocations for assertions.
type TypingEvent struct {
	To     string
	Typing bool
}
