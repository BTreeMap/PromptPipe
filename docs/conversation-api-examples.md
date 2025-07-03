# Conversation API Examples

This document provides practical examples of using the conversation API endpoints.

## 1. Enroll a New Conversation Participant

```bash
curl -X POST http://localhost:8080/conversation/participants \
  -H "Content-Type: application/json" \
  -d '{
    "phoneNumber": "+1234567890",
    "name": "Alice Smith",
    "gender": "female",
    "ethnicity": "Hispanic",
    "background": "College student studying psychology, interested in mental health topics"
  }'
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
    "gender": "female",
    "ethnicity": "Hispanic",
    "background": "College student studying psychology, interested in mental health topics",
    "status": "active",
    "enrolledAt": "2025-07-03T14:30:00Z"
  }
}
```

## 2. List All Conversation Participants

```bash
curl -X GET http://localhost:8080/conversation/participants
```

Response:

```json
{
  "status": "ok",
  "message": "Conversation participants retrieved successfully",
  "result": [
    {
      "id": "conv_abc123",
      "phoneNumber": "+1234567890",
      "name": "Alice Smith",
      "status": "active"
    }
  ]
}
```

## 3. Get Specific Conversation Participant

```bash
curl -X GET http://localhost:8080/conversation/participants/conv_abc123
```

Response:

```json
{
  "status": "ok",
  "message": "Conversation participant retrieved successfully",
  "result": {
    "id": "conv_abc123",
    "phoneNumber": "+1234567890",
    "name": "Alice Smith",
    "gender": "female",
    "ethnicity": "Hispanic",
    "background": "College student studying psychology, interested in mental health topics",
    "status": "active",
    "enrolledAt": "2025-07-03T14:30:00Z"
  }
}
```

## 4. Update Conversation Participant

```bash
curl -X PUT http://localhost:8080/conversation/participants/conv_abc123 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Alice Johnson",
    "status": "paused"
  }'
```

## 5. Delete Conversation Participant

```bash
curl -X DELETE http://localhost:8080/conversation/participants/conv_abc123
```

## Automatic Conversation Flow

After enrollment, the system automatically:

1. **Sends first message**: AI initiates contact with the enrolled user
2. **Processes responses**: All incoming messages from the user are handled by the conversation flow
3. **Maintains history**: Full conversation history is preserved and used for context
4. **Generates responses**: AI generates contextually appropriate responses based on user background and conversation history

## Example Conversation Flow

1. User enrolls via API ↓
2. AI sends: "Hi Alice! I'm an AI assistant here to chat with you. As a psychology student, you might find our conversations interesting. What's on your mind today?" ↓
3. User responds: "I'm stressed about my upcoming exams" ↓
4. AI responds: "I understand exam stress can be overwhelming, especially in psychology where there's so much to remember. What specific aspects are causing you the most concern?" ↓
5. Conversation continues with AI maintaining context...

The AI's responses are informed by:

- The system prompt configuration
- User's background information (psychology student, Hispanic, female)
- Complete conversation history
- Current message content
