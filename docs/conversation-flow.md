# Conversation Flow

The conversation flow enables persistent conversational interactions with users using AI-powered responses. Similar to the micro health intervention flow, users must first enroll through the intervention API before engaging in conversations. The AI initiates the first message upon enrollment.

## Overview

The conversation flow:

- Requires user enrollment through the `/conversation/participants` API
- AI sends the first message automatically after enrollment  
- Maintains conversation history for each participant
- Uses a system prompt to guide AI behavior, with optional user background context
- Integrates with the existing GenAI client for response generation
- Stores conversation state using the standard state management system
- Automatically truncates history to prevent unbounded growth

## Architecture

The conversation flow follows the same pattern as other complex flows like micro health intervention:

- **ConversationFlow**: Main flow implementation with state management
- **ConversationParticipant**: User enrollment and profile data
- **ConversationMessage**: Individual message structure (user/assistant)
- **ConversationHistory**: Complete conversation history for a participant
- **System Prompt**: Configurable AI behavior guidance loaded from file

## Enrollment Process

Before engaging in conversations, users must be enrolled via the conversation API:

### 1. Enroll a Conversation Participant

```http
POST /conversation/participants
Content-Type: application/json

{
  "phoneNumber": "+1234567890",
  "name": "Alice Smith",
  "gender": "female",           // Optional: male, female, non-binary, other
  "ethnicity": "Hispanic",      // Optional: user's ethnic background
  "background": "College student studying psychology, interested in mental health topics"  // Optional
}
```

Response:

```json
{
  "status": "ok",
  "message": "Conversation participant enrolled successfully",
  "result": {
    "id": "conv_abc123",
    "phoneNumber": "+1234567890",
    "name": "Alice Smith",
    "status": "active"
  }
}
```

### 2. First Message Initiation

After successful enrollment:

- The AI automatically sends the first message to the user
- The system prompt is enhanced with user background information if provided
- The conversation begins immediately with AI-initiated contact

## System Prompt

The system prompt is loaded from `prompts/conversation_system.txt` and guides the AI's behavior. User background information (gender, ethnicity, background) is automatically appended to the system prompt when available.

Example enhanced prompt:

```text
You are a helpful AI assistant designed to engage in meaningful conversations about various topics...

User Context:
- Name: Alice Smith  
- Gender: female
- Ethnicity: Hispanic
- Background: College student studying psychology, interested in mental health topics

Please be culturally sensitive and tailor your responses appropriately to this user's background.
```

## API Endpoints

### Participant Management

- `POST /conversation/participants` - Enroll new participant
- `GET /conversation/participants` - List all participants  
- `GET /conversation/participants/{id}` - Get participant details
- `PUT /conversation/participants/{id}` - Update participant
- `DELETE /conversation/participants/{id}` - Remove participant

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
