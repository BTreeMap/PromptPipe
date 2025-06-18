#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe GenAI Advanced Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "=========================================="
echo "Testing Advanced GenAI Prompt Engineering"
echo "=========================================="

# Test 1: Chain-of-Thought Reasoning
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Solve a problem using step-by-step reasoning",
    "system_prompt": "You are an expert problem solver who thinks through challenges step by step. You show your reasoning process clearly and arrive at well-thought-out conclusions.",
    "user_prompt": "A person has 3 cups. Cup A has 8 ounces of coffee, Cup B has 5 ounces of milk, and Cup C is empty. They want to end up with equal amounts of liquid in all 3 cups without measuring tools. Think through this step-by-step: 1) State the goal clearly, 2) List what we know, 3) Work through the solution step by step, 4) Verify the answer makes sense."
}' "200" "GenAI Chain-of-Thought Reasoning"

# Test 2: Role-Based Response with Constraints
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Respond as a specific character with strict constraints",
    "system_prompt": "You are a wise, elderly librarian who has worked in the same small town library for 50 years. You speak in a gentle, thoughtful manner and always relate things back to books or stories you'\''ve encountered. You never use modern slang and prefer classic references.",
    "user_prompt": "Someone asks you for advice about dealing with a difficult coworker. Respond in character with: 1) A greeting that fits your personality, 2) A relevant story or book reference, 3) Practical wisdom drawn from literature, 4) A gentle closing thought. Keep it authentic to your character throughout."
}' "200" "GenAI Role-Based Character Response"

# Test 3: Multi-Perspective Analysis
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Analyze a situation from multiple viewpoints",
    "system_prompt": "You are skilled at seeing situations from multiple perspectives and helping people understand different viewpoints without taking sides. You present balanced analysis while remaining helpful and constructive.",
    "user_prompt": "A small neighborhood wants to build a community garden on an empty lot, but some residents prefer a playground instead. Present this situation from 3 different perspectives: 1) Parents with young children, 2) Elderly residents who want to garden, 3) Busy working adults. For each perspective, explain their priorities and concerns. Then suggest one creative solution that addresses multiple viewpoints."
}' "200" "GenAI Multi-Perspective Analysis"

# Test 4: Creative Constraints Challenge
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Create content with multiple creative constraints",
    "system_prompt": "You are a creative writing expert who thrives on constraints and limitations as fuel for creativity. You see constraints as puzzles that lead to innovative solutions.",
    "user_prompt": "Write a product review for a time machine, but with these constraints: 1) Exactly 75 words, 2) Must include the words '\''Tuesday'\'', '\''umbrella'\'', and '\''disappointed'\'', 3) Written from the perspective of someone who only traveled 3 minutes into the past, 4) Must sound like a genuine Amazon review with star rating, 5) Include one unexpected plot twist. Make it entertaining and believable within the absurd premise."
}' "200" "GenAI Creative Constraints Challenge"

# Test 5: Socratic Method Teaching
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Teach using the Socratic method",
    "system_prompt": "You are a master teacher who uses the Socratic method - teaching through thoughtful questions rather than direct answers. You guide people to discover insights themselves through carefully crafted questions.",
    "user_prompt": "Someone wants to understand why plants are green. Instead of explaining directly, use the Socratic method: 1) Start with a question that gets them thinking about what they observe, 2) Ask follow-up questions that guide them toward the concept of photosynthesis, 3) Help them connect the dots through their own reasoning, 4) Ask a final question that helps them see the bigger picture. Make it feel like a natural conversation."
}' "200" "GenAI Socratic Method Teaching"

# Test 6: Emotional Intelligence in Communication
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Demonstrate high emotional intelligence in communication",
    "system_prompt": "You are a master communicator with exceptional emotional intelligence. You read between the lines, acknowledge emotions, and respond with empathy while still being helpful and constructive.",
    "user_prompt": "Someone writes: '\''I guess I'\''ll just cancel my vacation plans since my friend can'\''t come anymore. It'\''s fine, I didn'\''t really want to go anyway.'\'' Respond with high emotional intelligence: 1) Acknowledge what they'\''re really feeling (beyond their words), 2) Validate their disappointment without pushing, 3) Gently explore if they might still enjoy the trip, 4) Offer support regardless of their decision. Be warm but not overly pushy."
}' "200" "GenAI Emotional Intelligence Communication"

# Test 7: Systems Thinking Approach
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Apply systems thinking to a complex problem",
    "system_prompt": "You are a systems thinking expert who sees connections, patterns, and feedback loops that others miss. You help people understand how different parts of a system interact and influence each other.",
    "user_prompt": "A small business owner says their employees seem unmotivated lately. Apply systems thinking: 1) Identify at least 4 different factors that might be interconnected, 2) Show how these factors might influence each other in feedback loops, 3) Suggest where small changes might have big impacts, 4) Explain why looking at isolated factors won'\''t solve the real problem. Make it practical and actionable."
}' "200" "GenAI Systems Thinking Analysis"

# Test 8: Analogical Reasoning Mastery
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Master analogical reasoning for complex explanation",
    "system_prompt": "You are brilliant at creating perfect analogies that make complex concepts crystal clear. Your analogies are not just similar - they map precisely onto the structure of the original concept.",
    "user_prompt": "Explain cryptocurrency and blockchain using only analogies from a small town community. Your analogy should accurately represent: 1) What blockchain technology actually is, 2) How cryptocurrency transactions work, 3) Why it'\''s considered secure, 4) What mining means in this context. Make sure every part of your town analogy corresponds exactly to a real aspect of crypto. Keep it complete but understandable."
}' "200" "GenAI Analogical Reasoning Mastery"

# Test 9: Metacognitive Awareness
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Demonstrate metacognitive awareness in response",
    "system_prompt": "You have strong metacognitive abilities - you'\''re aware of your own thinking process and can explain not just what you think, but how and why you think it. You model good thinking habits.",
    "user_prompt": "Someone asks: '\''Should I quit my job to start a business?'\'' Demonstrate metacognitive awareness by: 1) Explaining what type of question this is and why it'\''s complex, 2) Describing the thinking process you would use to approach it, 3) Identifying what information would be needed for a good decision, 4) Showing how you would weigh different factors, 5) Explaining why you can'\''t give a simple yes/no answer. Model good decision-making thinking."
}' "200" "GenAI Metacognitive Awareness"

# Test 10: Cross-Cultural Communication Bridge
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "Bridge cultural communication differences",
    "system_prompt": "You are an expert in cross-cultural communication who helps people understand and navigate cultural differences with sensitivity and respect. You build bridges between different cultural perspectives.",
    "user_prompt": "Two business partners from different cultural backgrounds are having misunderstandings about meeting styles. One prefers direct, time-focused meetings; the other values relationship-building and context in discussions. Help bridge this gap: 1) Explain each cultural approach respectfully, 2) Show the strengths of both styles, 3) Suggest a hybrid approach that honors both perspectives, 4) Give specific practical tips for their next meeting. Be culturally sensitive throughout."
}' "200" "GenAI Cross-Cultural Communication"

echo
echo "========================================"
echo "Testing GenAI Error Handling"
echo "========================================"

# Test 11: GenAI without system prompt
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "This should fail",
    "user_prompt": "Test message"
}' "400" "GenAI missing system prompt"

# Test 12: GenAI without user prompt
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "This should fail",
    "system_prompt": "You are a helpful assistant"
}' "400" "GenAI missing user prompt"

# Test 13: GenAI with empty prompts
test_endpoint "POST" "/send" '{
    "to": "'$TEST_PHONE'",
    "type": "genai",
    "body": "This should fail",
    "system_prompt": "",
    "user_prompt": ""
}' "400" "GenAI with empty prompts"

print_summary
