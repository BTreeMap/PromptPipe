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

# Test 3: GenAI - Quick Motivation
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a brief motivational message",
    "system_prompt": "You are a concise motivational speaker. Keep responses under 50 words.",
    "user_prompt": "Create a short motivational message for someone starting their workday. Include one positive affirmation and one actionable tip. Be inspiring but brief."
}' "200" "Send GenAI quick motivation"

# Test 4: GenAI - Simple Explanation
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Explain a concept simply",
    "system_prompt": "You explain complex topics in simple terms. Always keep responses under 75 words and use everyday analogies.",
    "user_prompt": "Explain what artificial intelligence is using only a cooking analogy. Make it accurate but very simple and brief."
}' "200" "Send GenAI simple explanation"

# Test 5: GenAI - Quick Tip
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a practical tip",
    "system_prompt": "You are a practical advice expert. Give concise, actionable tips in under 40 words.",
    "user_prompt": "Give one quick productivity tip for someone who feels overwhelmed with tasks. Make it immediately actionable."
}' "200" "Send GenAI quick tip"

# Test 6: Invalid phone number
test_endpoint "POST" "/send" '{
    "to": "invalid-phone",
    "type": "static",
    "body": "This should fail"
}' "400" "Send with invalid phone number"

# Test 7: Missing required fields
test_endpoint "POST" "/send" '{
    "type": "static",
    "body": "Missing phone number"
}' "400" "Send with missing phone number"

# Test 8: Empty body
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": ""
}' "400" "Send with empty body"

# Test 9: Invalid prompt type
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "invalid_type",
    "body": "Test message"
}' "400" "Send with invalid prompt type"

# Test 10: Wrong HTTP method
test_endpoint "GET" "/send" "" "405" "Send with GET method (should fail)"

# Test 11: Invalid JSON
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
