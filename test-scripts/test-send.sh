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

# Test 3: GenAI - Daily Motivation with Personalization
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a personalized daily motivation",
    "system_prompt": "You are an expert life coach and motivational speaker. Your responses should be warm, encouraging, and actionable. Always include a specific action item.",
    "user_prompt": "Create a motivational message for someone starting their day. Include: 1) A positive affirmation, 2) A growth mindset insight, 3) One small actionable step they can take today. Keep it under 150 words and make it feel personal and genuine."
}' "200" "Send GenAI daily motivation"

# Test 4: GenAI - Creative Writing with Constraints
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a micro-story with specific constraints",
    "system_prompt": "You are a master of micro-fiction and creative writing. You excel at creating complete, emotionally resonant stories in very few words.",
    "user_prompt": "Write a complete story in exactly 50 words that: 1) Takes place in a coffee shop, 2) Involves an unexpected discovery, 3) Has a twist ending, 4) Evokes a strong emotion. Count the words carefully and ensure it is exactly 50 words."
}' "200" "Send GenAI creative micro-story"

# Test 5: GenAI - Technical Explanation with Analogies
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Explain a complex technical concept using analogies",
    "system_prompt": "You are a brilliant teacher who excels at making complex technical concepts accessible to everyone. You use vivid analogies and everyday examples that anyone can understand.",
    "user_prompt": "Explain how machine learning works using only analogies to cooking and recipes. Make it engaging and accurate, but ensure someone with no technical background could understand it. Include at least 3 specific cooking analogies. Keep it under 200 words."
}' "200" "Send GenAI technical explanation"

# Test 6: GenAI - Wellness Check-in with Emotional Intelligence
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate an empathetic wellness check-in message",
    "system_prompt": "You are a compassionate wellness coach with expertise in mental health and emotional intelligence. Your tone is warm, non-judgmental, and supportive. You ask thoughtful questions and provide gentle guidance.",
    "user_prompt": "Create a wellness check-in message for someone who might be having a challenging week. Include: 1) A gentle acknowledgment of life'\''s difficulties, 2) A mindful breathing or grounding exercise, 3) A thoughtful question that encourages self-reflection, 4) A reminder of their inner strength. Make it feel like a caring friend checking in."
}' "200" "Send GenAI wellness check-in"

# Test 7: GenAI - Educational Content with Interactive Elements
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Create an interactive learning snippet",
    "system_prompt": "You are an engaging educator who creates interactive and memorable learning experiences. You use questions, examples, and practical applications to make learning stick.",
    "user_prompt": "Teach me one fascinating fact about the ocean that most people don'\''t know. Structure it as: 1) A surprising hook/question, 2) The amazing fact with vivid description, 3) Why this matters to everyday life, 4) A follow-up question to encourage further thinking. Make it feel like a mini adventure of discovery."
}' "200" "Send GenAI educational content"

# Test 8: GenAI - Problem-Solving with Structured Thinking
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Generate a structured problem-solving framework",
    "system_prompt": "You are a strategic thinking expert and problem-solving consultant. You break down complex challenges into manageable steps and provide frameworks that people can actually use.",
    "user_prompt": "Create a simple but powerful decision-making framework for when someone feels overwhelmed by choices. Include: 1) A 3-step process they can follow, 2) Key questions to ask at each step, 3) A real-world example of how to apply it, 4) One warning about common decision-making traps. Make it practical and memorable."
}' "200" "Send GenAI problem-solving framework"

# Test 9: GenAI - Cultural Insight with Global Perspective
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Share a cultural insight with global perspective",
    "system_prompt": "You are a cultural anthropologist and world traveler with deep respect for diverse traditions and perspectives. You help people understand and appreciate cultural differences while finding universal human connections.",
    "user_prompt": "Share one beautiful tradition from any culture around the world that demonstrates human kindness or community connection. Include: 1) What the tradition is and where it comes from, 2) Why it'\''s meaningful to that culture, 3) What universal human need it addresses, 4) How someone might apply its wisdom in their own life, regardless of their background."
}' "200" "Send GenAI cultural insight"

# Test 10: GenAI - Future Visioning with Optimistic Realism
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Create an inspiring yet realistic vision",
    "system_prompt": "You are a futurist and innovation strategist who combines optimism with realistic assessment. You help people envision positive futures while acknowledging current challenges.",
    "user_prompt": "Paint a picture of one small but meaningful way technology might improve daily life in the next 5 years. Be specific and realistic, not sci-fi. Include: 1) The current problem/friction it solves, 2) How the technology might work simply, 3) Why this particular improvement matters for human wellbeing, 4) One thoughtful consideration about potential challenges. Make it hopeful but grounded."
}' "200" "Send GenAI future visioning"

# Test 11: Invalid phone number
test_endpoint "POST" "/send" '{
    "to": "invalid-phone",
    "type": "static",
    "body": "This should fail"
}' "400" "Send with invalid phone number"

# Test 12: Missing required fields
test_endpoint "POST" "/send" '{
    "type": "static",
    "body": "Missing phone number"
}' "400" "Send with missing phone number"

# Test 13: Empty body
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "static",
    "body": ""
}' "400" "Send with empty body"

# Test 14: Invalid prompt type
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "invalid_type",
    "body": "Test message"
}' "400" "Send with invalid prompt type"

# Test 15: Wrong HTTP method
test_endpoint "GET" "/send" "" "405" "Send with GET method (should fail)"

# Test 16: Invalid JSON
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
