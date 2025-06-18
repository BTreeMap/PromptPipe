#!/bin/bash

# Load environment variables from .env files
# Check for .env files in order of priority
ENV_FILES=(".env" "../.env" "../../.env")

# Colors for output
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export BLUE='\033[0;34m'
export NC='\033[0m' # No Color

# Test result tracking
export TESTS_PASSED=0
export TESTS_FAILED=0

# Logging function
log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
    ((TESTS_PASSED++))
}

error() {
    echo -e "${RED}✗${NC} $1"
    ((TESTS_FAILED++))
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

for env_file in "${ENV_FILES[@]}"; do
    if [ -f "$env_file" ]; then
        log "Loading environment variables from $env_file"
        set -a  # automatically export all variables
        source "$env_file"
        set +a  # stop automatically exporting
        break
    fi
done

# PromptPipe API Test Configuration
export API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
export TEST_PHONE="${TEST_PHONE:-15551234567}"  # Default test phone number
export TEST_PHONE_2="${TEST_PHONE_2:-15557654321}"  # Second test phone number

# Test helper function
test_endpoint() {
    local method="$1"
    local endpoint="$2"
    local data="$3"
    local expected_status="$4"
    local test_name="$5"
    
    log "Testing: $test_name"
    echo "  Request: $method $endpoint"
    
    if [ -n "$data" ]; then
        echo "  Data: $data"
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$API_BASE_URL$endpoint")
    else
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" \
            "$API_BASE_URL$endpoint")
    fi
    
    # Extract HTTP status and body
    status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    echo "  Response Status: $status"
    echo "  Response Body: $body"
    
    # Validate JSON if body is not empty
    if [ -n "$body" ] && ! echo "$body" | jq . >/dev/null 2>&1; then
        error "Invalid JSON response"
        return 1
    fi
    
    # Check status code
    if [ "$status" = "$expected_status" ]; then
        success "$test_name - Status: $status"
        
        # Pretty print JSON response
        if [ -n "$body" ]; then
            echo "  Formatted Response:"
            echo "$body" | jq . 2>/dev/null | sed 's/^/    /'
        fi
        return 0
    else
        error "$test_name - Expected: $expected_status, Got: $status"
        return 1
    fi
}

# Check if API is running
check_api() {
    log "Checking if PromptPipe API is running at $API_BASE_URL..."
    
    if curl -s --connect-timeout 3 "$API_BASE_URL" >/dev/null 2>&1; then
        success "API is reachable"
        return 0
    else
        error "API is not reachable at $API_BASE_URL"
        warn "Please start PromptPipe with: ./PromptPipe -api-addr :8080"
        return 1
    fi
}

# Summary function
print_summary() {
    echo
    echo "=================================="
    echo "Test Summary"
    echo "=================================="
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    echo "Total: $((TESTS_PASSED + TESTS_FAILED))"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi
}
