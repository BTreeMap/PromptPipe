package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvironmentConfigDefaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("WHATSAPP_DB_DSN")
	os.Unsetenv("DATABASE_DSN")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	config := loadEnvironmentConfig()

	// Test default state directory
	if config.StateDir != DefaultStateDir {
		t.Errorf("Expected default state dir %q, got %q", DefaultStateDir, config.StateDir)
	}

	// Test default WhatsApp database DSN
	expectedWhatsAppDSN := filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName)
	if config.WhatsAppDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected default WhatsApp DSN %q, got %q", expectedWhatsAppDSN, config.WhatsAppDBDSN)
	}

	// Test default application database DSN
	expectedAppDSN := filepath.Join(DefaultStateDir, DefaultAppDBFileName)
	if config.AppDBDSN != expectedAppDSN {
		t.Errorf("Expected default app DSN %q, got %q", expectedAppDSN, config.AppDBDSN)
	}
}

func TestLoadEnvironmentConfigLegacySupport(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("WHATSAPP_DB_DSN")
	os.Unsetenv("DATABASE_DSN")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	// Set legacy DATABASE_URL
	legacyDSN := "postgres://user:pass@localhost/db"
	os.Setenv("DATABASE_URL", legacyDSN)
	defer os.Unsetenv("DATABASE_URL")

	config := loadEnvironmentConfig()

	// DATABASE_URL should be used for AppDBDSN when DATABASE_DSN is not set
	if config.AppDBDSN != legacyDSN {
		t.Errorf("Expected app DSN to use DATABASE_URL %q, got %q", legacyDSN, config.AppDBDSN)
	}

	// WhatsApp DSN should still use default
	expectedWhatsAppDSN := filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName)
	if config.WhatsAppDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected default WhatsApp DSN %q, got %q", expectedWhatsAppDSN, config.WhatsAppDBDSN)
	}
}

func TestLoadEnvironmentConfigSeparateDSNs(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	// Set separate DSNs
	whatsappDSN := "postgres://user:pass@localhost/whatsapp"
	appDSN := "postgres://user:pass@localhost/app"
	os.Setenv("WHATSAPP_DB_DSN", whatsappDSN)
	os.Setenv("DATABASE_DSN", appDSN)
	defer func() {
		os.Unsetenv("WHATSAPP_DB_DSN")
		os.Unsetenv("DATABASE_DSN")
	}()

	config := loadEnvironmentConfig()

	// Both DSNs should be set correctly
	if config.WhatsAppDBDSN != whatsappDSN {
		t.Errorf("Expected WhatsApp DSN %q, got %q", whatsappDSN, config.WhatsAppDBDSN)
	}

	if config.AppDBDSN != appDSN {
		t.Errorf("Expected app DSN %q, got %q", appDSN, config.AppDBDSN)
	}
}

func TestEnsureDirectoriesExist(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	whatsappDBPath := filepath.Join(tempDir, "subdir", "whatsapp.db")
	appDBPath := filepath.Join(tempDir, "subdir", "app.db")

	flags := Flags{
		whatsappDBDSN: &whatsappDBPath,
		appDBDSN:      &appDBPath,
		stateDir:      &tempDir,
	}

	err := ensureDirectoriesExist(flags)
	if err != nil {
		t.Fatalf("ensureDirectoriesExist failed: %v", err)
	}

	// Check that the subdirectory was created
	subDir := filepath.Join(tempDir, "subdir")
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Errorf("Directory %s was not created", subDir)
	}
}

func TestBuildWhatsAppOptions(t *testing.T) {
	qrPath := "/tmp/qr.txt"
	dsn := "postgres://test/whatsapp"
	numeric := true

	flags := Flags{
		qrOutput:      &qrPath,
		numeric:       &numeric,
		whatsappDBDSN: &dsn,
	}

	opts := buildWhatsAppOptions(flags)

	// Should have 3 options
	if len(opts) != 3 {
		t.Errorf("Expected 3 WhatsApp options, got %d", len(opts))
	}
}

func TestBuildStoreOptions(t *testing.T) {
	// Test PostgreSQL DSN
	pgDSN := "postgres://user:pass@localhost/db"
	flags := Flags{
		appDBDSN: &pgDSN,
	}

	opts := buildStoreOptions(flags)
	if len(opts) != 1 {
		t.Errorf("Expected 1 store option for PostgreSQL, got %d", len(opts))
	}

	// Test SQLite DSN
	sqliteDSN := "/tmp/app.db"
	flags.appDBDSN = &sqliteDSN

	opts = buildStoreOptions(flags)
	if len(opts) != 1 {
		t.Errorf("Expected 1 store option for SQLite, got %d", len(opts))
	}

	// Test empty DSN
	emptyDSN := ""
	flags.appDBDSN = &emptyDSN

	opts = buildStoreOptions(flags)
	if len(opts) != 0 {
		t.Errorf("Expected 0 store options for empty DSN, got %d", len(opts))
	}
}
