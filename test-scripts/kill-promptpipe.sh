#!/bin/bash

# Script to kill promptpipe process with SIGINT signal
PROCESS_NAME="promptpipe"

# Check if process exists (exact match)
if pgrep -x "$PROCESS_NAME" > /dev/null; then
    echo "Found $PROCESS_NAME process. Sending SIGINT signal..."
    pkill -x -SIGINT "$PROCESS_NAME"
    
    # Wait a moment and check if process was terminated
    sleep 2
    if pgrep -x "$PROCESS_NAME" > /dev/null; then
        echo "Warning: $PROCESS_NAME process may still be running"
        exit 1
    else
        echo "Successfully terminated $PROCESS_NAME"
        exit 0
    fi
else
    echo "No $PROCESS_NAME process found"
    exit 1
fi
