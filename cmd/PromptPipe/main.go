package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/lockfile"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"github.com/joho/godotenv"
)

// Default configuration constants
const (
	// DefaultStateDir is the default directory for PromptPipe state data
	DefaultStateDir = "/var/lib/promptpipe"
	// DefaultAppDBFileName is the default SQLite database filename for application data
	DefaultAppDBFileName = "state.db"
	// DefaultWhatsAppDBFileName is the default SQLite database filename for WhatsApp/whatsmeow data
	DefaultWhatsAppDBFileName = "whatsmeow.db"
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

	// Acquire lock on state directory to prevent multiple instances
	lock, err := lockfile.AcquireLock(*flags.stateDir)
	if err != nil {
		if lockErr, ok := err.(*lockfile.LockError); ok {
			// Print user-friendly error message
			slog.Error("Cannot start PromptPipe", "reason", "state directory already in use")
			slog.Error(lockErr.Error())
		} else {
			slog.Error("Failed to acquire state directory lock", "error", err)
		}
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown with lock cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Set up a cleanup function that releases the lock
	cleanup := func() {
		slog.Info("Releasing state directory lock")
		if err := lock.Release(); err != nil {
			slog.Error("Failed to release lock during cleanup", "error", err)
		}
	}

	// Ensure lock is released on exit
	defer cleanup()

	// Handle signals in a goroutine
	go func() {
		sig := <-signalChan
		slog.Info("Received shutdown signal", "signal", sig)
		cleanup()
		os.Exit(0)
	}()

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

	// DebugMode enables debug logging of API calls.
	// Environment variable: PROMPTPIPE_DEBUG
	DebugMode bool

	// IntakeBotPromptFile is the path to the intake bot system prompt file
	// Environment variable: INTAKE_BOT_PROMPT_FILE
	IntakeBotPromptFile string

	// PromptGeneratorPromptFile is the path to the prompt generator system prompt file
	// Environment variable: PROMPT_GENERATOR_PROMPT_FILE
	PromptGeneratorPromptFile string

	// FeedbackTrackerPromptFile is the path to the feedback tracker system prompt file
	// Environment variable: FEEDBACK_TRACKER_PROMPT_FILE
	FeedbackTrackerPromptFile string

	// ChatHistoryLimit limits the number of history messages sent to bot tools.
	// -1: no limit, 0: no history, positive: limit to last N messages
	// Environment variable: CHAT_HISTORY_LIMIT
	ChatHistoryLimit int
}

// Flags holds command line flag values for database and other configuration.
// This provides clear separation between WhatsApp and application database settings.
type Flags struct {
	qrOutput                  *string
	numeric                   *bool
	stateDir                  *string
	whatsappDBDSN             *string // WhatsApp/whatsmeow database connection string
	appDBDSN                  *string // Application database connection string
	openaiKey                 *string
	apiAddr                   *string
	defaultCron               *string
	debug                     *bool   // Enable debug mode for API call logging
	intakeBotPromptFile       *string // Path to intake bot system prompt file
	promptGeneratorPromptFile *string // Path to prompt generator system prompt file
	feedbackTrackerPromptFile *string // Path to feedback tracker system prompt file
	chatHistoryLimit          *int    // Limit for number of history messages sent to bot tools
}

// initializeLogger sets up structured logging with debug level
func initializeLogger() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
}

// loadEnvFile searches for and loads .env files from multiple possible locations.
// This allows the binary to work correctly regardless of the execution directory.
func loadEnvFile() {
	// Search for .env files in order of priority
	envFiles := []string{".env", "../.env", "../../.env"}

	for _, envFile := range envFiles {
		if err := godotenv.Load(envFile); err == nil {
			slog.Debug("Successfully loaded .env file", "file", envFile)
			return
		}
	}

	slog.Debug("No .env file found in any of the search locations", "locations", envFiles)
}

// loadEnvironmentConfig loads configuration from environment variables and .env file.
// This separates WhatsApp database configuration from application database configuration.
// It searches for .env files in multiple locations to handle different execution directories.
func loadEnvironmentConfig() Config {
	loadEnvFile()

	config := Config{
		WhatsAppDBDSN:             os.Getenv("WHATSAPP_DB_DSN"),
		ApplicationDBDSN:          os.Getenv("DATABASE_DSN"),
		StateDir:                  os.Getenv("PROMPTPIPE_STATE_DIR"),
		OpenAIKey:                 os.Getenv("OPENAI_API_KEY"),
		APIAddr:                   os.Getenv("API_ADDR"),
		DefaultCron:               os.Getenv("DEFAULT_SCHEDULE"),
		DebugMode:                 parseBoolEnv("PROMPTPIPE_DEBUG", false),
		IntakeBotPromptFile:       getEnvWithDefault("INTAKE_BOT_PROMPT_FILE", "prompts/intake_bot_system.txt"),
		PromptGeneratorPromptFile: getEnvWithDefault("PROMPT_GENERATOR_PROMPT_FILE", "prompts/prompt_generator_system.txt"),
		FeedbackTrackerPromptFile: getEnvWithDefault("FEEDBACK_TRACKER_PROMPT_FILE", "prompts/feedback_tracker_system.txt"),
		ChatHistoryLimit:          parseIntEnv("CHAT_HISTORY_LIMIT", -1),
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
		"DEFAULT_SCHEDULE", config.DefaultCron,
		"PROMPTPIPE_DEBUG", config.DebugMode)

	return config
}

// parseCommandLineFlags parses command line arguments with environment defaults.
// This provides clear separation between WhatsApp and application database configuration.
func parseCommandLineFlags(config Config) Flags {
	flags := Flags{
		qrOutput:                  flag.String("qr-output", "", "path to write login QR code"),
		numeric:                   flag.Bool("numeric-code", false, "use numeric login code instead of QR code"),
		stateDir:                  flag.String("state-dir", config.StateDir, "state directory for PromptPipe data (overrides $PROMPTPIPE_STATE_DIR)"),
		whatsappDBDSN:             flag.String("whatsapp-db-dsn", config.WhatsAppDBDSN, "WhatsApp/whatsmeow database connection string (overrides $WHATSAPP_DB_DSN)"),
		appDBDSN:                  flag.String("app-db-dsn", config.ApplicationDBDSN, "application database connection string for receipts/responses/flow state (overrides $DATABASE_DSN or $DATABASE_URL)"),
		openaiKey:                 flag.String("openai-api-key", config.OpenAIKey, "OpenAI API key (overrides $OPENAI_API_KEY)"),
		apiAddr:                   flag.String("api-addr", config.APIAddr, "API server address (overrides $API_ADDR)"),
		defaultCron:               flag.String("default-cron", config.DefaultCron, "default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)"),
		debug:                     flag.Bool("debug", config.DebugMode, "enable debug mode for API call logging (overrides $PROMPTPIPE_DEBUG)"),
		intakeBotPromptFile:       flag.String("intake-bot-prompt-file", config.IntakeBotPromptFile, "path to intake bot system prompt file (overrides $INTAKE_BOT_PROMPT_FILE)"),
		promptGeneratorPromptFile: flag.String("prompt-generator-prompt-file", config.PromptGeneratorPromptFile, "path to prompt generator system prompt file (overrides $PROMPT_GENERATOR_PROMPT_FILE)"),
		feedbackTrackerPromptFile: flag.String("feedback-tracker-prompt-file", config.FeedbackTrackerPromptFile, "path to feedback tracker system prompt file (overrides $FEEDBACK_TRACKER_PROMPT_FILE)"),
		chatHistoryLimit:          flag.Int("chat-history-limit", config.ChatHistoryLimit, "limit for number of history messages sent to bot tools: -1=no limit, 0=no history, positive=limit to last N messages (overrides $CHAT_HISTORY_LIMIT)"),
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
		"defaultCron", *flags.defaultCron,
		"debug", *flags.debug,
		"intakeBotPromptFile", *flags.intakeBotPromptFile,
		"promptGeneratorPromptFile", *flags.promptGeneratorPromptFile,
		"feedbackTrackerPromptFile", *flags.feedbackTrackerPromptFile,
		"chatHistoryLimit", *flags.chatHistoryLimit)

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
// This handles state directory creation (required for lockfile) and database directory creation.
func ensureDirectoriesExist(flags Flags) error {
	// Always ensure state directory exists (required for lockfile)
	dirsToCreate := make(map[string]bool)
	dirsToCreate[*flags.stateDir] = true

	// Collect DSNs and add their directories if they're file-based
	dbDSNs := []string{*flags.whatsappDBDSN, *flags.appDBDSN}

	for _, dsn := range dbDSNs {
		// Only process SQLite DSNs
		if store.DetectDSNType(dsn) == "sqlite3" {
			if dir, err := store.ExtractDirFromSQLiteDSN(dsn); err != nil {
				slog.Error("Failed to extract directory from SQLite DSN", "error", err, "dsn", dsn)
				return err
			} else if dir != "" {
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
	if *flags.debug {
		genaiOpts = append(genaiOpts, genai.WithDebugMode(true))
		genaiOpts = append(genaiOpts, genai.WithStateDir(*flags.stateDir))
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
	if *flags.intakeBotPromptFile != "" {
		apiOpts = append(apiOpts, api.WithIntakeBotPromptFile(*flags.intakeBotPromptFile))
	}
	if *flags.promptGeneratorPromptFile != "" {
		apiOpts = append(apiOpts, api.WithPromptGeneratorPromptFile(*flags.promptGeneratorPromptFile))
	}
	if *flags.feedbackTrackerPromptFile != "" {
		apiOpts = append(apiOpts, api.WithFeedbackTrackerPromptFile(*flags.feedbackTrackerPromptFile))
	}
	// Always pass the chat history limit since it has a meaningful default
	apiOpts = append(apiOpts, api.WithChatHistoryLimit(*flags.chatHistoryLimit))
	return apiOpts
}

// getEnvWithDefault gets an environment variable value or returns a default
func getEnvWithDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// parseBoolEnv parses a boolean environment variable with a default value.
// It accepts "true", "1", "yes", "on" (case insensitive) as true values.
func parseBoolEnv(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}

	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		slog.Warn("Invalid boolean value for environment variable", "key", key, "value", val, "defaulting_to", defaultValue)
		return defaultValue
	}
}

// parseIntEnv parses an integer environment variable with a default value.
// It accepts positive integers, 0, and negative integers.
func parseIntEnv(key string, defaultValue int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}

	intVal, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("Invalid integer value for environment variable", "key", key, "value", val, "defaulting_to", defaultValue)
		return defaultValue
	}

	return intVal
}
