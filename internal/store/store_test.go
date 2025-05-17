package store

import (
	"testing"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestInMemoryStore(t *testing.T) {
	s := NewInMemoryStore()
	r := models.Receipt{To: "+123", Status: "sent", Time: 1}
	s.AddReceipt(r)
	receipts := s.GetReceipts()
	if len(receipts) != 1 || receipts[0].To != "+123" {
		t.Error("Receipt not stored or retrieved correctly")
	}
}
