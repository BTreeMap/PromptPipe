package store

import (
	"syscall"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestInMemoryStore(t *testing.T) {
	s := NewInMemoryStore()
	r := models.Receipt{To: "+123", Status: "sent", Time: 1}
	err := s.AddReceipt(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(receipts) != 1 || receipts[0].To != "+123" {
		t.Error("Receipt not stored or retrieved correctly")
	}
}

func TestPostgresStore(t *testing.T) {
	connStr := getenvOrSkip(t, "DATABASE_URL")
	pgStore, err := NewPostgresStore(WithPostgresDSN(connStr))
	if err != nil {
		t.Skipf("Postgres not available: %v", err)
	}
	// Insert and verify receipt
	r := models.Receipt{To: "+123", Status: models.StatusTypeSent, Time: 1}
	err = pgStore.AddReceipt(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	receipts, err := pgStore.GetReceipts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(receipts) == 0 || receipts[0].To != "+123" {
		t.Error("Receipt not stored or retrieved correctly in Postgres")
	}
}

func getenvOrSkip(t *testing.T, key string) string {
	v := ""
	if val, ok := syscall.Getenv(key); ok {
		v = val
	}
	if v == "" {
		t.Skipf("env %s not set", key)
	}
	return v
}
