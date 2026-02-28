# Development

## Prerequisites

- Go **1.24.3+** (see `go.mod`)
- Make (optional)
- Docker & Docker Compose (optional, for running with PostgreSQL)

## Build

```bash
make build
# or
go build -o build/promptpipe cmd/PromptPipe/main.go
```

## Test

```bash
make test
# or
go test ./...
```

Additional checks:

```bash
go vet ./...
gofmt -d .
```

To run tests against PostgreSQL (requires a running Postgres instance):

```bash
POSTGRES_DSN_TEST='postgres://user:pass@localhost:5432/dbname?sslmode=disable' make test-postgres
```

## Configure

Copy the example environment file and edit it:

```bash
cp .env.example .env
```

At minimum, set `OPENAI_API_KEY` if you want GenAI / conversation features.
See [`.env.example`](../.env.example) for all available variables and their defaults, or
[Configuration](configuration.md) for the full reference.

PromptPipe searches for `.env` files in `./.env`, `../.env`, and `../../.env` (first match wins).

## Run locally

### Standalone (SQLite)

```bash
./build/promptpipe
```

The API listens on `:8080` by default. Verify with:

```bash
curl http://localhost:8080/health
```

### With Docker Compose (PostgreSQL)

The repository includes a `docker-compose.yml` that starts PromptPipe alongside PostgreSQL 16:

```bash
docker compose up -d
```

This exposes the API on the host port defined by `API_PORT` (default `8080`).
To tear down while keeping data: `docker compose down`.
To also remove volumes: `docker compose down -v`.

## Verify documentation

A drift-check script ensures `.env.example` and `docs/configuration.md` stay in sync with the
environment variables actually read in `cmd/PromptPipe/main.go`:

```bash
make check-docs
```
