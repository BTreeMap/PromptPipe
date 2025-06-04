package main

import (
	"flag"
	"log"
	"os"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("DEBUG: failed to load .env file: %v", err)
	}
	// Read environment variables
	envDbDriver := os.Getenv("WHATSAPP_DB_DRIVER")
	envWhatsAppDSN := os.Getenv("WHATSAPP_DB_DSN")
	envDatabaseURL := os.Getenv("DATABASE_URL")
	// Default to WhatsApp DSN if specific not set
	if envWhatsAppDSN == "" {
		envWhatsAppDSN = envDatabaseURL
	}
	envOpenAIKey := os.Getenv("OPENAI_API_KEY")
	envAPIAddr := os.Getenv("API_ADDR")
	envDefaultCron := os.Getenv("DEFAULT_SCHEDULE")

	// Command-line options (flags) with environment defaults
	qrOutput := flag.String("qr-output", "", "path to write login QR code")
	numeric := flag.Bool("numeric-code", false, "use numeric login code instead of QR code")

	dbDriver := flag.String("db-driver", envDbDriver, "database driver for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DRIVER)")
	dbDSN := flag.String("db-dsn", envWhatsAppDSN, "database DSN for WhatsApp and Postgres store (overrides $WHATSAPP_DB_DSN or $DATABASE_URL)")

	openaiKey := flag.String("openai-api-key", envOpenAIKey, "OpenAI API key (overrides $OPENAI_API_KEY)")

	apiAddr := flag.String("api-addr", envAPIAddr, "API server address (overrides $API_ADDR)")
	defaultCron := flag.String("default-cron", envDefaultCron, "default cron schedule for prompts (overrides $DEFAULT_SCHEDULE)")
	flag.Parse()

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
		storeOpts = append(storeOpts, store.WithPostgresDSN(*dbDSN))
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
	api.Run(waOpts, storeOpts, genaiOpts, apiOpts)
}
