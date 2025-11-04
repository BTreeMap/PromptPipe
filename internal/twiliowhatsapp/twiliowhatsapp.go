// Package twilio wraps the Twilio API for WhatsApp integration in PromptPipe.
package twiliowhatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

// TODO
type TwilioWhatsAppSender interface {
	SendMessage(ctx context.Context, to string, body string) error
	SendTypingIndicator(ctx context.Context, to string, typing bool) error
	SendPromptWithButtons(ctx context.Context, to string, body string) error
}

// Opts holds configuration options for the Twilio WhatsApp client.
// This focuses solely on Twilio API requirements
type Opts struct {
	AccountSID string
	AuthToken  string
	FromWhats  string
}

// Option defines a configuration option for the Twilio WhatsApp client.
type Option func(*Opts)

// TODO: add comments for these
func WithAccountSID(sid string) Option {
	return func(o *Opts) { o.AccountSID = sid }
}

func WithAuthToken(token string) Option {
	return func(o *Opts) { o.AuthToken = token }
}

func WithFromWhats(from string) Option {
	return func(o *Opts) { o.FromWhats = from }
}

// Client wraps Twilio REST API for WhatsApp
type Client struct {
	client    *twilio.RestClient
	fromWhats string // WhatsApp number in "whatsapp:+1234567890" format
}

func NewClient(opts ...Option) (*Client, error) {
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}
	// Fallback to environment variables if not provided via options
	if cfg.AccountSID == "" {
		cfg.AccountSID = os.Getenv("TWILIO_ACCOUNT_SID")
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = os.Getenv("TWILIO_AUTH_TOKEN")
	}
	if cfg.FromWhats == "" {
		cfg.FromWhats = os.Getenv("TWILIO_FROM_NUMBER")
	}
	// Debug: print out whether they were actually loaded
	slog.Debug("Twilio client config loaded",
		"AccountSID_set", cfg.AccountSID != "",
		"AuthToken_set", cfg.AuthToken != "",
		"FromWhats_set", cfg.FromWhats != "")

	if cfg.AccountSID == "" || cfg.AuthToken == "" {
		return nil, fmt.Errorf("account SID and auth token must be provided")
	}
	if cfg.FromWhats == "" {
		return nil, fmt.Errorf("fromWhats number must be provided")
	}

	client := twilio.NewRestClientWithParams(
		twilio.ClientParams{
			Username: cfg.AccountSID,
			Password: cfg.AuthToken,
		},
	)

	return &Client{
		client:    client,
		fromWhats: cfg.FromWhats,
	}, nil
}

//TODO: old version
// func NewClient(opts ...Option) (*Client, error) {
// 	var cfg Opts

// 	// Apply all options
// 	for _, opt := range opts {
// 		opt(&cfg)
// 	}

// 	// Create default Twilio REST client if not injected
// 	//TODO: redo error checking
// 	if cfg.Client == nil {
// 		if cfg.AccountSID == "" || cfg.AuthToken == "" {
// 			return nil, fmt.Errorf("account SID and auth token must be provided")
// 		}
// 		cfg.Client = twilio.NewRestClientWithParams(
// 			twilio.ClientParams{Username: cfg.AccountSID, Password: cfg.AuthToken},
// 		)
// 	}

// 	if cfg.FromWhats == "" {
// 		return nil, fmt.Errorf("fromWhats number must be provided")
// 	}

// 	return &Client{
// 		client:    cfg.Client,
// 		fromWhats: cfg.FromWhats,
// 	}, nil
// }

// SendMessage sends a WhatsApp message using Twilio API
func (c *Client) SendMessage(ctx context.Context, to string, body string) error {
	params := &twilioApi.CreateMessageParams{}
	params.SetTo("whatsapp:" + to)
	params.SetFrom(c.fromWhats)
	params.SetBody(body)

	_, err := c.client.Api.CreateMessage(params)
	if err != nil {
		slog.Error("Twilio SendMessage failed", "to", to, "error", err)
		return fmt.Errorf("failed to send message to %s: %w", to, err)
	}

	slog.Debug("Twilio message sent", "to", to)
	return nil
}

// SendPromptWithButtons simulates a poll by sending a text prompt (Twilio does not support WhatsApp buttons in Go SDK)
func (c *Client) SendPromptWithButtons(ctx context.Context, to string, body string) error {
	// First send the main message
	if err := c.SendMessage(ctx, to, body); err != nil {
		return err
	}

	// Then send a simulated poll
	pollText := fmt.Sprintf("Poll: Did you do it? Options: Done, Next time")
	if err := c.SendMessage(ctx, to, pollText); err != nil {
		return err
	}

	slog.Debug("Twilio prompt with poll sent", "to", to)
	return nil
}

// SendTypingIndicator does nothing since Twilio API does not support typing indicators
func (c *Client) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	slog.Debug("Twilio SendTypingIndicator ignored (unsupported)", "to", to, "typing", typing)
	return nil
}

type MockClient struct {
	SentMessages         []SentMessage
	PromptButtonMessages []SentMessage
	TypingEvents         []TypingEvent
}

type SentMessage struct {
	To   string
	Body string
}

type TypingEvent struct {
	To     string
	Typing bool
}

func NewMockClient() *MockClient {
	return &MockClient{
		SentMessages:         []SentMessage{},
		PromptButtonMessages: []SentMessage{},
		TypingEvents:         []TypingEvent{},
	}
}

func (m *MockClient) SendMessage(ctx context.Context, to string, body string) error {
	m.SentMessages = append(m.SentMessages, SentMessage{To: to, Body: body})
	return nil
}

func (m *MockClient) SendPromptWithButtons(ctx context.Context, to string, body string) error {
	m.PromptButtonMessages = append(m.PromptButtonMessages, SentMessage{To: to, Body: body})
	// Simulate sending main message
	_ = m.SendMessage(ctx, to, body)
	// Simulate sending poll
	_ = m.SendMessage(ctx, to, "Poll: Did you do it? Options: Done, Next time")
	return nil
}

func (m *MockClient) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	m.TypingEvents = append(m.TypingEvents, TypingEvent{To: to, Typing: typing})
	return nil
}
