package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestDetectDSNType(t *testing.T) {
	tests := []struct {
		name           string
		dsn            string
		expectedDriver string
	}{
		{
			name:           "PostgreSQL DSN with postgres:// scheme",
			dsn:            "postgres://user:password@localhost/dbname",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with host= parameter",
			dsn:            "host=localhost user=postgres dbname=test",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with multiple key=value pairs",
			dsn:            "user=postgres password=secret dbname=test sslmode=disable",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with single key=value pair",
			dsn:            "dbname=test",
			expectedDriver: "postgres",
		},
		{
			name:           "SQLite DSN with absolute file path",
			dsn:            "/var/lib/promptpipe/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with relative path",
			dsn:            "./data/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with .db extension",
			dsn:            "test.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN without extension",
			dsn:            "/tmp/database",
			expectedDriver: "sqlite3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detectedDriver := DetectDSNType(tt.dsn)
			if detectedDriver != tt.expectedDriver {
				t.Errorf("DSN detection failed for %q: expected driver %q, got %q", tt.dsn, tt.expectedDriver, detectedDriver)
			}
		})
	}
}

func TestInMemoryStore(t *testing.T) {
	s := NewInMemoryStore()
	r := models.Receipt{To: "+123", Status: models.StatusTypeSent, Time: 1}
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
	defer pgStore.ClearReceipts()

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

func TestSQLiteStore(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	sqliteStore, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer sqliteStore.db.Close()

	// Test receipt storage
	r := models.Receipt{To: "+123", Status: models.StatusTypeSent, Time: 1}
	err = sqliteStore.AddReceipt(r)
	if err != nil {
		t.Fatalf("unexpected error adding receipt: %v", err)
	}

	receipts, err := sqliteStore.GetReceipts()
	if err != nil {
		t.Fatalf("unexpected error getting receipts: %v", err)
	}

	if len(receipts) != 1 || receipts[0].To != "+123" {
		t.Error("Receipt not stored or retrieved correctly in SQLite")
	}

	// Test response storage
	resp := models.Response{From: "+456", Body: "Hello", Time: 2}
	err = sqliteStore.AddResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error adding response: %v", err)
	}

	responses, err := sqliteStore.GetResponses()
	if err != nil {
		t.Fatalf("unexpected error getting responses: %v", err)
	}

	if len(responses) != 1 || responses[0].From != "+456" || responses[0].Body != "Hello" {
		t.Error("Response not stored or retrieved correctly in SQLite")
	}

	// Test clear receipts
	err = sqliteStore.ClearReceipts()
	if err != nil {
		t.Fatalf("unexpected error clearing receipts: %v", err)
	}

	receipts, err = sqliteStore.GetReceipts()
	if err != nil {
		t.Fatalf("unexpected error getting receipts after clear: %v", err)
	}

	if len(receipts) != 0 {
		t.Error("Receipts not cleared properly in SQLite")
	}
}

func getenvOrSkip(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("Environment variable %s not set", key)
	}
	return value
}
