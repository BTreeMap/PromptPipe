// Package whatsapp wraps the Whatsmeow client for WhatsApp integration in PromptPipe.
//
// It provides methods for sending messages and handling WhatsApp events.
package whatsapp

import (
	"context"
	"fmt"
	"os"

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

// Client wraps the Whatsmeow client for modular use

type Client struct {
	waClient *whatsmeow.Client
}

func NewClient() (*Client, error) {
	// Use environment variables for DB driver and DSN
	dbDriver := os.Getenv("WHATSAPP_DB_DRIVER")
	if dbDriver == "" {
		dbDriver = "postgres"
	}
	dbDSN := os.Getenv("WHATSAPP_DB_DSN")
	if dbDSN == "" {
		// Default to a typical local Postgres connection string
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
