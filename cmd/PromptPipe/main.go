package main

import (
	"github.com/BTreeMap/PromptPipe/internal/api"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}
	api.Run()
}
