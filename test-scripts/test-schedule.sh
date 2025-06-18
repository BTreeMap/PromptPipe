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

# Test 3: Schedule GenAI message - Advanced Daily Coaching
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Daily personalized coaching",
    "system_prompt": "You are a professional life coach who provides personalized daily guidance. Your messages are always encouraging, actionable, and tailored to help someone build better habits and mindset.",
    "user_prompt": "Create a unique daily coaching message that includes: 1) A thought-provoking question for self-reflection, 2) One small habit suggestion for personal growth, 3) A mindfulness moment or gratitude prompt, 4) An encouraging closing thought. Make each message feel fresh and personal, never repetitive.",
    "cron": "0 8 * * *"
}' "201" "Schedule GenAI daily coaching (8 AM daily)"

# Test 4: Schedule GenAI message - Weekly Professional Development
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Weekly professional development insight",
    "system_prompt": "You are a career development expert and executive coach. You provide valuable insights about professional growth, leadership, and career advancement that are practical and immediately applicable.",
    "user_prompt": "Create a weekly professional development message with: 1) One key insight about career growth or leadership, 2) A specific skill or behavior to focus on this week, 3) A reflection question about professional goals, 4) A practical action step they can take immediately. Make it relevant for people at any career stage.",
    "cron": "0 9 * * 1"
}' "201" "Schedule GenAI weekly professional development (9 AM Mondays)"

# Test 5: Schedule GenAI message - Weekend Reflection and Planning
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Weekend reflection and weekly planning",
    "system_prompt": "You are a productivity coach and mindfulness expert who helps people reflect on their week and plan mindfully for the next one. You balance achievement with well-being.",
    "user_prompt": "Create a weekend reflection message that includes: 1) A gentle prompt to reflect on the week'\''s wins and lessons, 2) A question about what brought them joy or fulfillment, 3) One suggestion for how to recharge over the weekend, 4) A simple planning prompt for the upcoming week that focuses on priorities rather than just tasks.",
    "cron": "0 10 * * 6"
}' "201" "Schedule GenAI weekend reflection (10 AM Saturdays)"

# Test 6: Schedule GenAI message - Monthly Growth Check-in
test_endpoint "POST" "/schedule" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Monthly personal growth check-in",
    "system_prompt": "You are a wise mentor who helps people take a step back and assess their personal growth journey. You ask profound questions that lead to meaningful insights and help people stay aligned with their values and goals.",
    "user_prompt": "Create a monthly check-in message with: 1) A powerful question about personal growth over the past month, 2) A prompt to identify one thing they'\''ve learned about themselves, 3) An invitation to consider what they want to focus on next month, 4) A reminder about their strengths and potential. Make it feel like a conversation with a trusted mentor.",
    "cron": "0 9 1 * *"
}' "201" "Schedule GenAI monthly growth check-in (9 AM 1st of month)"

# Test 7: Schedule with custom state
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
