#!/bin/bash

# Enroll Participant 1 Script
# This script enrolls only PARTICIPANT_1_PHONE and does nothing else

# Load configuration
source "$(dirname "$0")/config.sh"

log "Enrolling PARTICIPANT_1_PHONE only"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Enroll Participant 1"
echo "========================================================================"

# Test variables - Use unique phone number
CURRENT_TIME=$(date +%s)
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555${CURRENT_TIME}001}"

log "Enrolling participant with phone: $PARTICIPANT_1_PHONE"

# Enroll participant 1
response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "phone_number": "'$PARTICIPANT_1_PHONE'",
        "name": "Test Participant One",
        "timezone": "America/Toronto"
    }' \
    "$API_BASE_URL/intervention/participants")

status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')

if [ "$status" = "201" ]; then
    success "Successfully enrolled participant - Status: $status"
    
    # Extract participant ID
    PARTICIPANT_1_ID=$(echo "$body" | jq -r '.result.id' 2>/dev/null || echo "")
    if [ -n "$PARTICIPANT_1_ID" ]; then
        log "Participant ID: $PARTICIPANT_1_ID"
        log "Phone: $PARTICIPANT_1_PHONE"
        log "📱 Check $PARTICIPANT_1_PHONE for welcome message"
    else
        warn "Could not extract participant ID from response"
    fi
    
    echo "Response details:"
    echo "$body" | jq . 2>/dev/null | sed 's/^/  /' || echo "  $body"
else
    error "Failed to enroll participant - Expected: 201, Got: $status"
    echo "Error details: $body"
    exit 1
fi

echo
log "Enrollment completed successfully"
