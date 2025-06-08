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

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Debug("failed to load .env file", "error", err)
	} else {
		slog.Debug("successfully loaded .env file")
	}
	// Read environment variables
	envDbDriver := os.Getenv("WHATSAPP_DB_DRIVER")
	envWhatsAppDSN := os.Getenv("WHATSAPP_DB_DSN")
	envDatabaseURL := os.Getenv("DATABASE_URL")
	envStateDir := os.Getenv("PROMPTPIPE_STATE_DIR")
	if envStateDir == "" {
		envStateDir = "/var/lib/promptpipe"
		slog.Debug("No PROMPTPIPE_STATE_DIR set, using default", "default_state_dir", envStateDir)
	} else {
		slog.Debug("PROMPTPIPE_STATE_DIR found in environment", "state_dir", envStateDir)
	}
	slog.Debug("environment variables loaded", "WHATSAPP_DB_DRIVER", envDbDriver, "WHATSAPP_DB_DSN", envWhatsAppDSN, "DATABASE_URL", envDatabaseURL, "PROMPTPIPE_STATE_DIR", envStateDir)
	// Default to WhatsApp DSN if specific not set
	if envWhatsAppDSN == "" {
		envWhatsAppDSN = envDatabaseURL
		if envDatabaseURL != "" {
			slog.Debug("Using DATABASE_URL as WHATSAPP_DB_DSN", "dsn_set", true)
		}
	}
	// If no database URL is provided, default to SQLite in the state directory
	if envWhatsAppDSN == "" {
		envWhatsAppDSN = filepath.Join(envStateDir, "promptpipe.db")
		slog.Debug("No database DSN provided, defaulting to SQLite", "sqlite_path", envWhatsAppDSN)
	}
	envOpenAIKey := os.Getenv("OPENAI_API_KEY")
	envAPIAddr := os.Getenv("API_ADDR")
	envDefaultCron := os.Getenv("DEFAULT_SCHEDULE")
	slog.Debug("additional environment variables", "OPENAI_API_KEY_SET", envOpenAIKey != "", "API_ADDR", envAPIAddr, "DEFAULT_SCHEDULE", envDefaultCron)

	// Command-line options (flags) with environment defaults
	qrOutput := flag.String("qr-output", "", "path to write login QR code")
	numeric := flag.Bool("numeric-code", false, "use numeric login code instead of QR code")

	stateDir := flag.String("state-dir", envStateDir, "state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)")
	dbDriver := flag.String("db-driver", envDbDriver, "database driver for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DRIVER)")
	dbDSN := flag.String("db-dsn", envWhatsAppDSN, "database DSN for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DSN or $DATABASE_URL)")

	openaiKey := flag.String("openai-api-key", envOpenAIKey, "OpenAI API key (overrides $OPENAI_API_KEY)")

	apiAddr := flag.String("api-addr", envAPIAddr, "API server address (overrides $API_ADDR)")
	defaultCron := flag.String("default-cron", envDefaultCron, "default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)")
	flag.Parse()
	slog.Debug("flags parsed", "qrOutput", *qrOutput, "numeric", *numeric, "stateDir", *stateDir, "dbDriver", *dbDriver, "dbDSN", *dbDSN, "openaiKeySet", *openaiKey != "", "apiAddr", *apiAddr, "defaultCron", *defaultCron)

	// Update database DSN if not explicitly set but state directory is provided
	if *dbDSN == envWhatsAppDSN && envWhatsAppDSN == filepath.Join(envStateDir, "promptpipe.db") && *stateDir != envStateDir {
		*dbDSN = filepath.Join(*stateDir, "promptpipe.db")
		slog.Debug("Updated dbDSN based on state directory", "newDBDSN", *dbDSN, "old_state_dir", envStateDir, "new_state_dir", *stateDir)
	}

	// Ensure state directory exists if we're using a file-based DSN
	if !strings.Contains(*dbDSN, "postgres://") && !strings.Contains(*dbDSN, "host=") {
		stateDir := filepath.Dir(*dbDSN)
		slog.Debug("Creating state directory for file-based database", "state_dir", stateDir, "db_path", *dbDSN)
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			slog.Error("Failed to create state directory", "error", err, "state_dir", stateDir)
		} else {
			slog.Debug("State directory created successfully", "state_dir", stateDir)
		}
	}

	// Build WhatsApp options
	var waOpts []whatsapp.Option
	if *qrOutput != "" {
		waOpts = append(waOpts, whatsapp.WithQRCodeOutput(*qrOutput))
	}
	if *numeric {
		waOpts = append(waOpts, whatsapp.WithNumericCode())
	}
	if *dbDriver != "" {
		waOpts = append(waOpts, whatsapp.WithDBDriver(*dbDriver))
	}
	if *dbDSN != "" {
		waOpts = append(waOpts, whatsapp.WithDBDSN(*dbDSN))
	}

	// Build store options
	var storeOpts []store.Option
	if *dbDSN != "" {
		// Check if it's a PostgreSQL DSN using the shared detection function
		if store.DetectDSNType(*dbDSN) == "postgres" {
			slog.Debug("Detected PostgreSQL DSN, configuring PostgreSQL store", "dsn_type", "postgresql", "dsn_set", true)
			storeOpts = append(storeOpts, store.WithPostgresDSN(*dbDSN))
		} else {
			// Assume SQLite for file paths
			slog.Debug("Detected SQLite DSN, configuring SQLite store", "dsn_type", "sqlite", "db_path", *dbDSN)
			storeOpts = append(storeOpts, store.WithSQLiteDSN(*dbDSN))
		}
	} else {
		slog.Debug("No database DSN provided, will use in-memory store")
	}

	// Build GenAI options
	var genaiOpts []genai.Option
	if *openaiKey != "" {
		genaiOpts = append(genaiOpts, genai.WithAPIKey(*openaiKey))
	}

	// Build API server options
	var apiOpts []api.Option
	if *apiAddr != "" {
		apiOpts = append(apiOpts, api.WithAddr(*apiAddr))
	}
	if *defaultCron != "" {
		apiOpts = append(apiOpts, api.WithDefaultCron(*defaultCron))
	}

	// Start the service
	slog.Info("Bootstrapping PromptPipe with configured modules")
	slog.Debug("Module options counts", "whatsapp", len(waOpts), "store", len(storeOpts), "genai", len(genaiOpts), "api", len(apiOpts))
	slog.Debug("Final configuration", "state_dir", *stateDir, "db_dsn", *dbDSN, "api_addr", *apiAddr)
	api.Run(waOpts, storeOpts, genaiOpts, apiOpts)
	slog.Info("PromptPipe exited")
}
