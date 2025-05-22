package main

import (
	"flag"
	"log"

	"github.com/BTreeMap/PromptPipe/internal/api"
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
	flag.Parse()

	// Build WhatsApp options
	var waOpts []whatsapp.Option
	if *qrOutput != "" {
		waOpts = append(waOpts, whatsapp.WithQRCodeOutput(*qrOutput))
	}
	if *numeric {
		waOpts = append(waOpts, whatsapp.WithNumericCode())
	}

	api.Run(waOpts...)
}
