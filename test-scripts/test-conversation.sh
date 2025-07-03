#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe Conversation Enrollment Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Testing Conversation Participant Enrollment & Flow"
echo "========================================================================"

# Test variables - Use unique phone numbers for each test run
CURRENT_TIME=$(date +%s)
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555${CURRENT_TIME}001}"
PARTICIPANT_2_PHONE="${PARTICIPANT_2_PHONE:-+1555${CURRENT_TIME}002}"
PARTICIPANT_3_PHONE="${PARTICIPANT_3_PHONE:-+1555${CURRENT_TIME}003}"

# Global variables for participant IDs (extracted from responses)
PARTICIPANT_1_ID=""
PARTICIPANT_2_ID=""
PARTICIPANT_3_ID=""

# Helper function to extract participant ID from JSON response
extract_participant_id() {
    local response="$1"
    echo "$response" | jq -r '.result.id' 2>/dev/null || echo ""
}

echo
echo "=================================================="
echo "PHASE 1: Conversation Participant Enrollment"
echo "=================================================="

# Test 1: Enroll first participant - Psychology Student
log "Testing conversation participant enrollment - Psychology Student..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phoneNumber": "'$PARTICIPANT_1_PHONE'",
        "name": "Alice Smith",
        "gender": "female",
        "ethnicity": "Hispanic",
        "background": "College student studying psychology, interested in mental health topics and mindfulness practices"
    }' \
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
        PARTICIPANT_1_ID=$(echo "$response" | jq -r ".result[] | select(.phoneNumber == \"$PARTICIPANT_1_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_2_ID" ]; then
        PARTICIPANT_2_ID=$(echo "$response" | jq -r ".result[] | select(.phoneNumber == \"$PARTICIPANT_2_PHONE\") | .id")
    fi
    if [ -z "$PARTICIPANT_3_ID" ]; then
        PARTICIPANT_3_ID=$(echo "$response" | jq -r ".result[] | select(.phoneNumber == \"$PARTICIPANT_3_PHONE\") | .id")
    fi
    
    echo "  Participant 1 (Alice - Psychology): $PARTICIPANT_1_ID"
    echo "  Participant 2 (Marcus - Software): $PARTICIPANT_2_ID"
    echo "  Participant 3 (Elena - Art Teacher): $PARTICIPANT_3_ID"
    
    echo "  Full participant list:"
    echo "$response" | jq '.result' | sed 's/^/    /'
else
    error "List conversation participants - Invalid JSON response"
fi

# Test 5: Get specific participants with full details
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Getting full details for Alice (Psychology Student)..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_1_ID" "" "200" "Get Alice's details"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Getting full details for Marcus (Software Engineer)..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_2_ID" "" "200" "Get Marcus's details"
fi

if [ -n "$PARTICIPANT_3_ID" ]; then
    log "Getting full details for Elena (Art Teacher)..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_3_ID" "" "200" "Get Elena's details"
fi

echo
echo "=================================================="
echo "PHASE 3: Conversation Flow Testing"
echo "=================================================="

# Test 6: Update participant details to test personalization
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Testing participant update for Alice..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_1_ID" '{
        "background": "Psychology student with special interest in cognitive behavioral therapy and anxiety management techniques"
    }' "200" "Update Alice's background"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Testing participant update for Marcus..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_2_ID" '{
        "background": "Senior software engineer specializing in machine learning infrastructure, currently working on conversational AI systems"
    }' "200" "Update Marcus's background"
fi

# Test 7: Test various validation scenarios
log "Testing conversation enrollment validation..."

# Try to enroll duplicate participant
test_endpoint "POST" "/conversation/participants" '{
    "phoneNumber": "'$PARTICIPANT_1_PHONE'",
    "name": "Duplicate Alice"
}' "409" "Enroll duplicate participant (should fail)"

# Invalid phone number
test_endpoint "POST" "/conversation/participants" '{
    "phoneNumber": "invalid-phone",
    "name": "Invalid Phone User"
}' "400" "Enroll with invalid phone number"

# Missing phone number
test_endpoint "POST" "/conversation/participants" '{
    "name": "Missing Phone User"
}' "400" "Enroll without phone number"

# Empty name should still work
TEMP_PHONE="+155510$(date +%s)$RANDOM"
test_endpoint "POST" "/conversation/participants" '{
    "phoneNumber": "'$TEMP_PHONE'",
    "gender": "other"
}' "201" "Enroll with minimal info (no name)"

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
log "   1. Alice Smith ($PARTICIPANT_1_PHONE) - Psychology Student"
log "      ID: $PARTICIPANT_1_ID"
log "      Background: Psychology, mental health, mindfulness"
echo
log "   2. Marcus Johnson ($PARTICIPANT_2_PHONE) - Software Engineer" 
log "      ID: $PARTICIPANT_2_ID"
log "      Background: AI/ML, tech startup, conversational AI"
echo
log "   3. Elena Rodriguez ($PARTICIPANT_3_PHONE) - Art Teacher"
log "      ID: $PARTICIPANT_3_ID"
log "      Background: Art education, graphic design, creativity"
echo
log "💬 WHAT SHOULD HAPPEN NEXT:"
log "   • Each participant should have received a personalized welcome message"
log "   • The AI should tailor responses based on their backgrounds"
log "   • Alice's conversations should reference psychology/mental health"
log "   • Marcus's conversations should reference technology/AI topics"
log "   • Elena's conversations should reference art/creativity/teaching"
echo
log "🧪 TESTING SUGGESTIONS:"
log "   • Send messages from each phone number to test conversation flow"
log "   • Try different topics to see how AI adapts to each participant"
log "   • Test how conversation history is maintained across exchanges"
echo
log "🧹 CLEANUP:"
log "   • Run 'test-scripts/quick-delete-conversation-participants.sh' to remove all test participants"
log "   • Or delete individual participants via the DELETE API endpoints"

print_summary

if test_endpoint "GET" "/conversation/participants" "" "200" "List all enrolled participants"; then
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
fi

echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "TEST 9: Test Status Updates for Each Participant"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

statuses=("paused" "active" "inactive")

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
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "TEST 10: Clean Up - Delete Test Participants"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for participant_id in "${PARTICIPANT_IDS[@]}"; do
    test_endpoint "DELETE" "/conversation/participants/$participant_id" "" "200" "Delete participant $participant_id"
done

# Verify cleanup
echo
log "Verifying cleanup - checking participant list is empty or contains no test participants"
if test_endpoint "GET" "/conversation/participants" "" "200" "Verify cleanup"; then
    remaining_count=$(echo "$body" | jq '.result | length')
    log "Remaining participants after cleanup: $remaining_count"
    
    # Check if any of our test participants still exist
    cleanup_failed=false
    for participant_id in "${PARTICIPANT_IDS[@]}"; do
        if echo "$body" | jq -e ".result[] | select(.id == \"$participant_id\")" >/dev/null; then
            error "Cleanup failed: Participant $participant_id still exists"
            cleanup_failed=true
        fi
    done
    
    if [ "$cleanup_failed" = false ]; then
        success "Cleanup successful - all test participants removed"
    fi
fi

echo
echo "========================================================================"
echo "CONVERSATION ENROLLMENT TEST SUMMARY"
echo "========================================================================"
echo -e "Tests Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests Failed: ${RED}$TESTS_FAILED${NC}"
echo -e "Total Tests: $((TESTS_PASSED + TESTS_FAILED))"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ All conversation enrollment tests passed!${NC}"
    exit 0
else
    echo -e "${RED}❌ Some conversation enrollment tests failed!${NC}"
    exit 1
fi
