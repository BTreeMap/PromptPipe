package main

import (
	"flag"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"github.com/joho/godotenv"
)

// Default configuration constants
const (
	// DefaultStateDir is the default directory for PromptPipe state data
	DefaultStateDir = "/var/lib/promptpipe"
	// DefaultAppDBFileName is the default SQLite database filename for application data
	DefaultAppDBFileName = "app.db"
	// DefaultWhatsAppDBFileName is the default SQLite database filename for WhatsApp/whatsmeow data
	DefaultWhatsAppDBFileName = "whatsapp.db"
)

func main() {
	// Initialize structured logger
	initializeLogger()

	// Load environment configuration
	config := loadEnvironmentConfig()

	// Parse command line flags
	flags := parseCommandLineFlags(config)

	// Ensure required directories exist
	if err := ensureDirectoriesExist(flags); err != nil {
		slog.Error("Failed to create required directories", "error", err)
		os.Exit(1)
	}

	// Build module options
	waOpts := buildWhatsAppOptions(flags)
	storeOpts := buildStoreOptions(flags)
	genaiOpts := buildGenAIOptions(flags)
	apiOpts := buildAPIOptions(flags)

	// Start the service
	slog.Info("Bootstrapping PromptPipe with configured modules")
	slog.Debug("Module options counts", "whatsapp", len(waOpts), "store", len(storeOpts), "genai", len(genaiOpts), "api", len(apiOpts))
	slog.Debug("Final configuration", "state_dir", *flags.stateDir, "whatsapp_dsn_set", *flags.whatsappDBDSN != "", "app_dsn_set", *flags.appDBDSN != "", "api_addr", *flags.apiAddr)
	if err := api.Run(waOpts, storeOpts, genaiOpts, apiOpts); err != nil {
		slog.Error("PromptPipe failed to run", "error", err)
		os.Exit(1)
	}
	slog.Info("PromptPipe exited successfully")
}

// Config holds environment configuration for database connections and other settings.
// This enforces clear separation between WhatsApp/whatsmeow database and application database.
type Config struct {
	// WhatsAppDBDSN is the connection string for the WhatsApp/whatsmeow database.
	// This database is managed by the whatsmeow library and stores WhatsApp session data.
	// Environment variable: WHATSAPP_DB_DSN
	WhatsAppDBDSN string

	// ApplicationDBDSN is the connection string for the application database.
	// This database stores receipts, responses, flow state, and other application data.
	// Environment variables: DATABASE_DSN (preferred) or DATABASE_URL (legacy support)
	ApplicationDBDSN string

	// StateDir is the directory for file-based storage (used for default SQLite paths).
	// Environment variable: PROMPTPIPE_STATE_DIR
	StateDir string

	// OpenAIKey is the API key for OpenAI GenAI operations.
	// Environment variable: OPENAI_API_KEY
	OpenAIKey string

	// APIAddr is the HTTP server address for the REST API.
	// Environment variable: API_ADDR
	APIAddr string

	// DefaultCron is the default cron schedule for prompts.
	// Environment variable: DEFAULT_SCHEDULE
	DefaultCron string
}

// Flags holds command line flag values for database and other configuration.
// This provides clear separation between WhatsApp and application database settings.
type Flags struct {
	qrOutput      *string
	numeric       *bool
	stateDir      *string
	whatsappDBDSN *string // WhatsApp/whatsmeow database connection string
	appDBDSN      *string // Application database connection string
	openaiKey     *string
	apiAddr       *string
	defaultCron   *string
}

// initializeLogger sets up structured logging with debug level
func initializeLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
}

// loadEnvironmentConfig loads configuration from environment variables and .env file.
// This separates WhatsApp database configuration from application database configuration.
func loadEnvironmentConfig() Config {
	if err := godotenv.Load(); err != nil {
		slog.Debug("failed to load .env file", "error", err)
	} else {
		slog.Debug("successfully loaded .env file")
	}

	config := Config{
		WhatsAppDBDSN:    os.Getenv("WHATSAPP_DB_DSN"),
		ApplicationDBDSN: os.Getenv("DATABASE_DSN"),
		StateDir:         os.Getenv("PROMPTPIPE_STATE_DIR"),
		OpenAIKey:        os.Getenv("OPENAI_API_KEY"),
		APIAddr:          os.Getenv("API_ADDR"),
		DefaultCron:      os.Getenv("DEFAULT_SCHEDULE"),
	}

	// Set default state directory if not specified
	if config.StateDir == "" {
		config.StateDir = DefaultStateDir
		slog.Debug("No PROMPTPIPE_STATE_DIR set, using default", "default_state_dir", config.StateDir)
	} else {
		slog.Debug("PROMPTPIPE_STATE_DIR found in environment", "state_dir", config.StateDir)
	}

	// Handle legacy DATABASE_URL - use it for ApplicationDBDSN if DATABASE_DSN is not set
	if config.ApplicationDBDSN == "" {
		if legacyDBURL := os.Getenv("DATABASE_URL"); legacyDBURL != "" {
			config.ApplicationDBDSN = legacyDBURL
			slog.Debug("Using DATABASE_URL as application database DSN (legacy support)", "dsn_set", true)
		}
	}

	// Set default database DSNs if not provided
	if config.WhatsAppDBDSN == "" {
		// Default SQLite with foreign keys enabled (recommended by whatsmeow)
		config.WhatsAppDBDSN = "file:" + filepath.Join(config.StateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
		slog.Debug("No WhatsApp database DSN provided, defaulting to SQLite with foreign keys", "sqlite_path", config.WhatsAppDBDSN)
	}

	if config.ApplicationDBDSN == "" {
		// Default SQLite for application data with foreign keys enabled
		config.ApplicationDBDSN = "file:" + filepath.Join(config.StateDir, DefaultAppDBFileName) + "?_foreign_keys=on"
		slog.Debug("No application database DSN provided, defaulting to SQLite with foreign keys", "sqlite_path", config.ApplicationDBDSN)
	}

	slog.Debug("environment variables loaded",
		"WHATSAPP_DB_DSN_SET", config.WhatsAppDBDSN != "",
		"APPLICATION_DB_DSN_SET", config.ApplicationDBDSN != "",
		"DATABASE_URL_LEGACY_SET", os.Getenv("DATABASE_URL") != "",
		"PROMPTPIPE_STATE_DIR", config.StateDir,
		"OPENAI_API_KEY_SET", config.OpenAIKey != "",
		"API_ADDR", config.APIAddr,
		"DEFAULT_SCHEDULE", config.DefaultCron)

	return config
}

// parseCommandLineFlags parses command line arguments with environment defaults.
// This provides clear separation between WhatsApp and application database configuration.
func parseCommandLineFlags(config Config) Flags {
	flags := Flags{
		qrOutput:      flag.String("qr-output", "", "path to write login QR code"),
		numeric:       flag.Bool("numeric-code", false, "use numeric login code instead of QR code"),
		stateDir:      flag.String("state-dir", config.StateDir, "state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)"),
		whatsappDBDSN: flag.String("whatsapp-db-dsn", config.WhatsAppDBDSN, "WhatsApp/whatsmeow database connection string (overrides $WHATSAPP_DB_DSN)"),
		appDBDSN:      flag.String("app-db-dsn", config.ApplicationDBDSN, "application database connection string for receipts/responses/flow state (overrides $DATABASE_DSN or $DATABASE_URL)"),
		openaiKey:     flag.String("openai-api-key", config.OpenAIKey, "OpenAI API key (overrides $OPENAI_API_KEY)"),
		apiAddr:       flag.String("api-addr", config.APIAddr, "API server address (overrides $API_ADDR)"),
		defaultCron:   flag.String("default-cron", config.DefaultCron, "default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)"),
	}

	flag.Parse()

	slog.Debug("flags parsed",
		"qrOutput", *flags.qrOutput,
		"numeric", *flags.numeric,
		"stateDir", *flags.stateDir,
		"whatsappDBDSN_set", *flags.whatsappDBDSN != "",
		"appDBDSN_set", *flags.appDBDSN != "",
		"openaiKeySet", *flags.openaiKey != "",
		"apiAddr", *flags.apiAddr,
		"defaultCron", *flags.defaultCron)

	// Update database DSNs if not explicitly set but state directory has changed
	if *flags.whatsappDBDSN == config.WhatsAppDBDSN && config.WhatsAppDBDSN == "file:"+filepath.Join(config.StateDir, DefaultWhatsAppDBFileName)+"?_foreign_keys=on" && *flags.stateDir != config.StateDir {
		*flags.whatsappDBDSN = "file:" + filepath.Join(*flags.stateDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
		slog.Debug("Updated WhatsApp database DSN based on state directory", "old_state_dir", config.StateDir, "new_state_dir", *flags.stateDir)
	}

	if *flags.appDBDSN == config.ApplicationDBDSN && config.ApplicationDBDSN == "file:"+filepath.Join(config.StateDir, DefaultAppDBFileName)+"?_foreign_keys=on" && *flags.stateDir != config.StateDir {
		*flags.appDBDSN = "file:" + filepath.Join(*flags.stateDir, DefaultAppDBFileName) + "?_foreign_keys=on"
		slog.Debug("Updated application database DSN based on state directory", "old_state_dir", config.StateDir, "new_state_dir", *flags.stateDir)
	}

	return flags
}

// ensureDirectoriesExist creates necessary directories for file-based storage.
// This handles both WhatsApp and application database directory creation.
func ensureDirectoriesExist(flags Flags) error {
	// Ensure directories exist for both database files if they're file-based
	dirsToCreate := make(map[string]bool)

	// Collect DSNs
	dbDSNs := []string{*flags.whatsappDBDSN, *flags.appDBDSN}

	for _, dsn := range dbDSNs {
		if store.DetectDSNType(dsn) == "sqlite3" {
			// Extract file path from DSN, handling file:// URI scheme
			dbPath := dsn
			if strings.HasPrefix(dbPath, "file:") {
				if parsedURL, err := url.Parse(dbPath); err == nil {
					dbPath = parsedURL.Path
				}
				// If parsing fails, continue with original path
			}

			// Add directory to creation list if it's not current directory
			if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
				dirsToCreate[dir] = true
			}
		}
	}

	// Create directories
	for dir := range dirsToCreate {
		slog.Debug("Creating directory for file-based database", "dir", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("Failed to create directory", "error", err, "dir", dir)
			// fallback to temporary directory if creation fails
			tempDir, terr := os.MkdirTemp("", "promptpipe_state_")
			if terr != nil {
				slog.Error("Failed to create temporary directory", "error", terr)
				return err
			}
			slog.Warn("Falling back to temporary directory", "temp_dir", tempDir)
			*flags.stateDir = tempDir
			*flags.whatsappDBDSN = "file:" + filepath.Join(tempDir, DefaultWhatsAppDBFileName) + "?_foreign_keys=on"
			*flags.appDBDSN = "file:" + filepath.Join(tempDir, DefaultAppDBFileName) + "?_foreign_keys=on"
		} else {
			slog.Debug("Directory created successfully", "dir", dir)
		}
	}
	return nil
}

// buildWhatsAppOptions constructs WhatsApp configuration options.
// This configures the WhatsApp/whatsmeow database connection.
func buildWhatsAppOptions(flags Flags) []whatsapp.Option {
	var waOpts []whatsapp.Option
	if *flags.qrOutput != "" {
		waOpts = append(waOpts, whatsapp.WithQRCodeOutput(*flags.qrOutput))
	}
	if *flags.numeric {
		waOpts = append(waOpts, whatsapp.WithNumericCode())
	}
	if *flags.whatsappDBDSN != "" {
		waOpts = append(waOpts, whatsapp.WithDBDSN(*flags.whatsappDBDSN))
	}
	return waOpts
}

// buildStoreOptions constructs store configuration options.
// This configures the application database connection for receipts, responses, and flow state.
func buildStoreOptions(flags Flags) []store.Option {
	var storeOpts []store.Option
	if *flags.appDBDSN != "" {
		// Check if it's a PostgreSQL DSN using the shared detection function
		if store.DetectDSNType(*flags.appDBDSN) == "postgres" {
			slog.Debug("Detected PostgreSQL DSN, configuring PostgreSQL store", "dsn_type", "postgresql", "dsn_set", true)
			storeOpts = append(storeOpts, store.WithPostgresDSN(*flags.appDBDSN))
		} else {
			// Assume SQLite for file paths
			slog.Debug("Detected SQLite DSN, configuring SQLite store", "dsn_type", "sqlite", "db_path", *flags.appDBDSN)
			storeOpts = append(storeOpts, store.WithSQLiteDSN(*flags.appDBDSN))
		}
	} else {
		slog.Debug("No application database DSN provided, will use in-memory store")
	}
	return storeOpts
}

// buildGenAIOptions constructs GenAI configuration options
func buildGenAIOptions(flags Flags) []genai.Option {
	var genaiOpts []genai.Option
	if *flags.openaiKey != "" {
		genaiOpts = append(genaiOpts, genai.WithAPIKey(*flags.openaiKey))
	}
	return genaiOpts
}

// buildAPIOptions constructs API server configuration options
func buildAPIOptions(flags Flags) []api.Option {
	var apiOpts []api.Option
	if *flags.apiAddr != "" {
		apiOpts = append(apiOpts, api.WithAddr(*flags.apiAddr))
	}
	if *flags.defaultCron != "" {
		apiOpts = append(apiOpts, api.WithDefaultCron(*flags.defaultCron))
	}
	return apiOpts
}
