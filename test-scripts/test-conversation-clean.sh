#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe Conversation Enrollment Tests (Clean Version)"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Testing Conversation Participant Enrollment & Flow (Clean - Phone + Name Only)"
echo "========================================================================"

# Test variables - Use unique phone numbers and names for each test run
CURRENT_TIME=$(date +%s)
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555${CURRENT_TIME}001}"
PARTICIPANT_2_PHONE="${PARTICIPANT_2_PHONE:-+1555${CURRENT_TIME}002}"
PARTICIPANT_3_PHONE="${PARTICIPANT_3_PHONE:-+1555${CURRENT_TIME}003}"
PARTICIPANT_4_PHONE="${PARTICIPANT_4_PHONE:-+1555${CURRENT_TIME}004}"
PARTICIPANT_1_NAME="${PARTICIPANT_1_NAME:-Alice Smith}"
PARTICIPANT_2_NAME="${PARTICIPANT_2_NAME:-Marcus Johnson}"
PARTICIPANT_3_NAME="${PARTICIPANT_3_NAME:-Elena Rodriguez}"
PARTICIPANT_4_NAME="${PARTICIPANT_4_NAME:-David Chen}"

# Global variables for participant IDs (extracted from responses)
PARTICIPANT_1_ID=""
PARTICIPANT_2_ID=""
PARTICIPANT_3_ID=""
PARTICIPANT_4_ID=""
PARTICIPANT_IDS=()  # Array to store all participant IDs for bulk operations

# Helper function to extract participant ID from JSON response
extract_participant_id() {
    local response="$1"
    echo "$response" | jq -r '.result.id' 2>/dev/null || echo ""
}

echo
echo "=================================================="
echo "PHASE 1: Conversation Participant Enrollment"
echo "=================================================="

# Test 1: Enroll first participant
log "Testing conversation participant enrollment - Participant 1..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_1_PHONE'",
        "name": "'$PARTICIPANT_1_NAME'"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_1_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_1_ID")
    success "Enroll Participant 1 - Status: $status, ID: $PARTICIPANT_1_ID"
else
    error "Enroll Participant 1 - Expected: 201, Got: $status"
    echo "Response: $body"
fi

# Test 2: Enroll second participant
log "Testing conversation participant enrollment - Participant 2..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_2_PHONE'",
        "name": "'$PARTICIPANT_2_NAME'"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_2_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_2_ID")
    success "Enroll Participant 2 - Status: $status, ID: $PARTICIPANT_2_ID"
else
    error "Enroll Participant 2 - Expected: 201, Got: $status"
    echo "Response: $body"
fi

# Test 3: Enroll third participant
log "Testing conversation participant enrollment - Participant 3..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_3_PHONE'",
        "name": "'$PARTICIPANT_3_NAME'"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_3_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_3_ID")
    success "Enroll Participant 3 - Status: $status, ID: $PARTICIPANT_3_ID"
else
    error "Enroll Participant 3 - Expected: 201, Got: $status"
    echo "Response: $body"
fi

# Test 4: Enroll fourth participant
log "Testing conversation participant enrollment - Participant 4..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_4_PHONE'",
        "name": "'$PARTICIPANT_4_NAME'"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_4_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_4_ID")
    success "Enroll Participant 4 - Status: $status, ID: $PARTICIPANT_4_ID"
else
    error "Enroll Participant 4 - Expected: 201, Got: $status"
    echo "Response: $body"
fi

echo
echo "=================================================="
echo "PHASE 2: Participant Verification & Management"
echo "=================================================="

# Test 4: List all conversation participants
log "Testing conversation participant listing..."
response=$(curl -s "$API_BASE_URL/conversation/participants")
if echo "$response" | jq . >/dev/null 2>&1; then
    count=$(echo "$response" | jq '.result | length')
    success "List conversation participants - Retrieved $count participants"
    
    # Extract participant IDs if we don't have them yet
    if [ -z "$PARTICIPANT_1_ID" ]; then
        PARTICIPANT_1_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_1_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_2_ID" ]; then
        PARTICIPANT_2_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_2_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_3_ID" ]; then
        PARTICIPANT_3_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_3_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_4_ID" ]; then
        PARTICIPANT_4_ID=$(echo "$response" | jq -r ".result[] | select(.phone_number == \"$PARTICIPANT_4_PHONE\") | .id")
    fi
    
    echo "  Participant 1 ($PARTICIPANT_1_NAME): $PARTICIPANT_1_ID"
    echo "  Participant 2 ($PARTICIPANT_2_NAME): $PARTICIPANT_2_ID"
    echo "  Participant 3 ($PARTICIPANT_3_NAME): $PARTICIPANT_3_ID"
    echo "  Participant 4 ($PARTICIPANT_4_NAME): $PARTICIPANT_4_ID"
    
    echo "  Full participant list:"
    echo "$response" | jq '.result' | sed 's/^/    /'
else
    error "List conversation participants - Invalid JSON response"
fi

# Test 5: Get specific participants with full details
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Getting full details for $PARTICIPANT_1_NAME..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_1_ID" "" "200" "Get $PARTICIPANT_1_NAME's details"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Getting full details for $PARTICIPANT_2_NAME..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_2_ID" "" "200" "Get $PARTICIPANT_2_NAME's details"
fi

if [ -n "$PARTICIPANT_3_ID" ]; then
    log "Getting full details for $PARTICIPANT_3_NAME..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_3_ID" "" "200" "Get $PARTICIPANT_3_NAME's details"
fi

if [ -n "$PARTICIPANT_4_ID" ]; then
    log "Getting full details for $PARTICIPANT_4_NAME..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_4_ID" "" "200" "Get $PARTICIPANT_4_NAME's details"
fi

echo
echo "=================================================="
echo "PHASE 3: Conversation Flow Testing"
echo "=================================================="

# Test 6: Update participant details (name only)
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Testing participant update for $PARTICIPANT_1_NAME..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_1_ID" '{
        "name": "'$PARTICIPANT_1_NAME'-Updated"
    }' "200" "Update $PARTICIPANT_1_NAME's name"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Testing participant update for $PARTICIPANT_2_NAME..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_2_ID" '{
        "name": "'$PARTICIPANT_2_NAME'-Updated"
    }' "200" "Update $PARTICIPANT_2_NAME's name"
fi

if [ -n "$PARTICIPANT_3_ID" ]; then
    log "Testing participant update for $PARTICIPANT_3_NAME..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_3_ID" '{
        "name": "'$PARTICIPANT_3_NAME'-Updated"
    }' "200" "Update $PARTICIPANT_3_NAME's name"
fi

# Test 7: Test various validation scenarios
log "Testing conversation enrollment validation..."

# Try to enroll duplicate participant
test_endpoint "POST" "/conversation/participants" '{
    "phone_number": "'$PARTICIPANT_1_PHONE'",
    "name": "Duplicate Alice"
}' "409" "Enroll duplicate participant (should fail)"

# Invalid phone number
test_endpoint "POST" "/conversation/participants" '{
    "phone_number": "invalid-phone",
    "name": "Invalid Phone User"
}' "400" "Enroll with invalid phone number"

# Missing phone number
test_endpoint "POST" "/conversation/participants" '{
    "name": "Missing Phone User"
}' "400" "Enroll without phone number"

# Test enrollment with phone number only (no name)
TEMP_PHONE="+155510$(date +%s)$RANDOM"
test_endpoint "POST" "/conversation/participants" '{
    "phone_number": "'$TEMP_PHONE'"
}' "201" "Enroll with phone number only (no name)"

echo
echo "=================================================="
echo "PHASE 4: Edge Cases and Error Handling"
echo "=================================================="

# Test 8: Non-existent participant operations
test_endpoint "GET" "/conversation/participants/conv_nonexistent123" "" "404" "Get non-existent participant"
test_endpoint "PUT" "/conversation/participants/conv_nonexistent123" '{
    "name": "Non-existent"
}' "404" "Update non-existent participant"
test_endpoint "DELETE" "/conversation/participants/conv_nonexistent123" "" "404" "Delete non-existent participant"

# Test 9: Invalid JSON payloads
log "Testing invalid JSON handling..."

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

invalid_json_test "/conversation/participants" "POST" "Invalid JSON for enrollment"
if [ -n "$PARTICIPANT_1_ID" ]; then
    invalid_json_test "/conversation/participants/$PARTICIPANT_1_ID" "PUT" "Invalid JSON for update"
fi

echo
echo "=================================================="
echo "CONVERSATION ENROLLMENT TEST SUMMARY"
echo "=================================================="

echo
log "🎉 Conversation participant enrollment testing completed!"
echo
log "📋 ENROLLED PARTICIPANTS:"
log "   1. $PARTICIPANT_1_NAME ($PARTICIPANT_1_PHONE)"
log "      ID: $PARTICIPANT_1_ID"
echo
log "   2. $PARTICIPANT_2_NAME ($PARTICIPANT_2_PHONE)" 
log "      ID: $PARTICIPANT_2_ID"
echo
log "   3. $PARTICIPANT_3_NAME ($PARTICIPANT_3_PHONE)"
log "      ID: $PARTICIPANT_3_ID"
echo
log "   4. $PARTICIPANT_4_NAME ($PARTICIPANT_4_PHONE)"
log "      ID: $PARTICIPANT_4_ID"
echo
log "💬 WHAT SHOULD HAPPEN NEXT:"
log "   • Each participant should have received a welcome message"
log "   • The AI should handle responses with minimal participant data"
log "   • All participants are enrolled with basic information only"
echo
log "🧪 TESTING SUGGESTIONS:"
log "   • Send messages from each phone number to test conversation flow"
log "   • Test how AI responds without detailed background information"
log "   • Verify conversation history is maintained across exchanges"
echo
log "🧹 CLEANUP:"
log "   • Run 'test-scripts/quick-delete-conversation-participants.sh' to remove all test participants"
log "   • Or delete individual participants via the DELETE API endpoints"

print_summary

log "Verifying all enrolled participants are accessible..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X GET "$API_BASE_URL/conversation/participants")
status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "200" ]; then
    success "List all enrolled participants - Status: $status"
    participant_count=$(echo "$body" | jq '.result | length')
    log "Found $participant_count participants in the system"
    
    # Verify our enrolled participants are in the list
    for participant_id in "${PARTICIPANT_IDS[@]}"; do
        if echo "$body" | jq -e ".result[] | select(.id == \"$participant_id\")" >/dev/null; then
            success "Participant $participant_id found in list"
        else
            error "Participant $participant_id NOT found in list"
        fi
    done
else
    error "List all enrolled participants - Expected: 200, Got: $status"
    echo "Response: $body"
fi

echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "TEST 10: Test Status Updates for Each Participant"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

statuses=("paused" "active" "inactive" "paused")

for i in "${!PARTICIPANT_IDS[@]}"; do
    if [ $i -lt ${#statuses[@]} ]; then
        participant_id="${PARTICIPANT_IDS[$i]}"
        status="${statuses[$i]}"
        
        update_data='{
            "status": "'$status'"
        }'
        
        test_endpoint "PUT" "/conversation/participants/$participant_id" "$update_data" "200" "Update participant $participant_id to $status"
    fi
done

echo
echo "========================================================================"
echo "CONVERSATION ENROLLMENT TEST SUMMARY (CLEAN VERSION)"
echo "========================================================================"
echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"
echo -e "Total Tests: $((TESTS_PASSED + TESTS_FAILED))"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ All conversation enrollment tests passed!${NC}"
else
    echo -e "${RED}❌ Some conversation enrollment tests failed!${NC}"
fi

echo
log "🧹 CLEANUP INSTRUCTIONS:"
log "   • Participants have been left in the system for manual testing"
log "   • Run 'test-scripts/quick-delete-conversation-participants.sh' when ready to clean up"
log "   • Or delete individual participants via DELETE API endpoints:"
for i in "${!PARTICIPANT_IDS[@]}"; do
    log "     curl -X DELETE $API_BASE_URL/conversation/participants/${PARTICIPANT_IDS[$i]}"
done

if [ $TESTS_FAILED -eq 0 ]; then
    exit 0
else
    exit 1
fi
