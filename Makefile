build:
	mkdir -p build
	go build -o build/PromptPipe cmd/PromptPipe/main.go

test:
	go test ./...

.PHONY: build test
