#!/usr/bin/env bash
# check-env-docs.sh — verify that every environment variable read in
# cmd/PromptPipe/main.go is documented in .env.example and docs/configuration.md.
#
# Usage:  ./scripts/check-env-docs.sh          (run from repo root)
#         make check-docs                       (via Makefile target)
#
# Exit code 0 = all env vars are documented.  Non-zero = drift detected.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAIN_GO="$REPO_ROOT/cmd/PromptPipe/main.go"
ENV_EXAMPLE="$REPO_ROOT/.env.example"
CONFIG_DOC="$REPO_ROOT/docs/configuration.md"

errors=0

# Extract environment variable names from main.go.
# Matches os.Getenv("VAR"), ParseBoolEnv("VAR", ...), ParseIntEnv("VAR", ...),
# ParseFloatEnv("VAR", ...), and GetEnvWithDefault("VAR", ...).
env_vars=$(grep -oP '(?:os\.Getenv|ParseBoolEnv|ParseIntEnv|ParseFloatEnv|GetEnvWithDefault)\(\s*"([A-Z_]+)"' "$MAIN_GO" \
    | grep -oP '"[A-Z_]+"' \
    | tr -d '"' \
    | sort -u)

echo "Env vars found in $MAIN_GO:"
echo "$env_vars"
echo ""

for var in $env_vars; do
    # Check .env.example
    if ! grep -q "$var" "$ENV_EXAMPLE" 2>/dev/null; then
        echo "DRIFT: $var is used in main.go but missing from .env.example"
        errors=$((errors + 1))
    fi
    # Check docs/configuration.md
    if ! grep -q "$var" "$CONFIG_DOC" 2>/dev/null; then
        echo "DRIFT: $var is used in main.go but missing from docs/configuration.md"
        errors=$((errors + 1))
    fi
done

if [ "$errors" -gt 0 ]; then
    echo ""
    echo "❌ Found $errors documentation drift issue(s)."
    echo "   Update .env.example and/or docs/configuration.md to match main.go."
    exit 1
fi

echo "✅ All environment variables are documented."
