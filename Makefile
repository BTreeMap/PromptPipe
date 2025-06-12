build:
	mkdir -p build
	go build -o build/promptpipe cmd/PromptPipe/main.go

test:
	go test ./...

.PHONY: build test
