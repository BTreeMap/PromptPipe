#!/bin/bash

# Quick API health check and basic functionality test
# Load configuration
source "$(dirname "$0")/config.sh"

log "PromptPipe API Quick Test"

# Check if API is running
if ! check_api; then
    warn "Start PromptPipe with: ./PromptPipe -api-addr :8080"
    exit 1
fi

echo
echo "🔍 Quick Health Check"
echo "===================="

# Test basic endpoints quickly
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "static", 
    "body": "Quick test"
}' "200" "Quick send test"

test_endpoint "GET" "/receipts" "" "200" "Quick receipts check"

test_endpoint "GET" "/responses" "" "200" "Quick responses check"

test_endpoint "GET" "/stats" "" "200" "Quick stats check"

echo
log "✅ Basic functionality verified"
print_summary
