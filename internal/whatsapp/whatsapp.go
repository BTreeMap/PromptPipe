// Package whatsapp wraps the Whatsmeow client for WhatsApp integration in PromptPipe.
//
// It provides methods for sending messages and handling WhatsApp events.
package whatsapp

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// WhatsAppSender is an interface for sending WhatsApp messages (for production and testing)
type WhatsAppSender interface {
	SendMessage(ctx context.Context, to string, body string) error
}

// Opts holds configuration options for the WhatsApp client, including database and login settings.
// Database driver and DSN can be overridden via command-line options or environment variables.
type Opts struct {
	DBDriver    string // overrides WHATSAPP_DB_DRIVER
	DBDSN       string // overrides WHATSAPP_DB_DSN
	QRPath      string // path to write login QR code
	NumericCode bool   // use numeric login code instead of QR code
}

// Option defines a configuration option for the WhatsApp client.
type Option func(*Opts)

// WithDBDriver overrides the database driver used by the WhatsApp client.
func WithDBDriver(driver string) Option {
	return func(o *Opts) {
		o.DBDriver = driver
	}
}

// WithDBDSN overrides the DSN used by the WhatsApp client.
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
func NewClient(opts ...Option) (*Client, error) {
	// Apply options
	var cfg Opts
	for _, opt := range opts {
		opt(&cfg)
	}

	// Determine database driver and DSN based on options or defaults
	dbDriver := cfg.DBDriver
	if dbDriver == "" {
		dbDriver = "postgres"
	}
	dbDSN := cfg.DBDSN
	if dbDSN == "" {
		dbDSN = "postgres://postgres:postgres@localhost:5432/whatsapp?sslmode=disable"
	}
	logger := waLog.Stdout("Database", "INFO", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, dbDriver, dbDSN, logger)
	if err != nil {
		return nil, err
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, err
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	waClient := whatsmeow.NewClient(deviceStore, clientLog)
	if waClient.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := waClient.GetQRChannel(context.Background())
		err = waClient.Connect()
		if err != nil {
			return nil, err
		}
		// Determine output writer for QR or code
		writer := io.Writer(os.Stdout)
		if cfg.QRPath != "" {
			f, ferr := os.Create(cfg.QRPath)
			if ferr != nil {
				return nil, fmt.Errorf("failed to create QR file: %w", ferr)
			}
			defer f.Close()
			writer = f
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				if cfg.NumericCode {
					fmt.Fprintln(writer, evt.Code)
				} else {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, writer)
				}
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	}
	// Connect to WhatsApp server
	if err := waClient.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to WhatsApp: %w", err)
	}
	return &Client{waClient: waClient}, nil
}

func (c *Client) SendMessage(ctx context.Context, to string, body string) error {
	if c.waClient == nil || c.waClient.Store == nil {
		return fmt.Errorf("whatsapp client not initialized")
	}
	jid := types.NewJID(to, "s.whatsapp.net")
	msg := &waE2E.Message{Conversation: &body}
	_, err := c.waClient.SendMessage(ctx, jid, msg)
	return err
}

// MockClient implements the same interface as Client but does nothing (for tests)
// In tests, use whatsapp.NewMockClient() instead of NewClient to avoid real WhatsApp connections.
// Update api_test.go to use MockClient for waClient.
type MockClient struct{}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) SendMessage(ctx context.Context, to string, body string) error {
	return nil
}
