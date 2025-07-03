# Conversation Flow

The conversation flow enables persistent conversational interactions with users using AI-powered responses. Unlike other flows that have predefined states and branching logic, the conversation flow maintains a continuous dialogue history and generates contextual responses using the configured LLM.

## Overview

The conversation flow:
- Maintains conversation history for each participant
- Uses a system prompt to guide AI behavior
- Integrates with the existing GenAI client for response generation
- Stores conversation state using the standard state management system
- Automatically truncates history to prevent unbounded growth

## Architecture

The conversation flow follows the same pattern as other complex flows like micro health intervention:

- **ConversationFlow**: Main flow implementation with state management
- **ConversationMessage**: Individual message structure (user/assistant)
- **ConversationHistory**: Complete conversation history for a participant
- **System Prompt**: Configurable AI behavior guidance loaded from file

## System Prompt

The system prompt is loaded from `prompts/conversation_system.txt` and guides the AI's behavior. If the file doesn't exist, a default prompt is created automatically.

To customize the AI behavior, edit this file with your preferred instructions.

## Usage

The conversation flow is automatically registered when the API starts if GenAI is configured. To use it:

1. Send a prompt with `type: "conversation"`
2. Include a `userPrompt` field with the user's message
3. The system will maintain conversation history and generate contextual responses

Example API call:
```json
{
  "to": "+1234567890",
  "type": "conversation",
  "userPrompt": "Hello, how are you today?"
}
```

## State Management

The conversation flow uses these states:
- `CONVERSATION_ACTIVE`: Normal conversation state

And these data keys:
- `conversationHistory`: JSON-serialized conversation history
- `systemPrompt`: System prompt content (if needed)

## History Management

- Conversation history is automatically maintained
- History is limited to the last 50 messages to prevent unbounded growth
- Context provided to the AI includes the last 20 messages (10 exchanges)
- All messages include timestamps for potential future use

## Dependencies

The conversation flow requires:
- Configured GenAI client for response generation
- Store backend for state persistence
- System prompt file (created automatically if missing)

## Integration

The conversation flow integrates seamlessly with the existing PromptPipe architecture:
- Uses the same state management as other flows
- Leverages existing GenAI integration
- Works with the standard messaging system
- Follows the same validation patterns
