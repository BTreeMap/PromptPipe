# API Response Structure Examples

## Before Factory Pattern + Result Field

**Old response structure (inconsistent):**

```json
// Error responses
{"status": "error", "message": "Invalid JSON format"}

// Success responses (no data)
{"status": "ok"}

// Success responses with data (raw data, no status)
[
  {"from": "+123", "body": "hello", "time": 1623456789}
]

// Stats response (raw data, no status)
{
  "total_responses": 3,
  "responses_per_sender": {"+123": 2, "+456": 1},
  "avg_response_length": 5.5
}
```

## After Factory Pattern + Result Field

**New response structure (consistent):**

```json
// Error responses
{
  "status": "error",
  "message": "Invalid JSON format"
}

// Success responses (no data)
{
  "status": "ok"
}

// Success responses with data
{
  "status": "ok",
  "result": [
    {"from": "+123", "body": "hello", "time": 1623456789}
  ]
}

// Stats response
{
  "status": "ok",
  "result": {
    "total_responses": 3,
    "responses_per_sender": {"+123": 2, "+456": 1},
    "avg_response_length": 5.5
  }
}

// Scheduled response
{
  "status": "scheduled"
}

// Recorded response
{
  "status": "recorded"
}
```

## Factory Pattern Usage

### Old way (still supported but deprecated)

```go
response := models.NewErrorResponse("Something went wrong")
```

### New factory pattern

```go
response := models.Factory.Error("Something went wrong")
response := models.Factory.Success(data)
response := models.Factory.SuccessWithMessage("Created successfully", data)
response := models.Factory.Scheduled()
response := models.Factory.Recorded()
```

## Benefits

1. **Consistency**: All API responses now have the same structure
2. **Type Safety**: Factory methods ensure correct status values
3. **Flexibility**: Result field can handle any JSON-serializable data
4. **Extensibility**: Easy to add new response types or modify structure
5. **RESTful**: Proper status codes with structured JSON responses
