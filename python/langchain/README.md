# PromptPipe Agent

> ⚠️ **Experimental**: This Python/LangChain service is not wired into the current Go implementation. The Go service runs its own conversation flow. Use these docs only if you intend to operate the Python agent separately.

LangChain-based agentic conversation flow for PromptPipe. This Python service handles all intelligent conversation processing, while the Go layer acts as a pure message delivery service.

## Architecture

This service implements a 3-bot architecture using LangChain:

- **Coordinator Agent**: Routes conversations and manages overall flow
- **Intake Agent**: Conducts intake conversations and builds user profiles
- **Feedback Agent**: Tracks user feedback and updates profiles

### Tools

Each agent has access to specialized tools:

- **StateTransitionTool**: Manages transitions between conversation states
- **ProfileSaveTool**: Saves and retrieves user profiles
- **SchedulerTool**: Schedules daily habit prompts
- **PromptGeneratorTool**: Generates personalized habit prompts

## Installation

This project uses `uv` for package management:

```bash
# Install dependencies
uv sync

# Install with dev dependencies
uv sync --extra dev
```

## Configuration

Create a `.env` file:

```bash
OPENAI_API_KEY=your_openai_api_key
OPENAI_MODEL=gpt-4o-mini
OPENAI_TEMPERATURE=0.1

# Path to Go application state directory
PROMPTPIPE_STATE_DIR=/var/lib/promptpipe

# Prompt files
INTAKE_BOT_PROMPT_FILE=../../prompts/intake_bot_system.txt
COORDINATOR_PROMPT_FILE=../../prompts/conversation_system_3bot.txt
FEEDBACK_TRACKER_PROMPT_FILE=../../prompts/feedback_tracker_system.txt
PROMPT_GENERATOR_PROMPT_FILE=../../prompts/prompt_generator_system.txt

# Timeouts
FEEDBACK_INITIAL_TIMEOUT=15m
FEEDBACK_FOLLOWUP_DELAY=3h

# API Configuration
API_HOST=0.0.0.0
API_PORT=8001
```

## Running

```bash
# Development mode
uv run uvicorn promptpipe_agent.api.main:app --reload --port 8001

# Production mode
uv run uvicorn promptpipe_agent.api.main:app --host 0.0.0.0 --port 8001
```

## Testing

```bash
# Run all tests
uv run pytest

# Run with coverage
uv run pytest --cov=promptpipe_agent --cov-report=html

# Run specific test file
uv run pytest tests/unit/test_coordinator_agent.py
```

## Development

```bash
# Format code
uv run black promptpipe_agent tests

# Lint
uv run ruff check promptpipe_agent tests

# Type check
uv run mypy promptpipe_agent
```

## API Endpoints

### POST /process-message

Process a user message through the conversation flow.

**Request:**
```json
{
  "participant_id": "part_abc123",
  "message": "User's message text",
  "phone_number": "+15551234567"
}
```

**Response:**
```json
{
  "response": "Agent's response text",
  "state": "COORDINATOR",
  "metadata": {}
}
```

### GET /health

Health check endpoint.

## Integration with Go Service

The Go service delegates message processing to this Python service via HTTP:

1. Go receives WhatsApp message
2. Go calls Python's `/process-message` endpoint
3. Python processes message through appropriate agent
4. Python returns response
5. Go sends response via WhatsApp
