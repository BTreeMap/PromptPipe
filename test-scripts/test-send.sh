#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe API Send Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "=================================="
echo "Testing POST /send endpoint"
echo "=================================="

# Test 1: Send static message
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "Hello from PromptPipe test!"
}' "200" "Send static message"

# Test 2: Send branch message
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "branch",
    "body": "Choose an option:",
    "branch_options": [
        {"label": "Option A", "body": "You chose A"},
        {"label": "Option B", "body": "You chose B"}
    ]
}' "200" "Send branch message"

# Test 3: Send GenAI message (will work even without OpenAI key)
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a motivational message",
    "system_prompt": "You are a helpful assistant",
    "user_prompt": "Create a short motivational message"
}' "200" "Send GenAI message"

# Test 4: Invalid phone number
test_endpoint "POST" "/send" '{
    "to": "invalid-phone",
    "type": "static",
    "body": "This should fail"
}' "400" "Send with invalid phone number"

# Test 5: Missing required fields
test_endpoint "POST" "/send" '{
    "type": "static",
    "body": "Missing phone number"
}' "400" "Send with missing phone number"

# Test 6: Empty body
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": ""
}' "400" "Send with empty body"

# Test 7: Invalid prompt type
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "invalid_type",
    "body": "Test message"
}' "400" "Send with invalid prompt type"

# Test 8: Wrong HTTP method
test_endpoint "GET" "/send" "" "405" "Send with GET method (should fail)"

# Test 9: Invalid JSON
log "Testing: Send with invalid JSON"
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{"invalid": json}' \
    "$API_BASE_URL/send")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
if [ "$status" = "400" ]; then
    success "Send with invalid JSON - Status: $status"
else
    error "Send with invalid JSON - Expected: 400, Got: $status"
fi

print_summary
