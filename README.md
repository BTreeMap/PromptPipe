# PromptPipe

PromptPipe is a Go-based WhatsApp messaging service built on the [whatsmeow](https://github.com/tulir/whatsmeow) client. It exposes a REST API to send and schedule prompts, track receipts and responses, and run a stateful conversation flow (intake + feedback). OpenAI-backed GenAI features are optional.

## Table of Contents

- [Overview](#overview)
- [Quickstart](#quickstart)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Conversation Flow](#conversation-flow)
- [Storage](#storage)
- [Development](#development)
- [Docs Index](#docs-index)
- [License](#license)

## Overview

Key capabilities:

- Send and schedule prompts (static, genai, branch, conversation, custom)
- WhatsApp integration via whatsmeow
- Conversation flow with intake and feedback modules
- Auto-enrollment of new participants (optional)
- Receipt/response tracking and timer introspection
- SQLite or PostgreSQL for application data

## Quickstart

```bash
# Build
make build

# Run (loads .env if present)
./build/promptpipe
```

Optional GenAI configuration:

```bash
export OPENAI_API_KEY="your-openai-key"
./build/promptpipe
```

PromptPipe searches for `.env` files in `./.env`, `../.env`, and `../../.env`.

## Configuration

See [docs/configuration.md](docs/configuration.md) for environment variables, CLI flags, defaults, and precedence rules.

## API Reference

See [docs/api.md](docs/api.md) for endpoints, request/response schemas, and examples. API test scripts live in [test-scripts/](test-scripts/README.md).

## Conversation Flow

See [docs/conversation.md](docs/conversation.md) for the current flow summary and [docs/conversation-flow.md](docs/conversation-flow.md) for detailed behavior.

## Storage

See [docs/storage.md](docs/storage.md) for database layout and persistence details.

## Development

See [docs/development.md](docs/development.md) for build/test instructions.

## Docs Index

Current implementation references:

- [Configuration](docs/configuration.md)
- [API Reference](docs/api.md)
- [Flow Generators](docs/flows.md)
- [Conversation Flow (summary)](docs/conversation.md)
- [Conversation Flow (detailed)](docs/conversation-flow.md)
- [Storage](docs/storage.md)
- [Debug Mode](docs/debug-mode.md)
- [Auto-enrollment](docs/auto-enrollment-feature.md)
- [Intensity adjustment](docs/intensity-adjustment-feature.md)
- [State recovery](docs/state-persistence-recovery.md)
- [Troubleshooting](docs/troubleshooting.md)

Legacy or historical design docs (kept for reference):

- [Beginner guide](docs/beginner-guide.md)
- [Onboarding guide](docs/onboarding.md)
- [Conversation flow architecture](docs/conversation-flow-architecture.md)
- [Conversation flow comprehensive](docs/conversation-flow-comprehensive.md)

Python/LangChain subproject: the `python/langchain` directory is experimental and **not integrated** with the Go service. Its documentation is marked accordingly.

## License

MIT License. See [LICENSE](LICENSE).
