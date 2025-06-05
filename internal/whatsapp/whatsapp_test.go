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

func TestWithDBDriverOption(t *testing.T) {
	opts := &Opts{}

	// Test setting PostgreSQL driver
	WithDBDriver("postgres")(opts)
	if opts.DBDriver != "postgres" {
		t.Errorf("Expected DBDriver to be 'postgres', got %q", opts.DBDriver)
	}

	// Test setting SQLite driver
	WithDBDriver("sqlite3")(opts)
	if opts.DBDriver != "sqlite3" {
		t.Errorf("Expected DBDriver to be 'sqlite3', got %q", opts.DBDriver)
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

func TestNewClientOptionsApplied(t *testing.T) {
	// Test that options are properly applied when creating a new client
	// We don't actually create the client to avoid database connections

	testDSN := "/tmp/test.db"
	testDriver := "sqlite3"

	opts := &Opts{}
	WithDBDSN(testDSN)(opts)
	WithDBDriver(testDriver)(opts)

	if opts.DBDSN != testDSN {
		t.Errorf("Expected DBDSN to be %q, got %q", testDSN, opts.DBDSN)
	}

	if opts.DBDriver != testDriver {
		t.Errorf("Expected DBDriver to be %q, got %q", testDriver, opts.DBDriver)
	}
}
