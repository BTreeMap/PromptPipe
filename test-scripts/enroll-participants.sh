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
DEFAULT_TIMEZONE="${PARTICIPANT_TIMEZONE:-America/Toronto}"

# Helper to get participant-specific fields from .env with defaults
get_participant_field() {
    local participant_num="$1"
    local field="$2"
    local default_value="$3"
    local var_name="PARTICIPANT_${participant_num}_${field}"
    local value="${!var_name}"

    if [ -z "$value" ]; then
        value="$default_value"
    fi

    echo "$value"
}

# Function to enroll a participant
enroll_participant() {
    local phone="$1"
    local participant_num="$2"

    local default_name
    case "$participant_num" in
        1) default_name="Test Participant One" ;;
        2) default_name="Test Participant Two" ;;
        3) default_name="Test Participant Three" ;;
        *) default_name="Test Participant $participant_num" ;;
    esac

    local name
    local gender
    local ethnicity
    local background
    local timezone

    name="$(get_participant_field "$participant_num" "NAME" "$default_name")"
    gender="$(get_participant_field "$participant_num" "GENDER" "")"
    ethnicity="$(get_participant_field "$participant_num" "ETHNICITY" "")"
    background="$(get_participant_field "$participant_num" "BACKGROUND" "")"
    timezone="$(get_participant_field "$participant_num" "TIMEZONE" "$DEFAULT_TIMEZONE")"

    log "Enrolling participant $participant_num with phone: $phone (name: $name)"

    payload=$(jq -n \
        --arg phone_number "$phone" \
        --arg name "$name" \
        --arg gender "$gender" \
        --arg ethnicity "$ethnicity" \
        --arg background "$background" \
        --arg timezone "$timezone" \
        '{
            phone_number: $phone_number
        }
        + (if ($name | length) > 0 then {name: $name} else {} end)
        + (if ($gender | length) > 0 then {gender: $gender} else {} end)
        + (if ($ethnicity | length) > 0 then {ethnicity: $ethnicity} else {} end)
        + (if ($background | length) > 0 then {background: $background} else {} end)
        + (if ($timezone | length) > 0 then {timezone: $timezone} else {} end)
        ')

    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "$API_BASE_URL/conversation/participants")
    
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
enroll_participant "$PARTICIPANT_1_PHONE" "1" || exit 1

# Enroll participant 2
enroll_participant "$PARTICIPANT_2_PHONE" "2" || exit 1

# Enroll participant 3
enroll_participant "$PARTICIPANT_3_PHONE" "3" || exit 1

echo
log "All participants enrolled successfully!"
log "Participant 1: $PARTICIPANT_1_PHONE (ID: $PARTICIPANT_1_ID)"
log "Participant 2: $PARTICIPANT_2_PHONE (ID: $PARTICIPANT_2_ID)"
log "Participant 3: $PARTICIPANT_3_PHONE (ID: $PARTICIPANT_3_ID)"
