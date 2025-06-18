# PromptPipe API Test Scripts

Simple bash scripts using curl and jq to test PromptPipe API functionality.

## Prerequisites

- `curl` - for making HTTP requests
- `jq` - for JSON parsing and formatting
- `bash` - shell environment
- Running PromptPipe instance on localhost:8080

## Installation

Install required tools:

```bash
# Ubuntu/Debian
sudo apt install curl jq

# macOS
brew install curl jq

# Alpine Linux
apk add curl jq
```

## Usage

### Make scripts executable

```bash
chmod +x test-scripts/*.sh
```

### Quick Test

```bash
# Basic health check
./test-scripts/quick-test.sh
```

### Individual Test Suites

```bash
# Test sending messages
./test-scripts/test-send.sh

# Test scheduling
./test-scripts/test-schedule.sh

# Test response handling
./test-scripts/test-responses.sh

# Test receipt tracking
./test-scripts/test-receipts.sh
```

### Complete Test Suite

```bash
# Run all tests
./test-scripts/run-all-tests.sh
```

## Configuration

Edit `config.sh` to customize:

- API base URL (default: <http://localhost:8080>)
- Test phone numbers (default: +15551234567, +15551234568)
- Output colors and logging

## Test Coverage

### Send Tests (`test-send.sh`)

- ✅ Static messages
- ✅ Branch messages with options
- ✅ GenAI messages (with/without OpenAI key)
- ✅ Custom flow messages
- ✅ Input validation (phone numbers, required fields)
- ✅ Error handling (invalid JSON, wrong methods)

### Schedule Tests (`test-schedule.sh`)

- ✅ Cron-based scheduling
- ✅ Different prompt types with scheduling
- ✅ Cron expression validation
- ✅ Edge cases (frequent schedules, future dates)

### Response Tests (`test-responses.sh`)

- ✅ Recording participant responses
- ✅ Special characters and emojis
- ✅ Long message handling
- ✅ Response statistics
- ✅ Data structure validation

### Receipt Tests (`test-receipts.sh`)

- ✅ Receipt retrieval
- ✅ Receipt data structure
- ✅ Status tracking
- ✅ Receipt analysis

### Integration Tests (`run-all-tests.sh`)

- ✅ Complete workflows (send → receipt → response → stats)
- ✅ All prompt types in sequence
- ✅ Error scenario testing
- ✅ Basic load testing
- ✅ Data consistency verification

## Output

Tests provide:

- ✅ **Colored output** for easy reading
- 📊 **Detailed request/response logging**
- 📈 **Test summaries with pass/fail counts**
- 🔍 **JSON structure validation**
- 📋 **Data verification and analysis**

## Example Output

```
[2025-06-18 10:30:15] Testing: Send static message
  Request: POST /send
  Data: {"to": "+15551234567", "type": "static", "body": "Hello from PromptPipe test!"}
  Response Status: 200
  Response Body: {"status":"ok"}
✓ Send static message - Status: 200
  Formatted Response:
    {
      "status": "ok"
    }

==================================
Test Summary
==================================
Passed: 15
Failed: 0
Total: 15
All tests passed!
```

## Files

- `config.sh` - Configuration and shared functions
- `test-send.sh` - Test POST /send endpoint
- `test-schedule.sh` - Test POST /schedule endpoint  
- `test-responses.sh` - Test response recording and stats
- `test-receipts.sh` - Test receipt tracking
- `run-all-tests.sh` - Complete test suite with integration tests
- `quick-test.sh` - Fast health check

## Notes

- Tests use test phone numbers (+15551234567, +15551234568) by default
- Scripts will detect if PromptPipe is not running and provide start instructions
- All JSON responses are validated and pretty-printed
- Error cases are tested to ensure proper API behavior
- Integration tests verify complete message workflows
