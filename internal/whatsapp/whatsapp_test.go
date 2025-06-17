package whatsapp

import (
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestDSNDetection(t *testing.T) {
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
			name:           "SQLite DSN with file path",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the DSN detection logic using the shared function
			detectedDriver := store.DetectDSNType(tt.dsn)

			if detectedDriver != tt.expectedDriver {
				t.Errorf("DSN detection failed for %q: expected driver %q, got %q", tt.dsn, tt.expectedDriver, detectedDriver)
			}
		})
	}
}

func TestWithDBDSNOption(t *testing.T) {
	opts := &Opts{}

	testDSN := "/var/lib/promptpipe/test.db"
	WithDBDSN(testDSN)(opts)

	if opts.DBDSN != testDSN {
		t.Errorf("Expected DBDSN to be %q, got %q", testDSN, opts.DBDSN)
	}
}

func TestWithQRCodeOutputOption(t *testing.T) {
	opts := &Opts{}

	testPath := "/tmp/qr.txt"
	WithQRCodeOutput(testPath)(opts)

	if opts.QRPath != testPath {
		t.Errorf("Expected QRPath to be %q, got %q", testPath, opts.QRPath)
	}
}

func TestWithNumericCodeOption(t *testing.T) {
	opts := &Opts{}

	WithNumericCode()(opts)

	if !opts.NumericCode {
		t.Errorf("Expected NumericCode to be true, got false")
	}
}

func TestNewClientOptionsApplied(t *testing.T) {
	// Test that options are properly applied when creating a new client
	// We don't actually create the client to avoid database connections

	testDSN := "/tmp/test.db"

	opts := &Opts{}
	WithDBDSN(testDSN)(opts)

	if opts.DBDSN != testDSN {
		t.Errorf("Expected DBDSN to be %q, got %q", testDSN, opts.DBDSN)
	}
}
