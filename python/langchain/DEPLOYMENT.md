# Deployment Guide for PromptPipe Agent

> ⚠️ **Experimental**: The Go service does not integrate with this Python agent today. Deploy only if you plan to run the agent independently or add the missing Go integration.

This guide explains how to deploy the Python/LangChain agentic layer alongside the Go message delivery service.

## Architecture Overview

```
┌─────────────────┐      ┌──────────────────┐      ┌─────────────────┐
│                 │      │                  │      │                 │
│   WhatsApp      │─────▶│   Go Service     │─────▶│  Python Agent   │
│   Messages      │      │  (Message        │      │  (Conversation  │
│                 │◀─────│   Delivery)      │◀─────│   Processing)   │
└─────────────────┘      └──────────────────┘      └─────────────────┘
                                │                          │
                                │                          │
                                ▼                          ▼
                         ┌──────────────┐          ┌──────────────┐
                         │   SQLite     │◀─────────│   SQLite     │
                         │  (Go Data)   │  Shared  │ (Agent Data) │
                         └──────────────┘          └──────────────┘
```

## Prerequisites

- Python 3.12+
- Go 1.21+
- OpenAI API key
- SQLite or PostgreSQL

## Option 1: Docker Deployment (Recommended)

### 1. Create docker-compose.yml

```yaml
version: '3.8'

services:
  go-service:
    build:
      context: .
      dockerfile: Dockerfile.go
    ports:
      - "8080:8080"
    environment:
      - API_ADDR=:8080
      - DATABASE_DSN=/data/state.db
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - PYTHON_AGENT_URL=http://python-agent:8001
    volumes:
      - promptpipe-data:/var/lib/promptpipe
    depends_on:
      - python-agent

  python-agent:
    build:
      context: ./python/langchain
      dockerfile: Dockerfile
    ports:
      - "8001:8001"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_MODEL=gpt-4o-mini
      - PROMPTPIPE_STATE_DIR=/var/lib/promptpipe
      - API_HOST=0.0.0.0
      - API_PORT=8001
    volumes:
      - promptpipe-data:/var/lib/promptpipe

volumes:
  promptpipe-data:
```

### 2. Create Dockerfile for Python Agent

```dockerfile
FROM python:3.12-slim

WORKDIR /app

# Install uv
RUN pip install uv

# Copy project files
COPY pyproject.toml uv.lock ./
COPY promptpipe_agent ./promptpipe_agent

# Install dependencies
RUN uv sync --frozen

# Expose port
EXPOSE 8001

# Run the application
CMD ["uv", "run", "uvicorn", "promptpipe_agent.api.main:app", "--host", "0.0.0.0", "--port", "8001"]
```

### 3. Deploy

```bash
# Set your OpenAI API key
export OPENAI_API_KEY=your_key_here

# Start services
docker-compose up -d

# Check logs
docker-compose logs -f
```

## Option 2: Manual Deployment

### 1. Deploy Python Agent

```bash
cd python/langchain

# Create virtual environment
uv sync --extra dev

# Create .env file
cat > .env << EOF
OPENAI_API_KEY=your_key_here
OPENAI_MODEL=gpt-4o-mini
PROMPTPIPE_STATE_DIR=/var/lib/promptpipe
API_HOST=0.0.0.0
API_PORT=8001
EOF

# Run the service
uv run uvicorn promptpipe_agent.api.main:app --host 0.0.0.0 --port 8001
```

### 2. Configure Go Service

Update your Go service environment variables:

```bash
export PYTHON_AGENT_URL=http://localhost:8001
export DATABASE_DSN=/var/lib/promptpipe/state.db
```

### 3. Start Go Service

```bash
./build/promptpipe
```

## Option 3: Systemd Services

### Python Agent Service

Create `/etc/systemd/system/promptpipe-agent.service`:

```ini
[Unit]
Description=PromptPipe Python Agent
After=network.target

[Service]
Type=simple
User=promptpipe
WorkingDirectory=/opt/promptpipe/python/langchain
Environment="OPENAI_API_KEY=your_key_here"
Environment="PROMPTPIPE_STATE_DIR=/var/lib/promptpipe"
ExecStart=/usr/local/bin/uv run uvicorn promptpipe_agent.api.main:app --host 0.0.0.0 --port 8001
Restart=always

[Install]
WantedBy=multi-user.target
```

### Go Service

Create `/etc/systemd/system/promptpipe.service`:

```ini
[Unit]
Description=PromptPipe Go Service
After=network.target promptpipe-agent.service
Requires=promptpipe-agent.service

[Service]
Type=simple
User=promptpipe
WorkingDirectory=/opt/promptpipe
Environment="PYTHON_AGENT_URL=http://localhost:8001"
ExecStart=/opt/promptpipe/build/promptpipe
Restart=always

[Install]
WantedBy=multi-user.target
```

### Enable and Start Services

```bash
sudo systemctl daemon-reload
sudo systemctl enable promptpipe-agent promptpipe
sudo systemctl start promptpipe-agent
sudo systemctl start promptpipe
```

## Health Checks

### Python Agent

```bash
curl http://localhost:8001/health
# Should return: {"status":"healthy","version":"0.1.0"}
```

### Go Service

```bash
curl http://localhost:8080/health
# Should return health status
```

## Monitoring

### Logs

```bash
# Python Agent logs
journalctl -u promptpipe-agent -f

# Go Service logs
journalctl -u promptpipe -f
```

### Metrics

Both services expose health endpoints for monitoring:

- Python Agent: `http://localhost:8001/health`
- Go Service: `http://localhost:8080/health`

## Troubleshooting

### Python Agent Not Starting

1. Check OpenAI API key is set
2. Verify state directory exists and is writable
3. Check logs for errors

```bash
journalctl -u promptpipe-agent -n 50
```

### Go Service Can't Connect to Python

1. Verify Python Agent is running
2. Check PYTHON_AGENT_URL is correct
3. Test connection:

```bash
curl http://localhost:8001/process-message -X POST \
  -H "Content-Type: application/json" \
  -d '{"participant_id":"test","message":"hello","phone_number":"+15551234567"}'
```

### Database Issues

Both services share the same SQLite database. Ensure:

1. Directory permissions are correct
2. Only one process writes at a time
3. Database file is not corrupted

```bash
sqlite3 /var/lib/promptpipe/state.db "PRAGMA integrity_check;"
```

## Production Considerations

### Security

1. Use HTTPS for production
2. Set up proper firewall rules
3. Use environment variables for secrets
4. Limit API access to internal network

### Performance

1. Use PostgreSQL instead of SQLite for high traffic
2. Add Redis for caching
3. Use multiple Python workers:

```bash
uv run gunicorn promptpipe_agent.api.main:app \
  -w 4 \
  -k uvicorn.workers.UvicornWorker \
  --bind 0.0.0.0:8001
```

### Backup

Backup the shared database regularly:

```bash
# Create backup
sqlite3 /var/lib/promptpipe/state.db ".backup '/backup/state-$(date +%Y%m%d).db'"

# Restore backup
sqlite3 /var/lib/promptpipe/state.db ".restore '/backup/state-20250128.db'"
```

## Scaling

For high traffic, consider:

1. Load balancing multiple Python Agent instances
2. Shared PostgreSQL database
3. Redis for session/state caching
4. Kubernetes deployment with HPA

Example Kubernetes scaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: python-agent-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: python-agent
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```
