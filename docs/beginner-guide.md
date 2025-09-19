# PromptPipe: Complete Beginner's Guide

## High-Level Overview

**PromptPipe** is a sophisticated messaging service that delivers intelligent, adaptive prompts over WhatsApp. Think of it as a smart chatbot platform that can:

- **Send scheduled reminders** - Set up daily habit reminders, health interventions, or any recurring messages
- **Have conversations** - Engage in AI-powered conversations using OpenAI's GPT models
- **Track engagement** - Monitor message delivery, read receipts, and participant responses
- **Manage participants** - Enroll users, track their preferences, and maintain conversation history
- **Support multiple workflows** - Handle static messages, branching prompts, or fully conversational AI

**What problem does it solve?**
PromptPipe solves the challenge of delivering personalized, timely interventions at scale. It's particularly useful for:

- Health and wellness programs
- Habit formation coaching
- Research studies requiring participant engagement
- Customer engagement campaigns
- Educational reminder systems

**Main function:**
The application acts as a bridge between your business logic and WhatsApp, providing a robust REST API to schedule, send, and track messages while maintaining persistent conversations with participants.

## Setup and Prerequisites

### System Requirements

- **Operating System**: Linux (tested), macOS, or Windows
- **Go Version**: 1.24.3 or later
- **Memory**: Minimum 512MB RAM (1GB+ recommended for production)
- **Storage**: 100MB for application + database storage space
- **Network**: Internet access for WhatsApp connectivity and OpenAI API

### Dependencies

PromptPipe relies on several external libraries and services:

#### Core Dependencies (automatically installed)

- **[whatsmeow](https://github.com/tulir/whatsmeow)** - WhatsApp Web API client library
- **[OpenAI Go SDK](https://github.com/openai/openai-go)** - For AI-powered conversations
- **[godotenv](https://github.com/joho/godotenv)** - Environment variable management
- **Database drivers**:
  - `github.com/mattn/go-sqlite3` - SQLite support (default)
  - `github.com/lib/pq` - PostgreSQL support (optional)

#### External Services

- **WhatsApp Business Account** - Required for sending messages
- **OpenAI API Key** - Required for AI conversations (optional for basic messaging)

### Configuration

#### Step 1: Environment Variables

Create a `.env` file in your project root:

```bash
# Required: Database Configuration
# PromptPipe uses TWO separate databases:
# 1. WhatsApp Database (managed by whatsmeow library)
WHATSAPP_DB_DSN="file:/var/lib/promptpipe/whatsmeow.db?_foreign_keys=on"

# 2. Application Database (managed by PromptPipe)
DATABASE_DSN="file:/var/lib/promptpipe/state.db?_foreign_keys=on"

# Required: State Directory
PROMPTPIPE_STATE_DIR="/var/lib/promptpipe"

# Optional: API Configuration
API_ADDR=":8080"                                    # HTTP server address
DEFAULT_SCHEDULE="0 9 * * *"                       # Default cron (9 AM daily)

# Optional: AI Features
OPENAI_API_KEY="your_openai_api_key_here"          # For conversation features
GENAI_MODEL="gpt-4o-mini"                          # Override default model (default: gpt-4o-mini)

# Optional: Debug and Logging
PROMPTPIPE_DEBUG="false"                           # Enable debug mode
GENAI_TEMPERATURE="0.1"                            # AI response consistency (0.0-1.0)

# Optional: Conversation Bot Settings
CHAT_HISTORY_LIMIT="-1"                            # Message history limit (-1 = unlimited)
INTAKE_BOT_PROMPT_FILE="prompts/intake_bot_system.txt"
PROMPT_GENERATOR_PROMPT_FILE="prompts/prompt_generator_system.txt"
FEEDBACK_TRACKER_PROMPT_FILE="prompts/feedback_tracker_system.txt"
FEEDBACK_INITIAL_TIMEOUT="15m"                     # Timeout for initial feedback (e.g., "15m")
FEEDBACK_FOLLOWUP_DELAY="3h"                       # Delay for followup feedback (e.g., "3h")
SCHEDULER_PREP_TIME_MINUTES="5"                    # Prep time for scheduled reminders (in minutes)
```

#### Step 2: Directory Setup

Create the necessary directories:

```bash
# Create state directory
sudo mkdir -p /var/lib/promptpipe
sudo chown $USER:$USER /var/lib/promptpipe

# Create prompts directory (if using AI features)
mkdir -p prompts
```

#### Step 3: Database Options

##### Option A: SQLite (Default - Recommended for development)

```bash
# No additional setup needed - uses file-based SQLite databases
# WhatsApp data: /var/lib/promptpipe/whatsmeow.db
# App data: /var/lib/promptpipe/state.db
```

##### Option B: PostgreSQL (Recommended for production)

```bash
# Update .env file:
WHATSAPP_DB_DSN="postgres://user:password@localhost:5432/whatsapp_db?sslmode=disable"
DATABASE_DSN="postgres://user:password@localhost:5432/promptpipe_db?sslmode=disable"
```

#### Step 4: Build the Application

```bash
# Clone the repository
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe

# Build the binary
make build
# OR use Go directly:
go build -o build/promptpipe cmd/PromptPipe/main.go
```

## Codebase Structure

### High-Level Architecture

```text
PromptPipe/
├── cmd/                     # Application entry points
├── internal/                # Core application logic
├── prompts/                 # AI system prompt templates
├── test-scripts/            # API testing utilities
├── docs/                    # Documentation
├── build/                   # Compiled binaries
└── docker/                  # Container configuration
```

### Core Directories

#### `/cmd/PromptPipe/`

**Purpose**: Application entry point and command-line interface

- `main.go` - Bootstrap logic, configuration loading, dependency injection

#### `/internal/` (Core Application Logic)

**`/internal/api/`**

- **Purpose**: HTTP REST API server and endpoint handlers
- `api.go` - Main server setup, dependency injection, graceful shutdown
- `handlers.go` - HTTP request/response handling for all endpoints
- `conversation_handlers.go` - Participant management endpoints
- `response.go` - API response formatting utilities

**`/internal/models/`**

- **Purpose**: Data structures and validation logic
- `models.go` - Core data types (Prompt, Receipt, Response, etc.)
- `state.go` - State management types for conversation flows
- `tools.go` - AI tool integration types
- `flow_types.go` - Flow-specific data structures

**`/internal/store/`**

- **Purpose**: Database abstraction and storage backends
- `store.go` - Common interfaces and in-memory implementation
- `sqlite.go` - SQLite database implementation
- `postgres.go` - PostgreSQL database implementation
- `migrations_*.sql` - Database schema definitions

**`/internal/whatsapp/`**

- **Purpose**: WhatsApp integration via whatsmeow library
- `whatsapp.go` - Client wrapper, message sending, authentication

**`/internal/messaging/`**

- **Purpose**: Messaging service abstraction
- `whatsapp_service.go` - WhatsApp service implementation
- `response_handler.go` - Incoming message routing and processing
- `service.go` - Messaging interface definitions

**`/internal/flow/`**

- **Purpose**: Conversation flow management and AI tools
- `conversation_flow.go` - Main conversation engine
- `state_manager.go` - State persistence and recovery
- `scheduler_tool.go` - Habit reminder scheduling
- `prompt_generator_tool.go` - AI-powered prompt generation

**`/internal/genai/`**

- **Purpose**: OpenAI integration for AI conversations
- `genai.go` - OpenAI client wrapper and conversation logic

**`/internal/recovery/`**

- **Purpose**: Application state recovery and resilience
- `recovery.go` - State recovery after restarts

### Key Files and Their Roles

| File | Purpose |
|------|---------|
| `cmd/PromptPipe/main.go` | Entry point - loads config, starts all services |
| `internal/api/api.go` | HTTP server - handles REST API requests |
| `internal/models/models.go` | Data types - defines all data structures |
| `internal/store/store.go` | Database layer - abstracts storage operations |
| `internal/whatsapp/whatsapp.go` | WhatsApp client - sends/receives messages |
| `internal/flow/conversation_flow.go` | AI brain - manages conversations |
| `internal/messaging/response_handler.go` | Message router - routes incoming messages |

## Execution Workflow

### Application Startup Sequence

1. **Configuration Loading** (`main.go`)
   - Load environment variables from `.env` files
   - Parse command-line flags
   - Set default values for missing configuration

2. **Directory and Lock Management**
   - Create state directory if it doesn't exist
   - Acquire file lock to prevent multiple instances
   - Set up graceful shutdown signal handlers

3. **Database Initialization**
   - Connect to WhatsApp database (for whatsmeow session data)
   - Connect to application database (for receipts, responses, state)
   - Run database migrations if needed

4. **WhatsApp Client Setup**
   - Initialize whatsmeow client with database connection
   - Handle login process (QR code or numeric code)
   - Establish connection to WhatsApp servers

5. **Service Layer Initialization**
   - Create messaging service (wraps WhatsApp client)
   - Initialize store backend (SQLite or PostgreSQL)
   - Set up GenAI client (if OpenAI key provided)

6. **Flow Engine Setup**
   - Initialize conversation flow manager
   - Load AI system prompts from files
   - Register AI tools (scheduler, prompt generator, etc.)
   - Set up response handler for incoming messages

7. **Recovery and State Restoration**
   - Recover active conversation states
   - Restore scheduled timers and reminders
   - Rebuild response handler hooks

8. **HTTP Server Startup**
   - Register API endpoints
   - Start HTTP server on configured port
   - Begin processing incoming requests

### Message Processing Flow

**Outgoing Messages (API → WhatsApp):**

1. **API Request** - Client sends POST to `/send` or `/schedule`
2. **Validation** - Validate phone number, message content, schedule
3. **Flow Processing** - Generate message content (static, AI, or branch)
4. **Message Delivery** - Send via WhatsApp client
5. **Receipt Tracking** - Store sent receipt in database
6. **Response** - Return success/error to API client

**Incoming Messages (WhatsApp → API):**

1. **Message Reception** - WhatsApp client receives message event
2. **Phone Number Canonicalization** - Normalize phone number format
3. **Hook Resolution** - Find registered response handler for phone number
4. **Flow Processing** - Route to appropriate conversation flow
5. **AI Processing** - Generate response using OpenAI (if applicable)
6. **Response Delivery** - Send reply via WhatsApp client
7. **State Persistence** - Save conversation state and history

## Core Components and Modules

### 1. API Server (`internal/api/`)

**Purpose**: Provides REST endpoints for external integration

**Key Functions**:

- `POST /send` - Send immediate messages
- `POST /schedule` - Schedule recurring messages  
- `GET /receipts` - Retrieve delivery status
- `POST /conversation/participants` - Manage participants
- `GET /timers` - View active schedules
- `GET /stats` - View application statistics
- `POST /response` - Manually submit a participant response

**How it contributes**: Acts as the primary interface for external systems to interact with PromptPipe

### 2. Messaging Service (`internal/messaging/`)

**Purpose**: Abstracts message sending/receiving across different platforms

**Key Components**:

- **WhatsAppService**: Implements WhatsApp-specific messaging
- **ResponseHandler**: Routes incoming messages to appropriate handlers
- **Phone Number Validation**: Ensures proper E.164 format

**How it contributes**: Provides platform-agnostic messaging interface, enabling easy switching between WhatsApp, SMS, or other providers

### 3. Conversation Flow Engine (`internal/flow/`)

**Purpose**: Manages stateful AI conversations and tool integration

**Key Features**:

- **State Management**: Tracks conversation history and participant context
- **AI Tool Integration**: Supports scheduler, prompt generator, and custom tools
- **Three-Bot Architecture**: Specialized bots for intake, prompts, and feedback

**How it contributes**: Enables sophisticated AI-powered conversations with persistent state and tool calling capabilities

### 4. Store Layer (`internal/store/`)

**Purpose**: Database abstraction with multiple backend support

**Supported Operations**:

- Receipt and response tracking
- Conversation participant management
- Flow state persistence
- Response hook registration

**How it contributes**: Provides reliable data persistence with backend flexibility (SQLite for development, PostgreSQL for production)

### 5. WhatsApp Integration (`internal/whatsapp/`)

**Purpose**: Wraps whatsmeow library for WhatsApp connectivity

**Key Capabilities**:

- Message sending with delivery confirmation
- Event handling (incoming messages, receipts)
- Authentication and session management

**How it contributes**: Handles all WhatsApp-specific communication protocols and state management

### 6. GenAI Integration (`internal/genai/`)

**Purpose**: Integrates OpenAI for AI-powered conversations

**Features**:

- Tool calling support for function integration
- Conversation history management
- Temperature and parameter control
- Model selection via env var (GENAI_MODEL) or CLI flag (--genai-model)

**How it contributes**: Enables intelligent, context-aware responses and tool-augmented AI conversations

## Database Description

PromptPipe uses **two separate databases** to maintain clear separation of concerns:

### 1. WhatsApp Database (Managed by whatsmeow)

**Purpose**: Stores WhatsApp session data and connection state
**Schema**: Controlled by whatsmeow library (we don't modify this)
**Tables**: Device info, encryption keys, contact data, message metadata

### 2. Application Database (Managed by PromptPipe)

**Purpose**: Stores application-specific data and conversation state

#### Core Tables

**`receipts`** - Message delivery tracking

```sql
receipts (
    id INTEGER PRIMARY KEY,
    recipient TEXT NOT NULL,        -- E.164 phone number
    status TEXT NOT NULL,           -- "sent", "delivered", "read", "failed"
    time INTEGER NOT NULL           -- Unix timestamp
)
```

**`responses`** - Incoming message storage

```sql
responses (
    id INTEGER PRIMARY KEY,
    sender TEXT NOT NULL,           -- E.164 phone number
    body TEXT NOT NULL,             -- Message content
    time INTEGER NOT NULL           -- Unix timestamp
)
```

**`flow_states`** - Conversation state management

```sql
flow_states (
    id INTEGER PRIMARY KEY,
    participant_id TEXT NOT NULL,  -- Unique participant identifier
    flow_type TEXT NOT NULL,       -- "conversation", "intervention", etc.
    current_state TEXT NOT NULL,   -- Current state in flow
    state_data TEXT,               -- JSON blob with conversation history/data
    last_prompted_at DATETIME,     -- Timestamp of the last prompt sent
    created_at DATETIME,
    updated_at DATETIME,
    UNIQUE(participant_id, flow_type)
)
```

**`conversation_participants`** - Participant profiles

```sql
conversation_participants (
    id TEXT PRIMARY KEY,           -- UUID
    phone_number TEXT UNIQUE,      -- E.164 phone number
    name TEXT,                     -- Display name
    gender TEXT,                   -- Demographics (optional)
    ethnicity TEXT,                -- Demographics (optional)
    background TEXT,               -- Cultural/context info (optional)
    status TEXT NOT NULL,          -- "active", "paused", "completed", "withdrawn"
    enrolled_at DATETIME,
    created_at DATETIME,
    updated_at DATETIME
)
```

**`registered_hooks`** - Response routing configuration

```sql
registered_hooks (
    phone_number TEXT PRIMARY KEY, -- E.164 phone number
    hook_type TEXT NOT NULL,       -- "conversation", "branch", "genai"
    parameters TEXT NOT NULL,      -- JSON configuration
    created_at DATETIME,
    updated_at DATETIME
)
```

### Database Relationships

```text
conversation_participants (1) ←→ (1) flow_states
conversation_participants (1) ←→ (1) registered_hooks
conversation_participants (1) ←→ (N) responses

#### Step 1: Install and Configure

```bash
# 1. Clone and build
git clone https://github.com/BTreeMap/PromptPipe.git
cd PromptPipe
go build -o build/promptpipe cmd/PromptPipe/main.go

# 2. Create configuration
cat > .env << EOF
PROMPTPIPE_STATE_DIR="/tmp/promptpipe"
API_ADDR=":8080"
DEFAULT_SCHEDULE="0 9 * * *"
OPENAI_API_KEY="your_key_here"
EOF

# 3. Create directories
mkdir -p /tmp/promptpipe
```

#### Step 2: Start the Service

```bash
# Start PromptPipe
./build/promptpipe

# Expected output:
# [2025-08-15 10:30:00] Bootstrapping PromptPipe with configured modules
# [2025-08-15 10:30:01] WhatsApp login required; starting QR code flow
# [QR CODE DISPLAYED] - Scan with WhatsApp
# [2025-08-15 10:30:05] WhatsApp client connected successfully
# [2025-08-15 10:30:05] HTTP server listening on :8080
```

#### Step 3: API Usage Examples

**Send a Simple Message:**

```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "+15551234567",
    "type": "static",
    "body": "Hello! This is a test message from PromptPipe."
  }'

# Response:
# {"status":"ok"}
```

**Send an AI-Generated Message:**

```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "+15551234567",
    "type": "genai",
    "system_prompt": "You are a helpful wellness coach. Keep responses under 50 words.",
    "user_prompt": "Send a motivational message about starting a new day with healthy habits."
  }'

# Response:
# {"status":"ok"}
# Message sent: "Good morning! Today is a fresh start. Begin with one small healthy choice - maybe a glass of water or a 2-minute walk. Small steps build big changes. You've got this! 💪✨"
```

**Schedule Daily Reminders:**

```bash
curl -X POST http://localhost:8080/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "to": "+15551234567",
    "type": "static",
    "body": "Daily reminder: Take your vitamins! 💊",
    "schedule": {
      "hour": 9,
      "minute": 0,
      "timezone": "America/Toronto"
    }
  }'

# Response:
# {"status":"ok"}
```

**Enroll a Conversation Participant:**

```bash
curl -X POST http://localhost:8080/conversation/participants \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+15551234567",
    "name": "John Doe",
    "timezone": "America/Toronto"
  }'
```

#### Step 4: Monitor and Check Status

**Check Message Receipts:**

```bash
curl http://localhost:8080/receipts

# Response:
# {"status":"ok","result":[
#   {"to":"+15551234567","status":"delivered","time":1692097200}
# ]}
```

**View Active Timers:**

```bash
curl http://localhost:8080/timers

# Response:
# {"status":"ok","result":[
#   {"id":"timer_xyz789","type":"recurring","next_run":"2025-08-16T09:00:00Z","description":"Daily reminder"}
# ]}
```

### Sample Output

When you start PromptPipe and send the above test message, you should see:

**Console Output:**

```text
[2025-08-15 10:35:22] WhatsAppService message sent and receipt emitted to=+15551234567
[2025-08-15 10:35:23] Receipt: delivered to=+15551234567 status=delivered
[2025-08-15 10:35:25] Receipt: read to=+15551234567 status=read
```

**WhatsApp Message Received:**

```text
Hello! This is a test message from PromptPipe.
```

**API Response:**

```json
{"status":"ok","message":"Message sent successfully"}
```

**WhatsApp Message Received:**

```text
Hello! This is a test message from PromptPipe.
```

**API Response:**

```json
{"status":"ok","message":"Message sent successfully"}
```

## Troubleshooting Guide

### Common Issues and Solutions

#### 1. WhatsApp Connection Problems

**Error**: `Failed to connect to WhatsApp server`
**Causes**:

- No internet connection
- WhatsApp servers down
- Blocked by firewall

**Solutions**:

```bash
# Check internet connectivity
ping 8.8.8.8

# Verify WhatsApp server access
curl -I https://web.whatsapp.com

# Check firewall settings
sudo ufw status
```

#### 2. Database Connection Issues

**Error**: `Failed to initialize store: database is locked`
**Causes**:

- Another PromptPipe instance running
- Database files have wrong permissions

**Solutions**:

```bash
# Kill existing instances
pkill -f PromptPipe

# Fix permissions
sudo chown $USER:$USER /var/lib/promptpipe/*
chmod 644 /var/lib/promptpipe/*.db

# Check for lock files
ls -la /var/lib/promptpipe/
rm -f /var/lib/promptpipe/*.lock
```

#### 3. API Server Won't Start

**Error**: `bind: address already in use`
**Causes**:

- Port 8080 already occupied
- Another web server running

**Solutions**:

```bash
# Find what's using the port
sudo lsof -i :8080

# Kill the process
sudo kill -9 <PID>

# Or use a different port
export API_ADDR=":8081"
```

#### 4. OpenAI API Errors

**Error**: `GenAI request failed: unauthorized`
**Causes**:

- Invalid API key
- Quota exceeded
- Network issues

**Solutions**:

```bash
# Test API key directly
curl -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/models

# Check usage at platform.openai.com
# Verify key in .env file
grep OPENAI_API_KEY .env
```

#### 5. Phone Number Validation Errors

**Error**: `recipient validation failed`
**Causes**:

- Wrong phone number format
- Missing country code

**Solutions**:

```bash
# Correct format examples:
# ✅ "+15551234567" (with country code)
# ❌ "555-123-4567" (no country code)
# ❌ "15551234567" (missing +)

# Test with curl:
curl -X POST http://localhost:8080/send \
  -d '{"to":"+15551234567","type":"static","body":"test"}'
```

#### 6. Message Not Delivered

**Symptoms**: API returns success but message never arrives

**Debugging Steps**:

```bash
# 1. Check receipts endpoint
curl http://localhost:8080/receipts | jq

# 2. Verify phone number is registered with WhatsApp
# 3. Check WhatsApp Business account status
# 4. Review application logs for errors

# 5. Test with a different phone number
curl -X POST http://localhost:8080/send \
  -d '{"to":"+1234567890","type":"static","body":"test"}'
```

#### 7. Performance Issues

**Symptoms**: Slow response times, high memory usage

**Optimization Steps**:

```bash
# Monitor resource usage
top -p $(pgrep PromptPipe)

# Check database size
du -h /var/lib/promptpipe/

# Clean old data (if needed)
sqlite3 /var/lib/promptpipe/state.db "DELETE FROM receipts WHERE time < strftime('%s', 'now', '-30 days');"

# Restart service
systemctl restart promptpipe
```

#### 8. Configuration Problems

**Error**: Environment variables not loaded

**Solutions**:

```bash
# Verify .env file location
ls -la .env

# Test environment loading
source .env && echo $API_ADDR

# Check file format (no spaces around =)
cat .env | grep -E '^[A-Z_]+=.*'

# Validate JSON in configuration
echo '{"test": "json"}' | jq .
```

### Getting Help

1. **Check Logs**: Review console output for error details
2. **Enable Debug Mode**: Set `PROMPTPIPE_DEBUG=true` in .env
3. **Test Endpoints**: Use the provided test scripts in `test-scripts/`
4. **Check Documentation**: Review other docs in the `docs/` folder
5. **GitHub Issues**: Report bugs at the project repository

### Quick Diagnostic Commands

```bash
# Health check
curl -f http://localhost:8080/stats || echo "API not responding"

# Database check
sqlite3 /var/lib/promptpipe/state.db ".tables"

# WhatsApp connection status
grep -i "whatsapp.*connected" logs/promptpipe.log

# Recent activity
tail -f logs/promptpipe.log | grep -E "(error|failed|success)"
```

---

This documentation provides a complete foundation for understanding and using PromptPipe. For advanced features like custom flow generators, AI tool development, or production deployment, refer to the specific documentation in the `docs/` directory.
