#!/bin/bash

# Quick Delete All Intervention Participants Script
# This is a simplified version for automated cleanup

# Load configuration
source "$(dirname "$0")/config.sh"

# Check if API is running
if ! check_api; then
    exit 1
fi

log "Quick cleanup: Deleting all intervention participants..."

# Get all participant IDs
response=$(curl -s "$API_BASE_URL/intervention/participants")

if ! echo "$response" | jq . >/dev/null 2>&1; then
    error "Failed to fetch participants"
    exit 1
fi

participant_ids=$(echo "$response" | jq -r '.result[].id')

if [ -z "$participant_ids" ]; then
    log "No participants found - system is already clean"
    exit 0
fi

count=$(echo "$participant_ids" | wc -l)
log "Deleting $count participants..."

# Delete each participant
deleted=0
failed=0

for id in $participant_ids; do
    if [ -n "$id" ]; then
        status=$(curl -s -w "%{http_code}" -X DELETE "$API_BASE_URL/intervention/participants/$id" -o /dev/null)
        if [ "$status" = "200" ] || [ "$status" = "404" ]; then
            ((deleted++))
            echo -n "."
        else
            ((failed++))
            echo -n "x"
        fi
    fi
done

echo
log "Deletion complete: $deleted successful, $failed failed"

# Quick verification
final_count=$(curl -s "$API_BASE_URL/intervention/participants" | jq '.result | length' 2>/dev/null || echo "unknown")
log "Remaining participants: $final_count"
