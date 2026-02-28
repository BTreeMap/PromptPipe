## 2026-02-27 - Env var drift is the #1 documentation risk

**Learning:** All 19 environment variables in `cmd/PromptPipe/main.go` were already correctly documented in `docs/configuration.md`, but there was no `.env.example` and no automated check to prevent future drift. Env vars are added via `os.Getenv`, `ParseBoolEnv`, `ParseIntEnv`, `ParseFloatEnv`, and `GetEnvWithDefault` — all easily grep-able.

**Action:** Always run `make check-docs` (or `scripts/check-env-docs.sh`) after adding new env vars. The script extracts var names from main.go and verifies they appear in both `.env.example` and `docs/configuration.md`.
