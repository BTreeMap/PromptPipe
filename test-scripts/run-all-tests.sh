#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting Complete PromptPipe API Test Suite"

# Check if API is running
if ! check_api; then
    warn "Please start PromptPipe first with: ./PromptPipe -api-addr :8080"
    exit 1
fi

echo
echo "🚀 Running comprehensive end-to-end tests..."
echo "API Base URL: $API_BASE_URL"
echo "Test Phone 1: $TEST_PHONE"
echo "Test Phone 2: $TEST_PHONE_2"
echo

# Reset counters
export TESTS_PASSED=0
export TESTS_FAILED=0

# Run all test suites
test_suites=(
    "test-send.sh"
    "test-genai.sh"
    "test-schedule.sh" 
    "test-responses.sh"
    "test-receipts.sh"
    "test-intervention.sh"
)

for suite in "${test_suites[@]}"; do
    echo
    echo "▶️  Running $suite..."
    echo "----------------------------------------"
    
    if [ -f "$(dirname "$0")/$suite" ]; then
        # Run the test suite and capture its results
        bash "$(dirname "$0")/$suite"
        suite_exit_code=$?
        
        if [ $suite_exit_code -eq 0 ]; then
            success "✅ $suite completed successfully"
        else
            error "❌ $suite had failures"
        fi
    else
        error "Test suite $suite not found"
    fi
    
    echo "----------------------------------------"
done

# Additional integration tests
echo
echo "🔄 Running integration workflow tests..."
echo "=========================================="

# Test workflow: Send -> Check receipts -> Record response -> Check stats
log "Testing complete message workflow..."

# 1. Send a message
send_response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{"to": "'$TEST_PHONE'", "type": "static", "body": "Integration test message"}' \
    "$API_BASE_URL/send")

send_status=$(echo "$send_response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
if [ "$send_status" = "200" ]; then
    success "Step 1: Message sent successfully"
    
    # 2. Wait and check receipts
    sleep 1
    receipts=$(curl -s "$API_BASE_URL/receipts")
    if echo "$receipts" | jq . >/dev/null 2>&1; then
        receipt_count=$(echo "$receipts" | jq '. | length')
        success "Step 2: Retrieved $receipt_count receipts"
    else
        error "Step 2: Failed to retrieve receipts"
    fi
    
    # 3. Simulate a response
    current_time=$(date +%s)
    response_result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{"from": "'$TEST_PHONE'", "body": "Integration test response", "time": '$current_time'}' \
        "$API_BASE_URL/response")
    
    response_status=$(echo "$response_result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    if [ "$response_status" = "201" ]; then
        success "Step 3: Response recorded successfully"
        
        # 4. Check updated stats
        stats=$(curl -s "$API_BASE_URL/stats")
        if echo "$stats" | jq . >/dev/null 2>&1; then
            total_responses=$(echo "$stats" | jq '.total_responses // 0')
            success "Step 4: Stats updated - Total responses: $total_responses"
        else
            error "Step 4: Failed to retrieve stats"
        fi
    else
        error "Step 3: Failed to record response"
    fi
else
    error "Step 1: Failed to send message"
fi

# Test different prompt types in sequence
log "Testing all prompt types..."

prompt_types=("static" "branch" "genai" "custom")
for prompt_type in "${prompt_types[@]}"; do
    case "$prompt_type" in
        "static")
            data='{"to": "'$TEST_PHONE'", "type": "static", "body": "Static test message"}'
            ;;
        "branch")
            data='{"to": "'$TEST_PHONE'", "type": "branch", "body": "Choose:", "branch_options": [{"label": "A", "body": "Option A"}, {"label": "B", "body": "Option B"}]}'
            ;;
        "genai")
            data='{"to": "'$TEST_PHONE'", "type": "genai", "body": "Generate message", "system_prompt": "You are helpful", "user_prompt": "Say hello"}'
            ;;
        "custom")
            data='{"to": "'$TEST_PHONE'", "type": "custom", "body": "Custom message", "state": "initial"}'
            ;;
    esac
    
    result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$data" \
        "$API_BASE_URL/send")
    
    status=$(echo "$result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    if [ "$status" = "200" ]; then
        success "Prompt type '$prompt_type' works correctly"
    else
        error "Prompt type '$prompt_type' failed with status $status"
    fi
done

# Test error handling
log "Testing comprehensive error scenarios..."

# Test malformed JSON
malformed_result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{"malformed": json}' \
    "$API_BASE_URL/send")

malformed_status=$(echo "$malformed_result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
if [ "$malformed_status" = "400" ]; then
    success "Malformed JSON properly rejected"
else
    error "Malformed JSON not properly handled"
fi

# Test rate limiting behavior (send multiple requests quickly)
log "Testing rapid requests (basic load test)..."
rapid_success=0
for i in {1..5}; do
    result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{"to": "'$TEST_PHONE'", "type": "static", "body": "Rapid test '$i'"}' \
        "$API_BASE_URL/send")
    
    status=$(echo "$result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    if [ "$status" = "200" ]; then
        ((rapid_success++))
    fi
done

if [ $rapid_success -eq 5 ]; then
    success "Handled 5 rapid requests successfully"
else
    warn "Only $rapid_success/5 rapid requests succeeded"
fi

echo
echo "🏁 Final Test Summary"
echo "===================="

# Final data verification
final_receipts=$(curl -s "$API_BASE_URL/receipts" | jq '. | length' 2>/dev/null || echo "0")
final_responses=$(curl -s "$API_BASE_URL/responses" | jq '. | length' 2>/dev/null || echo "0")
final_stats=$(curl -s "$API_BASE_URL/stats" 2>/dev/null)

echo "Final State:"
echo "  📧 Total Receipts: $final_receipts"
echo "  💬 Total Responses: $final_responses"

if [ -n "$final_stats" ] && echo "$final_stats" | jq . >/dev/null 2>&1; then
    total=$(echo "$final_stats" | jq '.total_responses // 0')
    avg_len=$(echo "$final_stats" | jq '.avg_response_length // 0')
    echo "  📊 Stats - Total: $total, Avg Length: $avg_len"
fi

print_summary
