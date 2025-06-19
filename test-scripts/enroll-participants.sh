#!/bin/bash

# Enroll Participants 1-3 Script
# This script enrolls participants 1, 2, and 3

# Load configuration
source "$(dirname "$0")/config.sh"

log "Enrolling participants 1, 2, and 3"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Enroll Participants 1-3"
echo "========================================================================"

# Test variables - Use unique phone numbers
CURRENT_TIME=$(date +%s)
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555${CURRENT_TIME}001}"
PARTICIPANT_2_PHONE="${PARTICIPANT_2_PHONE:-+1555${CURRENT_TIME}002}"
PARTICIPANT_3_PHONE="${PARTICIPANT_3_PHONE:-+1555${CURRENT_TIME}003}"

# Function to enroll a participant
enroll_participant() {
    local phone="$1"
    local name="$2"
    local participant_num="$3"
    
    log "Enrolling participant $participant_num with phone: $phone"
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "phone_number": "'$phone'",
            "name": "'$name'",
            "timezone": "America/Toronto"
        }' \
        "$API_BASE_URL/intervention/participants")
    
    status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [ "$status" = "201" ]; then
        success "Successfully enrolled participant $participant_num - Status: $status"
        
        # Extract participant ID
        participant_id=$(echo "$body" | jq -r '.result.id' 2>/dev/null || echo "")
        if [ -n "$participant_id" ]; then
            log "Participant $participant_num ID: $participant_id"
            log "Phone: $phone"
            log "📱 Check $phone for welcome message"
            
            # Store the participant ID in a variable for later use
            eval "PARTICIPANT_${participant_num}_ID=\"$participant_id\""
            eval "PARTICIPANT_${participant_num}_PHONE=\"$phone\""
        else
            warn "Could not extract participant ID from response for participant $participant_num"
        fi
        
        echo "Response details:"
        echo "$body" | jq . 2>/dev/null | sed 's/^/  /' || echo "  $body"
        echo
        return 0
    else
        error "Failed to enroll participant $participant_num - Expected: 201, Got: $status"
        echo "Error details: $body"
        echo
        return 1
    fi
}

# Enroll participant 1
enroll_participant "$PARTICIPANT_1_PHONE" "Test Participant One" "1" || exit 1

# Enroll participant 2
enroll_participant "$PARTICIPANT_2_PHONE" "Test Participant Two" "2" || exit 1

# Enroll participant 3
enroll_participant "$PARTICIPANT_3_PHONE" "Test Participant Three" "3" || exit 1

echo
log "All participants enrolled successfully!"
log "Participant 1: $PARTICIPANT_1_PHONE (ID: $PARTICIPANT_1_ID)"
log "Participant 2: $PARTICIPANT_2_PHONE (ID: $PARTICIPANT_2_ID)"
log "Participant 3: $PARTICIPANT_3_PHONE (ID: $PARTICIPANT_3_ID)"
