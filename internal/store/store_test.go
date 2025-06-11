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
	r1 := models.Receipt{To: "+123", Status: models.MessageStatusSent, Time: 1}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	resp1 := models.Response{From: "+123", Body: "response 1", Time: 2}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	if len(receipts) != 1 || receipts[0].To != r1.To {
		t.Errorf("GetReceipts: expected %+v, got %+v", r1, receipts)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	if len(responses) != 1 || responses[0].From != resp1.From {
		t.Errorf("GetResponses: expected %+v, got %+v", resp1, responses)
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func TestPostgresStore(t *testing.T) {
	dsn := getenvOrSkip(t, "POSTGRES_DSN_TEST") // Example: "postgres://user:pass@host:port/dbname?sslmode=disable"
	if dsn == "" {
		t.Skip("POSTGRES_DSN_TEST not set, skipping PostgresStore tests")
	}

	s, err := NewPostgresStore(WithPostgresDSN(dsn))
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer s.Close() // Correctly call Close on s

	// Clear existing data for a clean test run
	if err := s.ClearReceipts(); err != nil {
		// It's okay if the table doesn't exist yet on the first run before migrations
		// t.Logf("ClearReceipts (pre-test) failed, possibly due to no table: %v", err)
	}
	if err := s.ClearResponses(); err != nil {
		// t.Logf("ClearResponses (pre-test) failed, possibly due to no table: %v", err)
	}

	r1 := models.Receipt{To: "pg_test_1", Status: models.MessageStatusDelivered, Time: 100}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	foundR1 := false
	for _, r := range receipts {
		if r.To == r1.To && r.Status == r1.Status && r.Time == r1.Time {
			foundR1 = true
			break
		}
	}
	if !foundR1 {
		t.Errorf("GetReceipts: did not find expected receipt %+v in %+v", r1, receipts)
	}

	resp1 := models.Response{From: "pg_test_resp_1", Body: "postgres response", Time: 101}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	foundResp1 := false
	for _, r := range responses {
		if r.From == resp1.From && r.Body == resp1.Body && r.Time == resp1.Time {
			foundResp1 = true
			break
		}
	}
	if !foundResp1 {
		t.Errorf("GetResponses: did not find expected response %+v in %+v", resp1, responses)
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func TestSQLiteStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlite_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	s, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer s.Close() // Ensure SQLiteStore has a Close method

	r1 := models.Receipt{To: "sqlite_test_1", Status: models.MessageStatusRead, Time: 200}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	if len(receipts) != 1 || receipts[0].To != r1.To || receipts[0].Status != r1.Status || receipts[0].Time != r1.Time {
		t.Errorf("GetReceipts: expected %+v, got %+v", r1, receipts)
	}

	resp1 := models.Response{From: "sqlite_test_resp_1", Body: "sqlite response", Time: 201}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	if len(responses) != 1 || responses[0].From != resp1.From || responses[0].Body != resp1.Body || responses[0].Time != resp1.Time {
		t.Errorf("GetResponses: expected %+v, got %+v", resp1, responses[0])
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func getenvOrSkip(t *testing.T, key string) string {
	t.Helper()
	val := os.Getenv(key)
	if val == "" {
		// t.Skipf("Environment variable %s not set, skipping test", key)
	}
	return val
}
