#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe API Response Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "=================================="
echo "Testing Response Management"
echo "=================================="

# Current timestamp
CURRENT_TIME=$(date +%s)

# Test 1: Record a response
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE'",
    "body": "This is a test response",
    "time": '$CURRENT_TIME'
}' "201" "Record participant response"

# Test 2: Record another response from different user
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE_2'",
    "body": "Another test response from different user",
    "time": '$((CURRENT_TIME + 10))'
}' "201" "Record response from second participant"

# Test 3: Record response with emoji and special characters
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE'",
    "body": "Response with emoji 😊 and special chars: @#$%",
    "time": '$((CURRENT_TIME + 20))'
}' "201" "Record response with special characters"

# Test 4: Record long response
LONG_TEXT="This is a very long response message that contains multiple sentences and should test the system ability to handle longer text responses. It includes various punctuation marks, numbers like 123, and should be stored properly in the database without any truncation issues."
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE'",
    "body": "'"$LONG_TEXT"'",
    "time": '$((CURRENT_TIME + 30))'
}' "201" "Record long response message"

# Test 5: Get all responses
test_endpoint "GET" "/responses" "" "200" "Retrieve all responses"

# Test 6: Get response statistics
test_endpoint "GET" "/stats" "" "200" "Get response statistics"

# Test 7: Record response with missing fields
test_endpoint "POST" "/response" '{
    "body": "Missing from field",
    "time": '$CURRENT_TIME'
}' "400" "Record response without from field"

# Test 8: Record response with invalid phone
test_endpoint "POST" "/response" '{
    "from": "invalid-phone",
    "body": "Test response",
    "time": '$CURRENT_TIME'
}' "400" "Record response with invalid phone"

# Test 9: Record response with empty body
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE'",
    "body": "",
    "time": '$CURRENT_TIME'
}' "400" "Record response with empty body"

# Test 10: Record response with invalid timestamp
test_endpoint "POST" "/response" '{
    "from": "'$TEST_PHONE'",
    "body": "Test response",
    "time": "invalid-time"
}' "400" "Record response with invalid timestamp"

# Test 11: Wrong HTTP method for responses
test_endpoint "POST" "/responses" "" "405" "POST to /responses (should fail)"

# Test 12: Wrong HTTP method for stats
test_endpoint "POST" "/stats" "" "405" "POST to /stats (should fail)"

# Verify we can retrieve and parse the responses
log "Verifying response data structure..."
response=$(curl -s "$API_BASE_URL/responses")
if echo "$response" | jq . >/dev/null 2>&1; then
    count=$(echo "$response" | jq '. | length')
    success "Retrieved $count responses with valid JSON structure"
    
    # Show sample response structure
    if [ "$count" -gt 0 ]; then
        echo "  Sample response structure:"
        echo "$response" | jq '.[0]' 2>/dev/null | sed 's/^/    /'
    fi
else
    error "Invalid JSON structure in responses"
fi

# Verify stats structure
log "Verifying stats data structure..."
stats_response=$(curl -s "$API_BASE_URL/stats")
if echo "$stats_response" | jq . >/dev/null 2>&1; then
    total=$(echo "$stats_response" | jq '.total_responses // 0')
    avg_length=$(echo "$stats_response" | jq '.avg_response_length // 0')
    success "Stats retrieved - Total: $total, Avg Length: $avg_length"
    
    echo "  Stats structure:"
    echo "$stats_response" | jq . 2>/dev/null | sed 's/^/    /'
else
    error "Invalid JSON structure in stats"
fi

print_summary
