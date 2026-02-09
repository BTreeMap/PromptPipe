# Codebase Cleanup Report (2026-02-09)

## Summary

This report documents the cleanup work performed alongside the tone feature implementation.

## Dead Logic Removed

None. The `prompt_generator_module.go` tombstone file was evaluated and intentionally retained — it serves as a build-failure marker for stale references.

## Consolidations Made

### Profile Helper Consolidation

**Problem:** Three modules (`FeedbackModule`, `SchedulerTool`, `PromptGeneratorTool`) each had private copies of `getUserProfile()` and `saveUserProfile()` methods that performed the same JSON marshal/unmarshal operations against `StateManager`.

**Solution:** Extracted shared package-level helpers into `internal/flow/profile_helpers.go`:
- `loadUserProfile()` — retrieves and unmarshals a profile; returns error when absent.
- `loadOrCreateUserProfile()` — same as above but returns a default empty profile on absence.
- `persistUserProfile()` — marshals and saves a profile.

Each module's method now delegates to these shared helpers, reducing ~80 lines of duplicate code to ~10 lines. Behavior is preserved: `PromptGeneratorTool` still returns an error for missing profiles (intentional), while `FeedbackModule` and `SchedulerTool` create defaults.

### Unused Import Cleanup

Removed `encoding/json` import from `prompt_generator_tool.go` after the refactoring made it unused.

## Key Maintainability Improvements

1. **gofmt applied** to `internal/tone/tone.go` (was the only file out of format).
2. **`go mod tidy`** verified — no unused dependencies found.
3. **`go vet`** passes cleanly across the entire codebase.
4. **All tests pass** (`go test ./...`).

## Behavior Changes

None. All refactoring preserved existing behavior. The profile helper consolidation was a pure DRY improvement with identical semantics per module.

## Follow-ups Not Done (and Why)

- **Coordinator static module duplication**: `coordinator_module.go` and `coordinator_module_static.go` have similar structures but serve different purposes (LLM-based vs. rule-based routing). Consolidating them would require more invasive changes without clear benefit.
- **Legacy/outdated docs**: Several docs are marked as legacy or informational. Removing them would lose historical context without improving the active codebase.
- **Python/LangChain code**: The `python/` directory contains non-integrated utilities. Cleanup was deferred as it is outside the Go codebase scope.
- **Additional linting**: No `golangci-lint` config exists in the repo. Adding one was considered out of scope for this cleanup pass.
