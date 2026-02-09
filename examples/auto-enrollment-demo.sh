#!/bin/bash

# Example: Running PromptPipe with Auto-Enrollment Enabled
# 
# This script demonstrates how to enable the auto-enrollment feature
# for new users in PromptPipe.

echo "==================================================================="
echo "PromptPipe Auto-Enrollment Feature - Example"
echo "==================================================================="
echo ""

# Option 1: Using environment variable
echo "Option 1: Enable auto-enrollment via environment variable"
echo "-------------------------------------------------------------------"
echo "export AUTO_ENROLL_NEW_USERS=true"
echo "./build/promptpipe"
echo ""

# Option 2: Using command line flag
echo "Option 2: Enable auto-enrollment via command line flag"
echo "-------------------------------------------------------------------"
echo "./build/promptpipe --auto-enroll-new-users=true"
echo ""

# Option 3: Using .env file
echo "Option 3: Enable auto-enrollment via .env file"
echo "-------------------------------------------------------------------"
echo "Add to .env file:"
echo "  AUTO_ENROLL_NEW_USERS=true"
echo ""
echo "Then run:"
echo "  ./build/promptpipe"
echo ""

# Demonstration
echo "==================================================================="
echo "Demonstration"
echo "==================================================================="
echo ""

# Check if build exists
if [ ! -f "./build/promptpipe" ]; then
    echo "Building PromptPipe..."
    make build
    if [ $? -ne 0 ]; then
        echo "Build failed. Please run 'make build' manually."
        exit 1
    fi
fi

# Ask user if they want to run with auto-enrollment enabled
read -p "Do you want to run PromptPipe with auto-enrollment enabled? (y/n): " response

if [ "$response" = "y" ] || [ "$response" = "Y" ]; then
    echo ""
    echo "Starting PromptPipe with auto-enrollment ENABLED..."
    echo ""
    echo "When a user sends their first message, they will be:"
    echo "  1. Automatically enrolled with an empty profile"
    echo "  2. Added to the conversation flow"
    echo "  3. Handled by the intake/feedback flow logic"
    echo ""
    echo "Press Ctrl+C to stop"
    echo ""
    
    # Set environment variable and run
    export AUTO_ENROLL_NEW_USERS=true
    ./build/promptpipe
else
    echo ""
    echo "Starting PromptPipe with auto-enrollment DISABLED (default)..."
    echo ""
    echo "Users must be manually enrolled via the API endpoint:"
    echo "  POST /conversation/participants"
    echo ""
    echo "Press Ctrl+C to stop"
    echo ""
    
    ./build/promptpipe
fi
