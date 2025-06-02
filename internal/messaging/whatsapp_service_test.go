package messaging

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
)

// Ensure WhatsAppService implements Service interface
func TestWhatsAppService_ImplementsService(t *testing.T) {
	var _ Service = (*WhatsAppService)(nil)
}

// Test SendMessage emits a sent receipt
func TestWhatsAppService_SendMessage_Receipt(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)
	ctx := context.Background()
	to, body := "+123", "hello"
	if err := svc.SendMessage(ctx, to, body); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	select {
	case receipt := <-svc.Receipts():
		if receipt.To != to {
			t.Errorf("expected receipt.To %s, got %s", to, receipt.To)
		}
		if receipt.Status != models.StatusTypeSent {
			t.Errorf("expected receipt.Status %s, got %s", models.StatusTypeSent, receipt.Status)
		}
	default:
		t.Fatal("expected receipt, got none")
	}
}

// Test Start and Stop do not error and close channels
func TestWhatsAppService_StartStop(t *testing.T) {
	mockClient := whatsapp.NewMockClient()
	svc := NewWhatsAppService(mockClient)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	// After Stop, Receipts and Responses channels should be closed
	// Receiving from closed channels yields zero value immediately
	receipt, ok := <-svc.Receipts()
	if ok {
		t.Errorf("expected receipts channel closed, got value %v", receipt)
	}
	response, ok := <-svc.Responses()
	if ok {
		t.Errorf("expected responses channel closed, got value %v", response)
	}
}
