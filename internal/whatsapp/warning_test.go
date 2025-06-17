package whatsapp

import (
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestForeignKeyDetection(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		hasForeignKeys bool
	}{
		{
			name:        "SQLite DSN without foreign keys",
			dsn:         "/tmp/test.db",
			hasForeignKeys: false,
		},
		{
			name:        "SQLite DSN with _foreign_keys parameter",
			dsn:         "file:/tmp/test.db?_foreign_keys=on",
			hasForeignKeys: true,
		},
		{
			name:        "SQLite DSN with foreign_keys parameter",
			dsn:         "/tmp/test.db?foreign_keys=on",
			hasForeignKeys: true,
		},
		{
			name:        "PostgreSQL DSN (foreign keys irrelevant)",
			dsn:         "postgres://user:pass@localhost/db",
			hasForeignKeys: true, // PostgreSQL doesn't need this check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that determines if foreign keys are enabled
			isSQLite := store.DetectDSNType(tt.dsn) == "sqlite3"
			hasForeignKeys := strings.Contains(tt.dsn, "_foreign_keys") || strings.Contains(tt.dsn, "foreign_keys")
			
			shouldWarn := isSQLite && !hasForeignKeys
			expectedWarn := isSQLite && !tt.hasForeignKeys
			
			if shouldWarn != expectedWarn {
				t.Errorf("Foreign key detection failed for %q: shouldWarn=%v, expected=%v", 
					tt.dsn, shouldWarn, expectedWarn)
			}
		})
	}
}

func TestDSNTypeDetectionForForeignKeys(t *testing.T) {
	// Test that our foreign key warning logic only applies to SQLite
	sqliteDSN := "/tmp/test.db"
	postgresDSN := "postgres://user:pass@localhost/db"
	
	if store.DetectDSNType(sqliteDSN) != "sqlite3" {
		t.Errorf("Expected SQLite DSN to be detected as sqlite3")
	}
	
	if store.DetectDSNType(postgresDSN) != "postgres" {
		t.Errorf("Expected PostgreSQL DSN to be detected as postgres")
	}
}
