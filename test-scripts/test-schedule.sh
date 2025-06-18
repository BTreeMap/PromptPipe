#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe API Schedule Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "=================================="
echo "Testing POST /schedule endpoint"
echo "=================================="

# Test 1: Schedule static message
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "Scheduled message test",
    "cron": "0 9 * * *"
}' "201" "Schedule static message (9 AM daily)"

# Test 2: Schedule branch message
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "branch",
    "body": "Scheduled choice:",
    "branch_options": [
        {"label": "Morning", "body": "Good morning!"},
        {"label": "Evening", "body": "Good evening!"}
    ],
    "cron": "0 18 * * 1-5"
}' "201" "Schedule branch message (6 PM weekdays)"

# Test 3: Schedule GenAI message
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Daily motivation",
    "system_prompt": "You are a motivational coach",
    "user_prompt": "Generate a daily motivation quote",
    "cron": "0 8 * * *"
}' "201" "Schedule GenAI message (8 AM daily)"

# Test 4: Schedule with custom state
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "custom",
    "body": "Custom flow message",
    "state": "initial",
    "cron": "0 12 * * *"
}' "201" "Schedule custom message with state"

# Test 5: Missing cron expression
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "Missing cron"
}' "400" "Schedule without cron expression"

# Test 6: Invalid cron expression
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "Invalid cron test",
    "cron": "invalid cron"
}' "400" "Schedule with invalid cron expression"

# Test 7: Schedule every minute (valid but frequent)
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "Every minute test",
    "cron": "* * * * *"
}' "201" "Schedule every minute"

# Test 8: Schedule for specific date/time
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": "New Year message",
    "cron": "0 0 1 1 *"
}' "201" "Schedule for New Year (Jan 1st midnight)"

# Test 9: Schedule with invalid phone
test_endpoint "POST" "/schedule" '{
    "to": "invalid-phone",
    "type": "static",
    "body": "Test message",
    "cron": "0 9 * * *"
}' "400" "Schedule with invalid phone number"

# Test 10: Wrong HTTP method
test_endpoint "GET" "/schedule" "" "405" "Schedule with GET method (should fail)"

print_summary
