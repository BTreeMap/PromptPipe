package main

import (
	"flag"
	"log"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/store"
	"github.com/BTreeMap/PromptPipe/internal/whatsapp"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	// Command-line options for WhatsApp client
	qrOutput := flag.String("qr-output", "", "path to write login QR code")
	numeric := flag.Bool("numeric-code", false, "use numeric login code instead of QR code")

	// Command-line options for database configuration
	dbDriver := flag.String("db-driver", "", "database driver for WhatsApp and Postgres store (overrides env)")
	dbDSN := flag.String("db-dsn", "", "database DSN for WhatsApp and Postgres store (overrides env)")

	// Command-line option for OpenAI API key
	openaiKey := flag.String("openai-api-key", "", "OpenAI API key (overrides env)")

	// Command-line option for API server address
	apiAddr := flag.String("api-addr", "", "API server address (overrides $API_ADDR)")
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

	// Call API run with module options
	api.Run(waOpts, storeOpts, genaiOpts, apiOpts)
}
