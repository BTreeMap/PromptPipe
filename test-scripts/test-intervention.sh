#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe Micro Health Intervention API Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Testing Stateful Micro Health Intervention API"
echo "========================================================================"

# Test variables
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555100001}"
PARTICIPANT_2_PHONE="${PARTICIPANT_2_PHONE:-+1555100002}"
PARTICIPANT_3_PHONE="${PARTICIPANT_3_PHONE:-+1555100003}"
CURRENT_TIME=$(date +%s)

# Global variables for participant IDs (extracted from responses)
PARTICIPANT_1_ID=""
PARTICIPANT_2_ID=""
PARTICIPANT_3_ID=""

# Helper function to extract participant ID from JSON response
extract_participant_id() {
    local response="$1"
    echo "$response" | jq -r '.result.id' 2>/dev/null || echo ""
}

# Helper function to extract current state from JSON response
extract_current_state() {
    local response="$1"
    echo "$response" | jq -r '.current_state' 2>/dev/null || echo ""
}

echo
echo "=================================================="
echo "PHASE 1: Participant Enrollment Tests"
echo "=================================================="

# Test 1: Enroll first participant with full details
log "Testing participant enrollment with full details..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_1_PHONE'",
        "name": "Test Participant One",
        "timezone": "America/New_York",
        "daily_prompt_time": "09:00"
    }' \
    "$API_BASE_URL/intervention/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    success "Enroll participant with full details - Status: $status"
    PARTICIPANT_1_ID=$(extract_participant_id "$body")
    if [ -n "$PARTICIPANT_1_ID" ]; then
        log "Extracted participant ID: $PARTICIPANT_1_ID"
    else
        error "Failed to extract participant ID from response"
    fi
    echo "  Response: $body" | jq . 2>/dev/null | sed 's/^/    /'
else
    error "Enroll participant with full details - Expected: 201, Got: $status"
fi

# Test 2: Enroll second participant with minimal details  
test_endpoint "POST" "/intervention/participants" '{
    "phone_number": "'$PARTICIPANT_2_PHONE'"
}' "201" "Enroll participant with minimal details"

# Extract participant ID for later tests
if [ $? -eq 0 ]; then
    response=$(curl -s -X POST -H "Content-Type: application/json" \
        -d '{"phone_number": "'$PARTICIPANT_2_PHONE'"}' \
        "$API_BASE_URL/intervention/participants" 2>/dev/null)
    # This will fail with 409 since we just created it, but we need to get the ID from list
fi

# Test 3: Try to enroll duplicate participant
test_endpoint "POST" "/intervention/participants" '{
    "phone_number": "'$PARTICIPANT_1_PHONE'",
    "name": "Duplicate Participant"
}' "409" "Enroll duplicate participant (should fail)"

# Test 4: Invalid phone number
test_endpoint "POST" "/intervention/participants" '{
    "phone_number": "invalid-phone",
    "name": "Invalid Phone"
}' "400" "Enroll with invalid phone number"

# Test 5: Missing phone number
test_endpoint "POST" "/intervention/participants" '{
    "name": "Missing Phone"
}' "400" "Enroll without phone number"

# Test 6: Invalid timezone
test_endpoint "POST" "/intervention/participants" '{
    "phone_number": "'$PARTICIPANT_3_PHONE'",
    "timezone": "Invalid/Timezone"
}' "400" "Enroll with invalid timezone"

# Test 7: Invalid daily prompt time
test_endpoint "POST" "/intervention/participants" '{
    "phone_number": "'$PARTICIPANT_3_PHONE'",
    "daily_prompt_time": "25:00"
}' "400" "Enroll with invalid daily prompt time"

echo
echo "=================================================="
echo "PHASE 2: Participant Retrieval and Management"
echo "=================================================="

# Test 8: List all participants
log "Testing participant listing..."
response=$(curl -s "$API_BASE_URL/intervention/participants")
if echo "$response" | jq . >/dev/null 2>&1; then
    count=$(echo "$response" | jq '.result | length')
    success "List participants - Retrieved $count participants"
    
    # Extract participant IDs if we don't have them yet
    if [ -z "$PARTICIPANT_1_ID" ]; then
        PARTICIPANT_1_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_1_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_2_ID" ]; then
        PARTICIPANT_2_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_2_PHONE\") | .id")
    fi
    
    echo "  Participant 1 ID: $PARTICIPANT_1_ID"
    echo "  Participant 2 ID: $PARTICIPANT_2_ID"
else
    error "List participants - Invalid JSON response"
fi

# Test 9: Get specific participant
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "GET" "/intervention/participants/$PARTICIPANT_1_ID" "" "200" "Get specific participant"
else
    error "Cannot test get specific participant - no participant ID available"
fi

# Test 10: Get non-existent participant
test_endpoint "GET" "/intervention/participants/p_nonexistent123" "" "404" "Get non-existent participant"

echo
echo "=================================================="
echo "PHASE 3: State Management and Flow Testing"
echo "=================================================="

# Test 11: Get participant history (should show initial ORIENTATION state)
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Testing participant history retrieval..."
    response=$(curl -s "$API_BASE_URL/intervention/participants/$PARTICIPANT_1_ID/history")
    if echo "$response" | jq . >/dev/null 2>&1; then
        current_state=$(echo "$response" | jq -r '.result.current_state')
        response_count=$(echo "$response" | jq '.result.response_count')
        success "Get participant history - State: $current_state, Responses: $response_count"
    else
        error "Get participant history - Invalid JSON response"
    fi
else
    error "Cannot test participant history - no participant ID available"
fi

# Test 12: Advance participant state manually
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/advance" '{
        "to_state": "COMMITMENT_PROMPT",
        "reason": "Manual advancement for testing"
    }' "200" "Advance participant state to COMMITMENT_PROMPT"
fi

# Test 13: Try to advance to invalid state
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/advance" '{
        "to_state": "INVALID_STATE",
        "reason": "Testing invalid state"
    }' "400" "Advance to invalid state (should fail)"
fi

# Test 14: Advance without to_state
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/advance" '{
        "reason": "Missing to_state"
    }' "400" "Advance without to_state (should fail)"
fi

echo
echo "=================================================="
echo "PHASE 4: Response Processing and Flow Progression"
echo "=================================================="

# Test 15: Process commitment response
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/responses" '{
        "response_text": "1",
        "context": "WhatsApp message"
    }' "201" "Process commitment response"
fi

# Test 16: Advance to FEELING_PROMPT and test feeling response
if [ -n "$PARTICIPANT_1_ID" ]; then
    # First advance to feeling prompt
    curl -s -X POST -H "Content-Type: application/json" \
        -d '{"to_state": "FEELING_PROMPT", "reason": "Test progression"}' \
        "$API_BASE_URL/intervention/participants/$PARTICIPANT_1_ID/advance" >/dev/null
    
    # Then process feeling response
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/responses" '{
        "response_text": "3",
        "context": "Feeling motivated"
    }' "201" "Process feeling response"
fi

# Test 17: Test habit completion response
if [ -n "$PARTICIPANT_1_ID" ]; then
    # Advance to HABIT_REMINDER
    curl -s -X POST -H "Content-Type: application/json" \
        -d '{"to_state": "HABIT_REMINDER", "reason": "Test habit reminder"}' \
        "$API_BASE_URL/intervention/participants/$PARTICIPANT_1_ID/advance" >/dev/null
    
    # Process habit completion response
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/responses" '{
        "response_text": "1",
        "context": "Completed habit"
    }' "201" "Process habit completion response"
fi

# Test 18: Process response with empty text (should fail)
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/responses" '{
        "response_text": "",
        "context": "Empty response"
    }' "400" "Process empty response (should fail)"
fi

# Test 19: Process response for non-existent participant
test_endpoint "POST" "/intervention/participants/p_nonexistent123/responses" '{
    "response_text": "test",
    "context": "Non-existent participant"
}' "404" "Process response for non-existent participant"

echo
echo "=================================================="
echo "PHASE 5: Complete Flow Simulation"
echo "=================================================="

# Test 20: Complete full flow simulation for participant 2
if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Starting complete flow simulation for participant 2..."
    
    # Step 1: Check initial state (should be ORIENTATION)
    response=$(curl -s "$API_BASE_URL/intervention/participants/$PARTICIPANT_2_ID/history")
    initial_state=$(echo "$response" | jq -r '.result.current_state')
    log "Initial state: $initial_state"
    
    # Step 2: Progress through each state with responses
    states=("COMMITMENT_PROMPT" "FEELING_PROMPT" "RANDOM_ASSIGNMENT" "HABIT_REMINDER" "FOLLOW_UP" "COMPLETE")
    responses=("2" "1" "" "3" "2" "")
    
    for i in "${!states[@]}"; do
        state="${states[$i]}"
        response_text="${responses[$i]}"
        
        # Advance to state
        advance_result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -d "{\"to_state\": \"$state\", \"reason\": \"Flow simulation step $((i+1))\"}" \
            "$API_BASE_URL/intervention/participants/$PARTICIPANT_2_ID/advance")
        
        advance_status=$(echo "$advance_result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
        if [ "$advance_status" = "200" ]; then
            success "Flow simulation: Advanced to $state"
            
            # Process response if we have response text
            if [ -n "$response_text" ]; then
                response_result=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
                    -H "Content-Type: application/json" \
                    -d "{\"response_text\": \"$response_text\", \"context\": \"Flow simulation\"}" \
                    "$API_BASE_URL/intervention/participants/$PARTICIPANT_2_ID/responses")
                
                response_status=$(echo "$response_result" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
                if [ "$response_status" = "201" ]; then
                    success "Flow simulation: Processed response '$response_text' in $state"
                else
                    error "Flow simulation: Failed to process response in $state"
                fi
            fi
        else
            error "Flow simulation: Failed to advance to $state"
        fi
        
        # Brief pause between states
        sleep 0.5
    done
    
    # Check final history
    final_history=$(curl -s "$API_BASE_URL/intervention/participants/$PARTICIPANT_2_ID/history")
    final_state=$(echo "$final_history" | jq -r '.result.current_state')
    final_response_count=$(echo "$final_history" | jq '.result.response_count')
    log "Flow simulation complete - Final state: $final_state, Total responses: $final_response_count"
fi

echo
echo "=================================================="
echo "PHASE 6: Participant Reset and State Management"
echo "=================================================="

# Test 21: Reset participant state
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "POST" "/intervention/participants/$PARTICIPANT_1_ID/reset" "" "200" "Reset participant state"
    
    # Verify state was reset to ORIENTATION
    sleep 0.5
    response=$(curl -s "$API_BASE_URL/intervention/participants/$PARTICIPANT_1_ID/history")
    if echo "$response" | jq . >/dev/null 2>&1; then
        reset_state=$(echo "$response" | jq -r '.result.current_state')
        if [ "$reset_state" = "ORIENTATION" ]; then
            success "Verify reset - State correctly reset to ORIENTATION"
        else
            error "Verify reset - State not reset correctly, got: $reset_state"
        fi
    fi
fi

# Test 22: Reset non-existent participant
test_endpoint "POST" "/intervention/participants/p_nonexistent123/reset" "" "404" "Reset non-existent participant"

echo
echo "=================================================="
echo "PHASE 7: Statistics and Analytics"
echo "=================================================="

# Test 23: Get intervention statistics
log "Testing intervention statistics..."
response=$(curl -s "$API_BASE_URL/intervention/stats")
if echo "$response" | jq . >/dev/null 2>&1; then
    total_participants=$(echo "$response" | jq '.result.total_participants')
    total_responses=$(echo "$response" | jq '.result.total_responses')
    completion_rate=$(echo "$response" | jq '.result.completion_rate')
    success "Get intervention stats - Participants: $total_participants, Responses: $total_responses, Completion rate: $completion_rate%"
    
    echo "  Full statistics:"
    echo "$response" | jq '.result' | sed 's/^/    /'
else
    error "Get intervention stats - Invalid JSON response"
fi

# Test 24: Trigger weekly summary
test_endpoint "POST" "/intervention/weekly-summary" "" "200" "Trigger weekly summary"

echo
echo "=================================================="
echo "PHASE 8: Error Handling and Edge Cases"
echo "=================================================="

# Test 25: Wrong HTTP methods
test_endpoint "GET" "/intervention/participants/$PARTICIPANT_1_ID/responses" "" "405" "GET responses endpoint (should fail)"
test_endpoint "DELETE" "/intervention/participants/$PARTICIPANT_1_ID/responses" "" "405" "DELETE responses endpoint (should fail)"
test_endpoint "PUT" "/intervention/participants/$PARTICIPANT_1_ID/advance" "" "405" "PUT advance endpoint (should fail)"
test_endpoint "GET" "/intervention/weekly-summary" "" "405" "GET weekly-summary endpoint (should fail)"

# Test 26: Invalid JSON payloads
log "Testing invalid JSON payloads..."

# Invalid JSON for enrollment
invalid_json_test() {
    local endpoint="$1"
    local method="$2"
    local test_name="$3"
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" \
        -H "Content-Type: application/json" \
        -d '{"invalid": json syntax}' \
        "$API_BASE_URL$endpoint")
    
    status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    if [ "$status" = "400" ]; then
        success "$test_name - Status: $status"
    else
        error "$test_name - Expected: 400, Got: $status"
    fi
}

invalid_json_test "/intervention/participants" "POST" "Invalid JSON for enrollment"
if [ -n "$PARTICIPANT_1_ID" ]; then
    invalid_json_test "/intervention/participants/$PARTICIPANT_1_ID/responses" "POST" "Invalid JSON for response"
    invalid_json_test "/intervention/participants/$PARTICIPANT_1_ID/advance" "POST" "Invalid JSON for advance"
fi

echo
echo "=================================================="
echo "PHASE 9: Comprehensive Data Validation"
echo "=================================================="

# Test 27: Comprehensive phone number validation
phone_validation_tests() {
    local phones=(
        "123456789:400"           # Too short
        "+1234567890123456:400"   # Too long
        "not-a-phone:400"         # Invalid format
        "++1234567890:400"        # Double plus
        "+1 (555) 123-4567:400"   # With formatting (might be valid depending on implementation)
    )
    
    for phone_test in "${phones[@]}"; do
        IFS=':' read -r phone expected_status <<< "$phone_test"
        test_endpoint "POST" "/intervention/participants" "{\"phone_number\": \"$phone\"}" "$expected_status" "Phone validation: $phone"
    done
}

phone_validation_tests

# Test 28: Timezone validation
timezone_tests=(
    "America/Invalid:400"
    "NotATimezone:400" 
    "UTC/Invalid:400"
    "":201                    # Empty timezone should default to UTC
)

for tz_test in "${timezone_tests[@]}"; do
    IFS=':' read -r timezone expected_status <<< "$tz_test"
    unique_phone="+155510$(date +%s)$RANDOM"
    
    if [ -z "$timezone" ]; then
        payload="{\"phone_number\": \"$unique_phone\"}"
    else
        payload="{\"phone_number\": \"$unique_phone\", \"timezone\": \"$timezone\"}"
    fi
    
    test_endpoint "POST" "/intervention/participants" "$payload" "$expected_status" "Timezone validation: '$timezone'"
done

echo
echo "=================================================="
echo "PHASE 10: Cleanup and Final Tests"
echo "=================================================="

# Test 29: Delete participants
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "DELETE" "/intervention/participants/$PARTICIPANT_1_ID" "" "200" "Delete participant 1"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    test_endpoint "DELETE" "/intervention/participants/$PARTICIPANT_2_ID" "" "200" "Delete participant 2"
fi

# Test 30: Verify deletion
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "GET" "/intervention/participants/$PARTICIPANT_1_ID" "" "404" "Verify participant 1 deletion"
fi

# Test 31: Try to delete already deleted participant
if [ -n "$PARTICIPANT_1_ID" ]; then
    test_endpoint "DELETE" "/intervention/participants/$PARTICIPANT_1_ID" "" "404" "Delete already deleted participant"
fi

# Test 32: Final statistics check (should show fewer participants)
log "Final statistics check..."
response=$(curl -s "$API_BASE_URL/intervention/stats")
if echo "$response" | jq . >/dev/null 2>&1; then
    final_participants=$(echo "$response" | jq '.result.total_participants')
    success "Final stats - Remaining participants: $final_participants"
else
    error "Final stats check failed"
fi

echo
echo "=================================================="
echo "INTERVENTION API TEST SUMMARY"
echo "=================================================="

log "Comprehensive intervention API testing completed"
log "Tested all major endpoints and error conditions"
log "Verified stateful flow progression and data validation" 
log "Confirmed proper error handling and response formats"

print_summary
