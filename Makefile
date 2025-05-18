build:
	go build -o PromptPipe cmd/PromptPipe/main.go

test:
	go test ./...

.PHONY: build test
