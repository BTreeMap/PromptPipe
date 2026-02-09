# Docs Drift Fix Report

## What was wrong / outdated

- README and test docs referenced cron fields and response shapes that no longer match the API (the API expects a structured `schedule` object and wraps most responses).
- Conversation flow docs didn’t list newer state keys for intensity adjustment.
- Auto-enrollment docs implied a separate welcome bot message rather than the intake/feedback response path.
- Python/LangChain docs implied an active Go ↔ Python integration that is not present in the codebase.
- Legacy onboarding and architecture guides were presented without a clear “historical” disclaimer.

## What changed

- Rewrote `README.md` with an accurate overview and links to focused documentation.
- Added focused docs:
  - `docs/api.md` (endpoints, models, response envelopes)
  - `docs/configuration.md` (env vars, flags, defaults, precedence)
  - `docs/storage.md` (databases, schema, legacy tables)
  - `docs/flows.md` (flow generators and prompt validation)
  - `docs/conversation.md` (summary of conversation flow)
  - `docs/development.md` (build/test/run)
  - `docs/troubleshooting.md` (operational issues)
- Updated existing docs for accuracy:
  - `docs/conversation-flow.md` (added intensity state key + reminder notes)
  - `docs/debug-mode.md` (clarified structured thinking + debug output)
  - `docs/auto-enrollment-feature.md` and `docs/auto-enrollment-migration-guide.md`
  - `docs/state-persistence-recovery.md` (timer recovery behavior)
- Marked legacy or experimental docs explicitly:
  - `docs/beginner-guide.md`, `docs/onboarding.md`, `docs/conversation-flow-architecture.md`, `docs/conversation-flow-comprehensive.md`
  - `python/langchain/*` docs now state the Go service does not integrate with the Python agent
- Updated runnable examples:
  - `test-scripts/test-schedule.sh` now uses `schedule` objects
  - `test-scripts/README.md` updated for new schedule semantics
  - `examples/auto-enrollment-demo.sh` now matches actual flow behavior

## Behavior that was unclear & resolution

- **Conversation participant timezone**: The API validates `timezone` in requests, but the store does not persist it. This is now documented as “not persisted” in `docs/api.md`.
- **Timer recovery**: Generic timer recovery only logs callbacks; business timers are recovered by the scheduler tool. Clarified in `docs/state-persistence-recovery.md`.

## Intentional removals / de-scoping

- Removed README claims about cron-based scheduling and “3‑bot coordinator” behavior that is not wired in the current code.
- Marked Python agent integration as not implemented instead of documenting it as active.

## Doc inventory map (current)

- `README.md` — accurate project overview and doc links.
- `docs/api.md` — REST endpoints and schemas.
- `docs/configuration.md` — env vars, flags, precedence, defaults.
- `docs/storage.md` — DBs, schema tables, legacy tables.
- `docs/flows.md` — flow generators and prompt types.
- `docs/conversation.md` — conversation flow summary.
- `docs/conversation-flow.md` — detailed flow internals.
- `docs/debug-mode.md` — GenAI debug logging and thinking capture.
- `docs/auto-enrollment-feature.md` — auto-enrollment behavior.
- `docs/intensity-adjustment-feature.md` — intensity poll logic.
- `docs/state-persistence-recovery.md` — recovery architecture details.
- `docs/development.md` — build/test/run.
- `docs/troubleshooting.md` — operational pitfalls.
- Legacy docs flagged as historical: beginner guide, onboarding, comprehensive flow architectures.
- `python/langchain/*` — experimental Python agent docs (not integrated).

## Follow-ups (not done)

- Generate a formal OpenAPI spec from handlers if/when schema automation is introduced.
