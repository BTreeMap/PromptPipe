package main

import (
	"flag"
	"log/slog"
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
	// DefaultDBFileName is the default SQLite database filename
	DefaultDBFileName = "promptpipe.db"
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
	slog.Debug("Final configuration", "state_dir", *flags.stateDir, "dsn_set", *flags.dbDSN != "", "api_addr", *flags.apiAddr)
	if err := api.Run(waOpts, storeOpts, genaiOpts, apiOpts); err != nil {
		slog.Error("PromptPipe failed to run", "error", err)
		os.Exit(1)
	}
	slog.Info("PromptPipe exited successfully")
}

// Config holds environment configuration
type Config struct {
	DbDriver    string
	WhatsAppDSN string
	DatabaseURL string
	StateDir    string
	OpenAIKey   string
	APIAddr     string
	DefaultCron string
}

// Flags holds command line flag values
type Flags struct {
	qrOutput    *string
	numeric     *bool
	stateDir    *string
	dbDriver    *string
	dbDSN       *string
	openaiKey   *string
	apiAddr     *string
	defaultCron *string
}

// initializeLogger sets up structured logging with debug level
func initializeLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
}

// loadEnvironmentConfig loads configuration from environment variables and .env file
func loadEnvironmentConfig() Config {
	if err := godotenv.Load(); err != nil {
		slog.Debug("failed to load .env file", "error", err)
	} else {
		slog.Debug("successfully loaded .env file")
	}

	config := Config{
		DbDriver:    os.Getenv("WHATSAPP_DB_DRIVER"),
		WhatsAppDSN: os.Getenv("WHATSAPP_DB_DSN"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		StateDir:    os.Getenv("PROMPTPIPE_STATE_DIR"),
		OpenAIKey:   os.Getenv("OPENAI_API_KEY"),
		APIAddr:     os.Getenv("API_ADDR"),
		DefaultCron: os.Getenv("DEFAULT_SCHEDULE"),
	}

	// Set default state directory if not specified
	if config.StateDir == "" {
		config.StateDir = DefaultStateDir
		slog.Debug("No PROMPTPIPE_STATE_DIR set, using default", "default_state_dir", config.StateDir)
	} else {
		slog.Debug("PROMPTPIPE_STATE_DIR found in environment", "state_dir", config.StateDir)
	}

	// Default to WhatsApp DSN if specific not set
	if config.WhatsAppDSN == "" {
		config.WhatsAppDSN = config.DatabaseURL
		if config.DatabaseURL != "" {
			slog.Debug("Using DATABASE_URL as WHATSAPP_DB_DSN", "dsn_set", true)
		}
	}

	// If no database URL is provided, default to SQLite in the state directory
	if config.WhatsAppDSN == "" {
		config.WhatsAppDSN = filepath.Join(config.StateDir, DefaultDBFileName)
		slog.Debug("No database DSN provided, defaulting to SQLite", "sqlite_path", config.WhatsAppDSN)
	}

	slog.Debug("environment variables loaded",
		"WHATSAPP_DB_DRIVER", config.DbDriver,
		"WHATSAPP_DB_DSN_SET", config.WhatsAppDSN != "",
		"DATABASE_URL_SET", config.DatabaseURL != "",
		"PROMPTPIPE_STATE_DIR", config.StateDir,
		"OPENAI_API_KEY_SET", config.OpenAIKey != "",
		"API_ADDR", config.APIAddr,
		"DEFAULT_SCHEDULE", config.DefaultCron)

	return config
}

// parseCommandLineFlags parses command line arguments with environment defaults
func parseCommandLineFlags(config Config) Flags {
	flags := Flags{
		qrOutput:    flag.String("qr-output", "", "path to write login QR code"),
		numeric:     flag.Bool("numeric-code", false, "use numeric login code instead of QR code"),
		stateDir:    flag.String("state-dir", config.StateDir, "state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)"),
		dbDriver:    flag.String("db-driver", config.DbDriver, "database driver for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DRIVER)"),
		dbDSN:       flag.String("db-dsn", config.WhatsAppDSN, "database DSN for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DSN or $DATABASE_URL)"),
		openaiKey:   flag.String("openai-api-key", config.OpenAIKey, "OpenAI API key (overrides $OPENAI_API_KEY)"),
		apiAddr:     flag.String("api-addr", config.APIAddr, "API server address (overrides $API_ADDR)"),
		defaultCron: flag.String("default-cron", config.DefaultCron, "default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)"),
	}

	flag.Parse()

	slog.Debug("flags parsed",
		"qrOutput", *flags.qrOutput,
		"numeric", *flags.numeric,
		"stateDir", *flags.stateDir,
		"dbDriver", *flags.dbDriver,
		"dbDSN_set", *flags.dbDSN != "",
		"openaiKeySet", *flags.openaiKey != "",
		"apiAddr", *flags.apiAddr,
		"defaultCron", *flags.defaultCron)

	// Update database DSN if not explicitly set but state directory is provided
	if *flags.dbDSN == config.WhatsAppDSN && config.WhatsAppDSN == filepath.Join(config.StateDir, DefaultDBFileName) && *flags.stateDir != config.StateDir {
		*flags.dbDSN = filepath.Join(*flags.stateDir, DefaultDBFileName)
		slog.Debug("Updated dbDSN based on state directory", "dsn_updated", true, "old_state_dir", config.StateDir, "new_state_dir", *flags.stateDir)
	}

	return flags
}

// ensureDirectoriesExist creates necessary directories for file-based storage
func ensureDirectoriesExist(flags Flags) error {
	// Ensure state directory exists if we're using a file-based DSN
	if !strings.Contains(*flags.dbDSN, "postgres://") && !strings.Contains(*flags.dbDSN, "host=") {
		stateDir := filepath.Dir(*flags.dbDSN)
		slog.Debug("Creating state directory for file-based database", "state_dir", stateDir)
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			slog.Error("Failed to create state directory", "error", err, "state_dir", stateDir)
			return err
		}
		slog.Debug("State directory created successfully", "state_dir", stateDir)
	}
	return nil
}

// buildWhatsAppOptions constructs WhatsApp configuration options
func buildWhatsAppOptions(flags Flags) []whatsapp.Option {
	var waOpts []whatsapp.Option
	if *flags.qrOutput != "" {
		waOpts = append(waOpts, whatsapp.WithQRCodeOutput(*flags.qrOutput))
	}
	if *flags.numeric {
		waOpts = append(waOpts, whatsapp.WithNumericCode())
	}
	if *flags.dbDriver != "" {
		waOpts = append(waOpts, whatsapp.WithDBDriver(*flags.dbDriver))
	}
	if *flags.dbDSN != "" {
		waOpts = append(waOpts, whatsapp.WithDBDSN(*flags.dbDSN))
	}
	return waOpts
}

// buildStoreOptions constructs store configuration options
func buildStoreOptions(flags Flags) []store.Option {
	var storeOpts []store.Option
	if *flags.dbDSN != "" {
		// Check if it's a PostgreSQL DSN using the shared detection function
		if store.DetectDSNType(*flags.dbDSN) == "postgres" {
			slog.Debug("Detected PostgreSQL DSN, configuring PostgreSQL store", "dsn_type", "postgresql", "dsn_set", true)
			storeOpts = append(storeOpts, store.WithPostgresDSN(*flags.dbDSN))
		} else {
			// Assume SQLite for file paths
			slog.Debug("Detected SQLite DSN, configuring SQLite store", "dsn_type", "sqlite", "db_path", *flags.dbDSN)
			storeOpts = append(storeOpts, store.WithSQLiteDSN(*flags.dbDSN))
		}
	} else {
		slog.Debug("No database DSN provided, will use in-memory store")
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
