#!/bin/bash

# Quick Delete Conversation Participants Script
# Deletes all conversation participants for cleanup

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting Quick Delete for Conversation Participants"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Quick Delete Conversation Participants"
echo "========================================================================"

# Function to delete participant by ID
delete_participant_by_id() {
    local participant_id="$1"
    local participant_name="$2"
    
    if [ -z "$participant_id" ]; then
        warn "No participant ID provided for $participant_name"
        return 1
    fi
    
    log "Deleting participant: $participant_name (ID: $participant_id)"
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X DELETE \
        "$API_BASE_URL/conversation/participants/$participant_id")
    
    status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [ "$status" = "200" ]; then
        success "Deleted $participant_name successfully"
    elif [ "$status" = "404" ]; then
        warn "$participant_name not found (already deleted or never existed)"
    else
        error "Failed to delete $participant_name - Status: $status"
        echo "  Response: $body"
    fi
}

# Function to delete participant by phone number
delete_participant_by_phone() {
    local phone_number="$1"
    local participant_name="$2"
    
    if [ -z "$phone_number" ]; then
        warn "No phone number provided for $participant_name"
        return 1
    fi
    
    log "Looking up participant by phone: $phone_number"
    
    # Get all participants and find by phone number
    response=$(curl -s "$API_BASE_URL/conversation/participants")
    if echo "$response" | jq . >/dev/null 2>&1; then
        participant_id=$(echo "$response" | jq -r ".result[] | select(.phoneNumber == \"$phone_number\") | .id")
        
        if [ -n "$participant_id" ] && [ "$participant_id" != "null" ]; then
            delete_participant_by_id "$participant_id" "$participant_name ($phone_number)"
        else
            warn "No participant found with phone number: $phone_number"
        fi
    else
        error "Failed to retrieve participant list"
    fi
}

echo
echo "=================================================="
echo "PHASE 1: Delete Known Test Participants"
echo "=================================================="

# Test variables - Use same pattern as enrollment script
CURRENT_TIME=$(date +%s)
PARTICIPANT_1_PHONE="${PARTICIPANT_1_PHONE:-+1555${CURRENT_TIME}001}"
PARTICIPANT_2_PHONE="${PARTICIPANT_2_PHONE:-+1555${CURRENT_TIME}002}"
PARTICIPANT_3_PHONE="${PARTICIPANT_3_PHONE:-+1555${CURRENT_TIME}003}"

# Delete the main test participants
delete_participant_by_phone "$PARTICIPANT_1_PHONE" "Alice Smith (Psychology Student)"
delete_participant_by_phone "$PARTICIPANT_2_PHONE" "Marcus Johnson (Software Engineer)"
delete_participant_by_phone "$PARTICIPANT_3_PHONE" "Elena Rodriguez (Art Teacher)"

echo
echo "=================================================="
echo "PHASE 2: Delete All Remaining Test Participants"
echo "=================================================="

log "Retrieving all conversation participants for cleanup..."
response=$(curl -s "$API_BASE_URL/conversation/participants")

if echo "$response" | jq . >/dev/null 2>&1; then
    participant_count=$(echo "$response" | jq '.result | length')
    log "Found $participant_count conversation participants"
    
    if [ "$participant_count" -gt 0 ]; then
        echo "  Participant list:"
        echo "$response" | jq -r '.result[] | "    - \(.name // "Unnamed") (\(.phoneNumber)) - ID: \(.id)"'
        
        echo
        read -p "Delete ALL these conversation participants? (y/N): " confirm
        
        if [[ $confirm =~ ^[Yy]$ ]]; then
            log "Deleting all conversation participants..."
            
            # Extract all participant IDs and delete them
            echo "$response" | jq -r '.result[] | .id' | while read -r participant_id; do
                if [ -n "$participant_id" ]; then
                    participant_name=$(echo "$response" | jq -r ".result[] | select(.id == \"$participant_id\") | .name // \"Unnamed\"")
                    participant_phone=$(echo "$response" | jq -r ".result[] | select(.id == \"$participant_id\") | .phoneNumber")
                    delete_participant_by_id "$participant_id" "$participant_name ($participant_phone)"
                    sleep 0.1  # Brief pause between deletions
                fi
            done
        else
            log "Skipping bulk deletion"
        fi
    else
        success "No conversation participants found - nothing to delete"
    fi
else
    error "Failed to retrieve conversation participants list"
fi

echo
echo "=================================================="
echo "PHASE 3: Verification"
echo "=================================================="

log "Verifying deletion - checking remaining participants..."
response=$(curl -s "$API_BASE_URL/conversation/participants")

if echo "$response" | jq . >/dev/null 2>&1; then
    remaining_count=$(echo "$response" | jq '.result | length')
    
    if [ "$remaining_count" -eq 0 ]; then
        success "All conversation participants deleted successfully"
    else
        warn "Still found $remaining_count conversation participants:"
        echo "$response" | jq -r '.result[] | "    - \(.name // "Unnamed") (\(.phoneNumber)) - ID: \(.id)"'
    fi
else
    error "Failed to verify deletion"
fi

echo
echo "=================================================="
echo "DELETE SUMMARY"
echo "=================================================="

log "Quick delete conversation participants completed"
log "All test conversation participants should now be removed"
log "You can verify by running: curl -s $API_BASE_URL/conversation/participants | jq ."

print_summary
