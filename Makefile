build:
	mkdir -p build
	go build -o build/promptpipe cmd/PromptPipe/main.go

test:
	go test ./...

test-postgres:
	@echo "Run with: POSTGRES_DSN_TEST='postgres://user:pass@host:5432/dbname?sslmode=disable' make test-postgres"
	POSTGRES_DSN_TEST="$${POSTGRES_DSN_TEST}" go test ./internal/store/... -v -count=1

check-docs:
	@bash scripts/check-env-docs.sh

.PHONY: build test test-postgres check-docs
