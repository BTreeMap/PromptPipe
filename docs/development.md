# Development

## Prerequisites

- Go **1.24.3+** (see `go.mod`)
- Make (optional)

## Build

```bash
make build
# or
go build -o build/promptpipe cmd/PromptPipe/main.go
```

## Test

```bash
go test ./...
go vet ./...
```

## Run locally

```bash
./build/promptpipe
```

The server loads `.env` files from the repository root or parent directories if present (see [Configuration](configuration.md)).
