#!/bin/bash

# Delete All Intervention Participants Script
# This script removes all participants from the PromptPipe intervention system

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe Intervention Participant Cleanup"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "========================================================================"
echo "Delete All Intervention Participants"
echo "========================================================================"

# Function to confirm deletion
confirm_deletion() {
    echo
    warn "⚠️  WARNING: This will DELETE ALL intervention participants!"
    warn "⚠️  This action cannot be undone!"
    echo
    read -p "Are you sure you want to proceed? (type 'yes' to confirm): " confirmation
    
    if [ "$confirmation" != "yes" ]; then
        log "Operation cancelled by user"
        exit 0
    fi
}

# Function to list all participants and get their IDs
get_all_participant_ids() {
    log "Fetching all participants..."
    
    response=$(curl -s "$API_BASE_URL/intervention/participants")
    
    if ! echo "$response" | jq . >/dev/null 2>&1; then
        error "Failed to fetch participants - Invalid JSON response"
        echo "Response: $response"
        return 1
    fi
    
    # Extract participant IDs and basic info
    local participant_data=$(echo "$response" | jq -r '.result[] | "\(.id)|\(.name // "No Name")|\(.phone_number // "No Phone")"')
    
    if [ -z "$participant_data" ]; then
        log "No participants found in the system"
        return 0
    fi
    
    local count=$(echo "$response" | jq '.result | length')
    log "Found $count participants to delete"
    
    echo
    echo "Participants to be deleted:"
    echo "=================================="
    echo "$participant_data" | while IFS='|' read -r id name phone; do
        echo "  ID: $id"
        echo "  Name: $name"
        echo "  Phone: $phone"
        echo "  ──────────────────────"
    done
    echo
    
    # Return the IDs for deletion
    echo "$participant_data" | cut -d'|' -f1
}

# Function to delete a single participant
delete_participant() {
    local participant_id="$1"
    local participant_name="$2"
    local participant_phone="$3"
    
    log "Deleting participant: $participant_name ($participant_id)"
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X DELETE \
        "$API_BASE_URL/intervention/participants/$participant_id")
    
    # Extract HTTP status and body
    status=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [ "$status" = "200" ]; then
        success "Deleted participant: $participant_name ($participant_phone)"
        return 0
    elif [ "$status" = "404" ]; then
        warn "Participant not found (may have been already deleted): $participant_id"
        return 0
    else
        error "Failed to delete participant: $participant_name - Status: $status"
        if [ -n "$body" ]; then
            echo "  Error details: $body"
        fi
        return 1
    fi
}

# Function to verify deletion
verify_deletion() {
    log "Verifying all participants have been deleted..."
    
    response=$(curl -s "$API_BASE_URL/intervention/participants")
    
    if echo "$response" | jq . >/dev/null 2>&1; then
        count=$(echo "$response" | jq '.result | length')
        if [ "$count" = "0" ]; then
            success "Verification complete - No participants remaining"
        else
            warn "Verification warning - $count participants still exist"
            echo "Remaining participants:"
            echo "$response" | jq '.result[] | {id, name, phone_number}' | sed 's/^/  /'
        fi
    else
        error "Verification failed - Could not fetch participant list"
    fi
}

# Function to show final statistics
show_final_stats() {
    log "Fetching final intervention statistics..."
    
    response=$(curl -s "$API_BASE_URL/intervention/stats")
    if echo "$response" | jq . >/dev/null 2>&1; then
        total_participants=$(echo "$response" | jq '.result.total_participants')
        total_responses=$(echo "$response" | jq '.result.total_responses')
        log "Final statistics:"
        log "  Participants: $total_participants"
        log "  Total responses recorded: $total_responses"
    else
        warn "Could not fetch final statistics"
    fi
}

# Main execution flow
main() {
    # Get all participant IDs
    participant_ids=$(get_all_participant_ids)
    
    if [ -z "$participant_ids" ]; then
        log "No participants to delete. System is already clean."
        exit 0
    fi
    
    # Confirm deletion
    confirm_deletion
    
    # Delete each participant
    local deleted_count=0
    local failed_count=0
    
    echo "$participant_ids" | while read -r participant_line; do
        if [ -n "$participant_line" ]; then
            # Re-fetch participant info for this ID since we're in a subshell
            participant_info=$(curl -s "$API_BASE_URL/intervention/participants/$participant_line")
            if echo "$participant_info" | jq . >/dev/null 2>&1; then
                name=$(echo "$participant_info" | jq -r '.result.name // "Unknown"')
                phone=$(echo "$participant_info" | jq -r '.result.phone_number // "Unknown"')
                
                if delete_participant "$participant_line" "$name" "$phone"; then
                    ((deleted_count++))
                else
                    ((failed_count++))
                fi
            else
                # If we can't fetch details, try to delete anyway
                if delete_participant "$participant_line" "Unknown" "Unknown"; then
                    ((deleted_count++))
                else
                    ((failed_count++))
                fi
            fi
            
            # Small delay to avoid overwhelming the API
            sleep 0.1
        fi
    done
    
    echo
    log "Deletion process completed"
    
    # Verify deletion
    verify_deletion
    
    # Show final statistics
    show_final_stats
    
    echo
    echo "========================================================================"
    echo "Cleanup Summary"
    echo "========================================================================"
    success "All intervention participants have been deleted"
    log "The intervention system has been reset to a clean state"
}

# Add dry-run option
if [ "$1" = "--dry-run" ] || [ "$1" = "-n" ]; then
    log "DRY RUN MODE - No participants will be deleted"
    echo
    
    # Just list participants without deleting
    get_all_participant_ids
    
    echo
    log "Dry run completed. Use the script without --dry-run to perform actual deletion."
    exit 0
fi

# Add help option
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Delete All Intervention Participants Script"
    echo
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  --dry-run, -n    Show what would be deleted without actually deleting"
    echo "  --help, -h       Show this help message"
    echo
    echo "This script will:"
    echo "  1. List all participants in the intervention system"
    echo "  2. Ask for confirmation before proceeding"
    echo "  3. Delete all participants one by one"
    echo "  4. Verify the deletion was successful"
    echo "  5. Show final statistics"
    echo
    echo "Environment Variables:"
    echo "  API_BASE_URL     Base URL for the PromptPipe API (default: http://localhost:8080)"
    echo
    exit 0
fi

# Run main function
main
