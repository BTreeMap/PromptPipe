#!/bin/bash

# Load configuration
source "$(dirname "$0")/config.sh"

log "Starting PromptPipe API Receipt Tests"

# Check if API is running
if ! check_api; then
    exit 1
fi

echo
echo "=================================="
echo "Testing Receipt Tracking"
echo "=================================="

# Current timestamp
CURRENT_TIME=$(date +%s)

# First, send some messages to generate receipts
log "Sending messages to generate receipts..."

# Send a few test messages (these should generate receipts automatically)
curl -s -X POST -H "Content-Type: application/json" \
    -d '{"to": "'$TEST_PHONE'", "type": "static", "body": "Receipt test 1"}' \
    "$API_BASE_URL/send" >/dev/null

curl -s -X POST -H "Content-Type: application/json" \
    -d '{"to": "'$TEST_PHONE_2'", "type": "static", "body": "Receipt test 2"}' \
    "$API_BASE_URL/send" >/dev/null

# Wait a moment for receipts to be processed
sleep 1

# Test 1: Get all receipts
test_endpoint "GET" "/receipts" "" "200" "Retrieve all message receipts"

# Test 2: Wrong HTTP method
test_endpoint "POST" "/receipts" "" "405" "POST to /receipts (should fail)"

test_endpoint "DELETE" "/receipts" "" "405" "DELETE /receipts (should fail)"

# Verify receipt data structure
log "Verifying receipt data structure..."
response=$(curl -s "$API_BASE_URL/receipts")
if echo "$response" | jq . >/dev/null 2>&1; then
    count=$(echo "$response" | jq '. | length')
    success "Retrieved $count receipts with valid JSON structure"
    
    # Show sample receipt structure if any exist
    if [ "$count" -gt 0 ]; then
        echo "  Sample receipt structure:"
        echo "$response" | jq '.[0]' 2>/dev/null | sed 's/^/    /'
        
        # Verify required fields exist
        sample_receipt=$(echo "$response" | jq '.[0]')
        
        if echo "$sample_receipt" | jq -e '.to' >/dev/null 2>&1; then
            success "Receipt has 'to' field"
        else
            error "Receipt missing 'to' field"
        fi
        
        if echo "$sample_receipt" | jq -e '.status' >/dev/null 2>&1; then
            status=$(echo "$sample_receipt" | jq -r '.status')
            success "Receipt has status: $status"
        else
            error "Receipt missing 'status' field"
        fi
        
        if echo "$sample_receipt" | jq -e '.time' >/dev/null 2>&1; then
            success "Receipt has 'time' field"
        else
            error "Receipt missing 'time' field"
        fi
        
        # Check if status is one of the expected values
        status=$(echo "$sample_receipt" | jq -r '.status')
        case "$status" in
            "sent"|"delivered"|"read"|"failed"|"error"|"scheduled"|"cancelled")
                success "Receipt status '$status' is valid"
                ;;
            *)
                warn "Receipt status '$status' might be unexpected"
                ;;
        esac
    else
        warn "No receipts found - this might be expected if messages haven't been processed yet"
    fi
else
    error "Invalid JSON structure in receipts response"
fi

# Test receipt filtering/querying by examining the data
log "Analyzing receipt patterns..."
response=$(curl -s "$API_BASE_URL/receipts")
if echo "$response" | jq . >/dev/null 2>&1; then
    # Count receipts by status
    sent_count=$(echo "$response" | jq '[.[] | select(.status == "sent")] | length')
    delivered_count=$(echo "$response" | jq '[.[] | select(.status == "delivered")] | length')
    failed_count=$(echo "$response" | jq '[.[] | select(.status == "failed")] | length')
    
    echo "  Receipt breakdown:"
    echo "    Sent: $sent_count"
    echo "    Delivered: $delivered_count" 
    echo "    Failed: $failed_count"
    
    # Count receipts by recipient
    if [ "$sent_count" -gt 0 ] || [ "$delivered_count" -gt 0 ]; then
        echo "  Recipients:"
        echo "$response" | jq -r '.[].to' | sort | uniq -c | sed 's/^/    /'
    fi
fi

print_summary
