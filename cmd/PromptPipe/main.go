package main

import (
	"log"

	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	api.Run()
}
