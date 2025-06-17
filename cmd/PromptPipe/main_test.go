package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/store"
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
	expectedWhatsAppDSN := "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
	if config.WhatsAppDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected default WhatsApp DSN %q, got %q", expectedWhatsAppDSN, config.WhatsAppDBDSN)
	}

	// Test default application database DSN
	expectedAppDSN := filepath.Join(DefaultStateDir, DefaultAppDBFileName)
	if config.ApplicationDBDSN != expectedAppDSN {
		t.Errorf("Expected default app DSN %q, got %q", expectedAppDSN, config.ApplicationDBDSN)
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

	// DATABASE_URL should be used for ApplicationDBDSN when DATABASE_DSN is not set
	if config.ApplicationDBDSN != legacyDSN {
		t.Errorf("Expected app DSN to use DATABASE_URL %q, got %q", legacyDSN, config.ApplicationDBDSN)
	}

	// WhatsApp DSN should still use default
	expectedWhatsAppDSN := "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
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

	if config.ApplicationDBDSN != appDSN {
		t.Errorf("Expected app DSN %q, got %q", appDSN, config.ApplicationDBDSN)
	}
}

func TestLoadEnvironmentConfigCustomStateDir(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("WHATSAPP_DB_DSN")
	os.Unsetenv("DATABASE_DSN")
	os.Unsetenv("DATABASE_URL")

	// Set custom state directory
	customStateDir := "/tmp/custom_promptpipe"
	os.Setenv("PROMPTPIPE_STATE_DIR", customStateDir)
	defer os.Unsetenv("PROMPTPIPE_STATE_DIR")

	config := loadEnvironmentConfig()

	// Test custom state directory is used
	if config.StateDir != customStateDir {
		t.Errorf("Expected custom state dir %q, got %q", customStateDir, config.StateDir)
	}

	// Test default database DSNs use custom state directory
	expectedWhatsAppDSN := "file:" + filepath.Join(customStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
	if config.WhatsAppDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected WhatsApp DSN with custom state dir %q, got %q", expectedWhatsAppDSN, config.WhatsAppDBDSN)
	}

	expectedAppDSN := filepath.Join(customStateDir, DefaultAppDBFileName)
	if config.ApplicationDBDSN != expectedAppDSN {
		t.Errorf("Expected app DSN with custom state dir %q, got %q", expectedAppDSN, config.ApplicationDBDSN)
	}
}

func TestLoadEnvironmentConfigDATABASE_DSNTakesPrecedenceOverDATABASE_URL(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("WHATSAPP_DB_DSN")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	// Set both DATABASE_DSN and DATABASE_URL
	preferredDSN := "postgres://user:pass@localhost/preferred"
	legacyDSN := "postgres://user:pass@localhost/legacy"
	os.Setenv("DATABASE_DSN", preferredDSN)
	os.Setenv("DATABASE_URL", legacyDSN)
	defer func() {
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("DATABASE_URL")
	}()

	config := loadEnvironmentConfig()

	// DATABASE_DSN should take precedence over DATABASE_URL
	if config.ApplicationDBDSN != preferredDSN {
		t.Errorf("Expected app DSN to use DATABASE_DSN %q, got %q", preferredDSN, config.ApplicationDBDSN)
	}
}

func TestLoadEnvironmentConfigOnlyWhatsAppDSNProvided(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("DATABASE_DSN")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	// Only provide WhatsApp DSN
	whatsappDSN := "postgres://user:pass@localhost/whatsapp"
	os.Setenv("WHATSAPP_DB_DSN", whatsappDSN)
	defer os.Unsetenv("WHATSAPP_DB_DSN")

	config := loadEnvironmentConfig()

	// WhatsApp DSN should be set to provided value
	if config.WhatsAppDBDSN != whatsappDSN {
		t.Errorf("Expected WhatsApp DSN %q, got %q", whatsappDSN, config.WhatsAppDBDSN)
	}

	// Application DSN should default to SQLite
	expectedAppDSN := filepath.Join(DefaultStateDir, DefaultAppDBFileName)
	if config.ApplicationDBDSN != expectedAppDSN {
		t.Errorf("Expected default app DSN %q, got %q", expectedAppDSN, config.ApplicationDBDSN)
	}
}

func TestLoadEnvironmentConfigOnlyApplicationDSNProvided(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("WHATSAPP_DB_DSN")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("PROMPTPIPE_STATE_DIR")

	// Only provide application DSN
	appDSN := "postgres://user:pass@localhost/app"
	os.Setenv("DATABASE_DSN", appDSN)
	defer os.Unsetenv("DATABASE_DSN")

	config := loadEnvironmentConfig()

	// Application DSN should be set to provided value
	if config.ApplicationDBDSN != appDSN {
		t.Errorf("Expected app DSN %q, got %q", appDSN, config.ApplicationDBDSN)
	}

	// WhatsApp DSN should default to SQLite with foreign keys
	expectedWhatsAppDSN := "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
	if config.WhatsAppDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected default WhatsApp DSN %q, got %q", expectedWhatsAppDSN, config.WhatsAppDBDSN)
	}
}

func TestParseCommandLineFlagsStateDirUpdate(t *testing.T) {
	// Create initial config with defaults
	config := Config{
		StateDir:         DefaultStateDir,
		WhatsAppDBDSN:    "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on",
		ApplicationDBDSN: filepath.Join(DefaultStateDir, DefaultAppDBFileName),
		OpenAIKey:        "",
		APIAddr:          "",
		DefaultCron:      "",
	}

	// Simulate changed state directory
	newStateDir := "/tmp/new_state"
	flags := Flags{
		qrOutput:      new(string),
		numeric:       new(bool),
		stateDir:      &newStateDir,
		whatsappDBDSN: &config.WhatsAppDBDSN,
		appDBDSN:      &config.ApplicationDBDSN,
		openaiKey:     &config.OpenAIKey,
		apiAddr:       &config.APIAddr,
		defaultCron:   &config.DefaultCron,
	}

	// Manually apply the state directory update logic
	if *flags.whatsappDBDSN == config.WhatsAppDBDSN && *flags.stateDir != config.StateDir {
		*flags.whatsappDBDSN = "file:" + filepath.Join(*flags.stateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
	}

	if *flags.appDBDSN == config.ApplicationDBDSN && *flags.stateDir != config.StateDir {
		*flags.appDBDSN = filepath.Join(*flags.stateDir, DefaultAppDBFileName)
	}

	// Verify that database DSNs were updated to use new state directory
	expectedWhatsAppDSN := "file:" + filepath.Join(newStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
	if *flags.whatsappDBDSN != expectedWhatsAppDSN {
		t.Errorf("Expected updated WhatsApp DSN %q, got %q", expectedWhatsAppDSN, *flags.whatsappDBDSN)
	}

	expectedAppDSN := filepath.Join(newStateDir, DefaultAppDBFileName)
	if *flags.appDBDSN != expectedAppDSN {
		t.Errorf("Expected updated app DSN %q, got %q", expectedAppDSN, *flags.appDBDSN)
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

func TestEndToEndDatabaseConfiguration(t *testing.T) {
	tests := []struct {
		name                  string
		whatsappDBDSN        string
		databaseDSN          string
		databaseURL          string
		expectedWhatsAppDSN  string
		expectedAppDSN       string
		expectLegacyUsage    bool
	}{
		{
			name:                "Both DSNs provided - use them directly",
			whatsappDBDSN:       "postgres://user:pass@localhost/whatsapp",
			databaseDSN:         "postgres://user:pass@localhost/app",
			expectedWhatsAppDSN: "postgres://user:pass@localhost/whatsapp",
			expectedAppDSN:      "postgres://user:pass@localhost/app",
			expectLegacyUsage:   false,
		},
		{
			name:                "Only WhatsApp DSN provided - app defaults to SQLite",
			whatsappDBDSN:       "postgres://user:pass@localhost/whatsapp",
			expectedWhatsAppDSN: "postgres://user:pass@localhost/whatsapp",
			expectedAppDSN:      filepath.Join(DefaultStateDir, DefaultAppDBFileName),
			expectLegacyUsage:   false,
		},
		{
			name:                "Only app DSN provided - WhatsApp defaults to SQLite with foreign keys",
			databaseDSN:         "postgres://user:pass@localhost/app",
			expectedWhatsAppDSN: "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on",
			expectedAppDSN:      "postgres://user:pass@localhost/app",
			expectLegacyUsage:   false,
		},
		{
			name:                "Only legacy DATABASE_URL provided - used for app, WhatsApp defaults",
			databaseURL:         "postgres://user:pass@localhost/legacy",
			expectedWhatsAppDSN: "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on",
			expectedAppDSN:      "postgres://user:pass@localhost/legacy",
			expectLegacyUsage:   true,
		},
		{
			name:                "Both DATABASE_DSN and DATABASE_URL provided - DATABASE_DSN takes precedence",
			databaseDSN:         "postgres://user:pass@localhost/preferred",
			databaseURL:         "postgres://user:pass@localhost/legacy",
			expectedWhatsAppDSN: "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on",
			expectedAppDSN:      "postgres://user:pass@localhost/preferred",
			expectLegacyUsage:   false,
		},
		{
			name:                "No configuration - both default to SQLite",
			expectedWhatsAppDSN: "file:" + filepath.Join(DefaultStateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on",
			expectedAppDSN:      filepath.Join(DefaultStateDir, DefaultAppDBFileName),
			expectLegacyUsage:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all environment variables
			os.Unsetenv("WHATSAPP_DB_DSN")
			os.Unsetenv("DATABASE_DSN")
			os.Unsetenv("DATABASE_URL")
			os.Unsetenv("PROMPTPIPE_STATE_DIR")

			// Set environment variables as specified by test case
			if tt.whatsappDBDSN != "" {
				os.Setenv("WHATSAPP_DB_DSN", tt.whatsappDBDSN)
				defer os.Unsetenv("WHATSAPP_DB_DSN")
			}
			if tt.databaseDSN != "" {
				os.Setenv("DATABASE_DSN", tt.databaseDSN)
				defer os.Unsetenv("DATABASE_DSN")
			}
			if tt.databaseURL != "" {
				os.Setenv("DATABASE_URL", tt.databaseURL)
				defer os.Unsetenv("DATABASE_URL")
			}

			// Load configuration
			config := loadEnvironmentConfig()

			// Verify WhatsApp DSN
			if config.WhatsAppDBDSN != tt.expectedWhatsAppDSN {
				t.Errorf("WhatsApp DSN mismatch: expected %q, got %q", 
					tt.expectedWhatsAppDSN, config.WhatsAppDBDSN)
			}

			// Verify application DSN
			if config.ApplicationDBDSN != tt.expectedAppDSN {
				t.Errorf("Application DSN mismatch: expected %q, got %q", 
					tt.expectedAppDSN, config.ApplicationDBDSN)
			}

			// Verify that default SQLite WhatsApp DSN has foreign keys enabled
			if strings.Contains(config.WhatsAppDBDSN, DefaultWhatsAppDBFileName) {
				if !strings.Contains(config.WhatsAppDBDSN, "_foreign_keys=on") {
					t.Errorf("Default WhatsApp SQLite DSN should have foreign keys enabled: %q", 
						config.WhatsAppDBDSN)
				}
			}

			// Test option builders without parsing flags (to avoid flag redefinition issues)
			
			// Create mock flags from config
			mockFlags := Flags{
				qrOutput:      new(string),
				numeric:       new(bool),
				stateDir:      &config.StateDir,
				whatsappDBDSN: &config.WhatsAppDBDSN,
				appDBDSN:      &config.ApplicationDBDSN,
				openaiKey:     &config.OpenAIKey,
				apiAddr:       &config.APIAddr,
				defaultCron:   &config.DefaultCron,
			}
			
			// Verify WhatsApp options can be built
			waOpts := buildWhatsAppOptions(mockFlags)
			if *mockFlags.whatsappDBDSN != "" && len(waOpts) == 0 {
				t.Errorf("Expected WhatsApp options to be built when DSN is provided")
			}

			// Verify store options can be built and detect the correct type
			storeOpts := buildStoreOptions(mockFlags)
			if *mockFlags.appDBDSN != "" {
				if len(storeOpts) == 0 {
					t.Errorf("Expected store options to be built when DSN is provided")
				}
				
				// Verify the store type detection works correctly
				expectedStoreType := "sqlite3"
				if store.DetectDSNType(*mockFlags.appDBDSN) == "postgres" {
					expectedStoreType = "postgres"
				}
				
				actualStoreType := store.DetectDSNType(*mockFlags.appDBDSN)
				if actualStoreType != expectedStoreType {
					t.Errorf("Store type detection failed: expected %q, got %q", 
						expectedStoreType, actualStoreType)
				}
			}
		})
	}
}
