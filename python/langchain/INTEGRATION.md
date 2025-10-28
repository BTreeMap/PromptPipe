# Integration Guide: Connecting Go Service to Python Agent

This guide explains how to integrate the Go message delivery service with the Python LangChain agent.

## Overview

The Go service needs to call the Python Agent API when processing conversation messages. This involves:

1. Detecting conversation-type messages
2. Calling the Python `/process-message` endpoint
3. Handling the response and sending it via WhatsApp

## Implementation Steps

### 1. Add HTTP Client to Go

Create `internal/agent/client.go`:

```go
package agent

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Client represents the Python agent HTTP client
type Client struct {
    baseURL    string
    httpClient *http.Client
}

// ProcessMessageRequest represents the request to the Python agent
type ProcessMessageRequest struct {
    ParticipantID string `json:"participant_id"`
    Message       string `json:"message"`
    PhoneNumber   string `json:"phone_number"`
}

// ProcessMessageResponse represents the response from the Python agent
type ProcessMessageResponse struct {
    Response string                 `json:"response"`
    State    string                 `json:"state"`
    Metadata map[string]interface{} `json:"metadata"`
}

// NewClient creates a new agent client
func NewClient(baseURL string) *Client {
    return &Client{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

// ProcessMessage sends a message to the Python agent for processing
func (c *Client) ProcessMessage(ctx context.Context, participantID, message, phoneNumber string) (*ProcessMessageResponse, error) {
    req := ProcessMessageRequest{
        ParticipantID: participantID,
        Message:       message,
        PhoneNumber:   phoneNumber,
    }

    jsonData, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/process-message", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("agent returned error: %s (status %d)", string(body), resp.StatusCode)
    }

    var result ProcessMessageResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &result, nil
}

// Health checks the health of the Python agent
func (c *Client) Health(ctx context.Context) error {
    httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("agent health check failed: status %d", resp.StatusCode)
    }

    return nil
}
```

### 2. Update Conversation Handlers

Modify `internal/api/conversation_handlers.go` to use the agent client:

```go
// Add agent client to Server struct
type Server struct {
    // ... existing fields ...
    agentClient *agent.Client
}

// Initialize agent client in main.go
func main() {
    // ... existing code ...
    
    agentBaseURL := os.Getenv("PYTHON_AGENT_URL")
    if agentBaseURL == "" {
        agentBaseURL = "http://localhost:8001"
    }
    
    agentClient := agent.NewClient(agentBaseURL)
    
    // Check agent health
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := agentClient.Health(ctx); err != nil {
        slog.Warn("Python agent health check failed", "error", err)
    } else {
        slog.Info("Python agent is healthy")
    }
    
    server := &api.Server{
        // ... existing fields ...
        agentClient: agentClient,
    }
    
    // ... rest of main ...
}
```

### 3. Modify Response Handler

Update the response handler to delegate to Python agent:

```go
func (s *Server) handleConversationResponse(ctx context.Context, from, body string) error {
    // Get participant by phone number
    participant, err := s.st.GetConversationParticipantByPhone(from)
    if err != nil {
        return fmt.Errorf("failed to get participant: %w", err)
    }
    if participant == nil {
        // Not a conversation participant, handle normally
        return s.handleNormalResponse(ctx, from, body)
    }

    slog.Debug("Processing conversation message via Python agent", 
        "participantID", participant.ID, 
        "phone", from)

    // Call Python agent
    agentResp, err := s.agentClient.ProcessMessage(ctx, participant.ID, body, from)
    if err != nil {
        slog.Error("Failed to process message via agent", "error", err)
        // Fall back to default response
        return s.sendMessage(ctx, from, "I apologize, but I'm having trouble processing your message right now. Please try again later.")
    }

    // Send the agent's response
    if err := s.sendMessage(ctx, from, agentResp.Response); err != nil {
        return fmt.Errorf("failed to send response: %w", err)
    }

    slog.Info("Conversation message processed successfully", 
        "participantID", participant.ID,
        "state", agentResp.State)

    return nil
}
```

### 4. Add Configuration

Update `.env` or environment variables:

```bash
# Python Agent Configuration
PYTHON_AGENT_URL=http://localhost:8001
PYTHON_AGENT_TIMEOUT=30s
PYTHON_AGENT_RETRY_COUNT=3
```

### 5. Add Error Handling and Retry Logic

```go
func (c *Client) ProcessMessageWithRetry(ctx context.Context, participantID, message, phoneNumber string, maxRetries int) (*ProcessMessageResponse, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        if i > 0 {
            // Exponential backoff
            delay := time.Duration(i*i) * time.Second
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
            }
        }
        
        resp, err := c.ProcessMessage(ctx, participantID, message, phoneNumber)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        slog.Warn("Agent request failed, retrying", 
            "attempt", i+1, 
            "maxRetries", maxRetries, 
            "error", err)
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
```

### 6. Add Metrics and Logging

```go
func (c *Client) ProcessMessage(ctx context.Context, participantID, message, phoneNumber string) (*ProcessMessageResponse, error) {
    start := time.Now()
    
    // ... existing code ...
    
    duration := time.Since(start)
    slog.Info("Agent request completed",
        "participantID", participantID,
        "duration", duration,
        "state", result.State)
    
    // Record metrics (if using prometheus or similar)
    // agentRequestDuration.Observe(duration.Seconds())
    // agentRequestCount.Inc()
    
    return &result, nil
}
```

## Testing the Integration

### 1. Unit Tests

Create `internal/agent/client_test.go`:

```go
package agent

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestProcessMessage(t *testing.T) {
    // Create mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.Method != "POST" {
            t.Errorf("expected POST, got %s", r.Method)
        }
        if r.URL.Path != "/process-message" {
            t.Errorf("expected /process-message, got %s", r.URL.Path)
        }

        // Return mock response
        resp := ProcessMessageResponse{
            Response: "Hello! How can I help you?",
            State:    "COORDINATOR",
            Metadata: map[string]interface{}{
                "participant_id": "test_123",
            },
        }
        json.NewEncoder(w).Encode(resp)
    }))
    defer server.Close()

    // Create client
    client := NewClient(server.URL)

    // Test
    ctx := context.Background()
    resp, err := client.ProcessMessage(ctx, "test_123", "Hello", "+15551234567")
    if err != nil {
        t.Fatalf("ProcessMessage failed: %v", err)
    }

    if resp.Response != "Hello! How can I help you?" {
        t.Errorf("unexpected response: %s", resp.Response)
    }
    if resp.State != "COORDINATOR" {
        t.Errorf("unexpected state: %s", resp.State)
    }
}
```

### 2. Integration Test

```bash
# Start Python agent
cd python/langchain
uv run uvicorn promptpipe_agent.api.main:app --port 8001 &

# Test from Go
curl -X POST http://localhost:8001/process-message \
  -H "Content-Type: application/json" \
  -d '{
    "participant_id": "test_123",
    "message": "Hello!",
    "phone_number": "+15551234567"
  }'

# Should return:
# {
#   "response": "...",
#   "state": "COORDINATOR",
#   "metadata": {...}
# }
```

## Migration Strategy

### Phase 1: Parallel Running (Recommended)

1. Deploy Python agent alongside Go service
2. Configure Go to call Python for conversation messages
3. Keep existing Go conversation flow as fallback
4. Monitor and compare responses

### Phase 2: Gradual Migration

1. Route 10% of conversation traffic to Python agent
2. Monitor metrics and error rates
3. Gradually increase percentage
4. Compare user engagement metrics

### Phase 3: Full Migration

1. Route 100% of conversation traffic to Python
2. Remove Go conversation flow code
3. Go service becomes pure message delivery

### Rollback Plan

If issues occur:

1. Update `PYTHON_AGENT_URL` to point to Go fallback
2. Restart Go service
3. Python agent can stay running for gradual re-migration

## Monitoring

### Key Metrics to Track

1. **Agent Response Time**: How long Python takes to process
2. **Error Rate**: Failed agent calls
3. **State Distribution**: How users move through states
4. **User Satisfaction**: Track conversation quality

### Example Prometheus Metrics

```go
var (
    agentRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "promptpipe_agent_request_duration_seconds",
            Help: "Duration of Python agent requests",
        },
        []string{"state"},
    )
    
    agentRequestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "promptpipe_agent_request_total",
            Help: "Total number of agent requests",
        },
        []string{"state", "status"},
    )
)
```

## Troubleshooting

### Common Issues

1. **Connection Refused**: Python agent not running
   - Check: `curl http://localhost:8001/health`

2. **Timeout**: Requests taking too long
   - Increase timeout in Go client
   - Check OpenAI API performance

3. **Invalid Response**: JSON parsing errors
   - Check Python agent logs
   - Validate response schema

4. **Database Lock**: SQLite conflicts
   - Consider PostgreSQL for production
   - Use connection pooling
