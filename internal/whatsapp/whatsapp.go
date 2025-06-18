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

// WhatsAppSender is an interface for sending WhatsApp messages (for production and testing)
type WhatsAppSender interface {
	SendMessage(ctx context.Context, to string, body string) error
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
	slog.Debug("WhatsApp NewClient options set", "DBDSN_set", cfg.DBDSN != "", "QRPath_set", cfg.QRPath != "", "NumericCode", cfg.NumericCode)

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
// It performs comprehensive validation and provides detailed error information.
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

	slog.Debug("Sending WhatsApp message", "to", to, "body_length", len(body))
	jid := types.NewJID(to, JIDSuffix)
	msg := &waE2E.Message{Conversation: &body}

	_, err := c.waClient.SendMessage(ctx, jid, msg)
	if err != nil {
		slog.Error("Failed to send WhatsApp message", "error", err, "to", to)
		return fmt.Errorf("failed to send message to %s: %w", to, err)
	}

	slog.Debug("WhatsApp message sent successfully", "to", to)
	return nil
}

// GetClient returns the underlying whatsmeow client for event handling
func (c *Client) GetClient() *whatsmeow.Client {
	return c.waClient
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
