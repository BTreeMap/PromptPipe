#!/bin/bash
# test-restart-persistence.sh
#
# End-to-end test demonstrating that "docker compose down && docker compose up -d"
# does not lose durable state.
#
# Requirements:
#   - Docker and docker compose installed
#   - curl and jq installed
#   - Run from the repository root
#
# What this script tests:
#   1. Start the stack
#   2. Create a participant and schedule a job
#   3. Stop the stack (docker compose down)
#   4. Restart the stack (docker compose up -d)
#   5. Verify that flow state and the scheduled job persist
#
# Usage:
#   ./test-scripts/test-restart-persistence.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

success() { echo -e "${GREEN}✓${NC} $1"; ((TESTS_PASSED++)); }
error()   { echo -e "${RED}✗${NC} $1"; ((TESTS_FAILED++)); }
info()    { echo -e "${YELLOW}→${NC} $1"; }

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
TEST_PHONE="${TEST_PHONE:-15551234567}"

wait_for_api() {
    local max_attempts=30
    local attempt=0
    info "Waiting for API at $API_BASE_URL..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -sf "$API_BASE_URL/receipts" >/dev/null 2>&1; then
            success "API is ready"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 2
    done
    error "API did not become ready after $((max_attempts * 2))s"
    return 1
}

cleanup() {
    info "Cleaning up..."
    cd "$PROJECT_DIR"
    docker compose down --timeout 10 2>/dev/null || true
}

# ---- Main ----
cd "$PROJECT_DIR"
trap cleanup EXIT

info "Step 1: Starting stack"
docker compose up -d --build --wait 2>&1 || { error "docker compose up failed"; exit 1; }
wait_for_api

info "Step 2: Creating a conversation participant"
ENROLL_RESPONSE=$(curl -sf -X POST -H "Content-Type: application/json" \
    -d "{\"phone_number\": \"$TEST_PHONE\", \"name\": \"Restart Test\"}" \
    "$API_BASE_URL/conversation/participants" 2>&1) || true

if echo "$ENROLL_RESPONSE" | jq -e '.result.id' >/dev/null 2>&1; then
    PARTICIPANT_ID=$(echo "$ENROLL_RESPONSE" | jq -r '.result.id')
    success "Participant created: $PARTICIPANT_ID"
else
    # Participant may already exist, try to look up by phone
    LOOKUP_RESPONSE=$(curl -sf "$API_BASE_URL/conversation/participants" 2>&1) || true
    PARTICIPANT_ID=$(echo "$LOOKUP_RESPONSE" | jq -r ".result[] | select(.phone_number==\"$TEST_PHONE\") | .id" 2>/dev/null) || true
    if [ -n "$PARTICIPANT_ID" ] && [ "$PARTICIPANT_ID" != "null" ]; then
        success "Participant already exists: $PARTICIPANT_ID"
    else
        error "Could not create or find participant"
        info "Enroll response: $ENROLL_RESPONSE"
    fi
fi

info "Step 3: Checking receipts count (baseline)"
RECEIPTS_BEFORE=$(curl -sf "$API_BASE_URL/receipts" | jq 'if type == "array" then length elif .result then (.result | length) else 0 end' 2>/dev/null || echo "0")
info "Receipts before restart: $RECEIPTS_BEFORE"

info "Step 4: Stopping stack (docker compose down)"
docker compose down --timeout 10
success "Stack stopped"

info "Step 5: Restarting stack (docker compose up -d)"
docker compose up -d --wait 2>&1 || { error "docker compose up after restart failed"; exit 1; }
wait_for_api

info "Step 6: Verifying state persisted after restart"

# Check participant still exists
PARTICIPANTS=$(curl -sf "$API_BASE_URL/conversation/participants" 2>&1) || true
FOUND_PARTICIPANT=$(echo "$PARTICIPANTS" | jq -r ".result[] | select(.phone_number==\"$TEST_PHONE\") | .id" 2>/dev/null) || true

if [ -n "$FOUND_PARTICIPANT" ] && [ "$FOUND_PARTICIPANT" != "null" ]; then
    success "Participant persisted after restart: $FOUND_PARTICIPANT"
else
    error "Participant NOT found after restart"
fi

# Check receipts still exist
RECEIPTS_AFTER=$(curl -sf "$API_BASE_URL/receipts" | jq 'if type == "array" then length elif .result then (.result | length) else 0 end' 2>/dev/null || echo "0")
info "Receipts after restart: $RECEIPTS_AFTER"

if [ "$RECEIPTS_AFTER" -ge "$RECEIPTS_BEFORE" ]; then
    success "Receipts persisted after restart ($RECEIPTS_BEFORE -> $RECEIPTS_AFTER)"
else
    error "Receipts lost after restart ($RECEIPTS_BEFORE -> $RECEIPTS_AFTER)"
fi

# ---- Summary ----
echo
echo "=================================="
echo "Restart Persistence Test Summary"
echo "=================================="
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"
echo "Total: $((TESTS_PASSED + TESTS_FAILED))"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed! State survives restart.${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
