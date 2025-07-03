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

# Test 1: Enroll first participant - Busy Parent
log "Testing conversation participant enrollment - Busy Parent..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_1_PHONE'",
        "name": "Alice Smith",
        "gender": "female",
        "ethnicity": "Hispanic",
        "background": "Working mother of two young children, struggles with finding time for self-care habits, wants to develop a consistent morning routine but often gets distracted by family needs"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_1_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_1_ID")
    success "Enroll Busy Parent - Status: $status, ID: $PARTICIPANT_1_ID"
else
    error "Enroll Busy Parent - Expected: 201, Got: $status"
    echo "Response: $body"
fi

# Test 2: Enroll second participant - Busy Professional
log "Testing conversation participant enrollment - Busy Professional..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_2_PHONE'",
        "name": "Marcus Johnson",
        "gender": "male",
        "ethnicity": "African American",
        "background": "Sales manager who travels frequently, wants to build consistent exercise habits but struggles with irregular schedule and hotel gyms, motivated by stress relief"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_2_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_2_ID")
    success "Enroll Busy Professional - Status: $status, ID: $PARTICIPANT_2_ID"
else
    error "Enroll Busy Professional - Expected: 201, Got: $status"
    echo "Response: $body"
fi

# Test 3: Enroll third participant - Graduate Student
log "Testing conversation participant enrollment - Graduate Student..."
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_3_PHONE'",
        "name": "Elena Rodriguez",
        "gender": "female", 
        "ethnicity": "Latina",
        "background": "Graduate student working on thesis, struggles with procrastination and wants to build better study habits, interested in mindfulness and productivity techniques"
    }' \
    "$API_BASE_URL/conversation/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    PARTICIPANT_3_ID=$(extract_participant_id "$body")
    PARTICIPANT_IDS+=("$PARTICIPANT_3_ID")
    success "Enroll Graduate Student - Status: $status, ID: $PARTICIPANT_3_ID"
else
    error "Enroll Graduate Student - Expected: 201, Got: $status"
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
    
    echo "  Participant 1 (Alice - Busy Parent): $PARTICIPANT_1_ID"
    echo "  Participant 2 (Marcus - Busy Professional): $PARTICIPANT_2_ID"
    echo "  Participant 3 (Elena - Graduate Student): $PARTICIPANT_3_ID"
    
    echo "  Full participant list:"
    echo "$response" | jq '.result' | sed 's/^/    /'
else
    error "List conversation participants - Invalid JSON response"
fi

# Test 5: Get specific participants with full details
if [ -n "$PARTICIPANT_1_ID" ]; then
    log "Getting full details for Alice (Busy Parent)..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_1_ID" "" "200" "Get Alice's details"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Getting full details for Marcus (Busy Professional)..."
    test_endpoint "GET" "/conversation/participants/$PARTICIPANT_2_ID" "" "200" "Get Marcus's details"
fi

if [ -n "$PARTICIPANT_3_ID" ]; then
    log "Getting full details for Elena (Graduate Student)..."
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
        "background": "Working mother of two young children, struggles with finding time for self-care habits, wants to develop a consistent morning routine but often gets distracted by family needs. Interested in micro-habits that can fit into busy parenting schedule."
    }' "200" "Update Alice's background"
fi

if [ -n "$PARTICIPANT_2_ID" ]; then
    log "Testing participant update for Marcus..."
    test_endpoint "PUT" "/conversation/participants/$PARTICIPANT_2_ID" '{
        "background": "Sales manager who travels frequently, wants to build consistent exercise habits but struggles with irregular schedule and hotel gyms, motivated by stress relief. Looking for portable fitness routines and habits that work on the road."
    }' "200" "Update Marcus's background"
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

# Empty name should still work
TEMP_PHONE="+155510$(date +%s)$RANDOM"
test_endpoint "POST" "/conversation/participants" '{
    "phone_number": "'$TEMP_PHONE'",
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
log "   1. Alice Smith ($PARTICIPANT_1_PHONE) - Busy Parent"
log "      ID: $PARTICIPANT_1_ID"
log "      Background: Working mother, morning routines, micro-habits"
echo
log "   2. Marcus Johnson ($PARTICIPANT_2_PHONE) - Busy Professional" 
log "      ID: $PARTICIPANT_2_ID"
log "      Background: Sales manager, travel, exercise habits, stress relief"
echo
log "   3. Elena Rodriguez ($PARTICIPANT_3_PHONE) - Graduate Student"
log "      ID: $PARTICIPANT_3_ID"
log "      Background: Thesis work, procrastination, study habits, mindfulness"
echo
log "💬 WHAT SHOULD HAPPEN NEXT:"
log "   • Each participant should have received a personalized welcome message"
log "   • The AI should tailor responses based on their backgrounds"
log "   • Alice's conversations should reference parenting/family time management"
log "   • Marcus's conversations should reference travel/business routines"
log "   • Elena's conversations should reference study habits/productivity"
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
echo "========================================================================"
echo "CONVERSATION ENROLLMENT TEST SUMMARY"
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
