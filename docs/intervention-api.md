# Micro Health Intervention API Documentation

This document describes the APIs for enrolling, managing, and tracking participants in the Micro Health Intervention flow.

## Overview

The Micro Health Intervention is a stateful messaging flow designed for health behavior research. It guides participants through a structured series of prompts and responses over time, tracking their engagement and progress.

## Flow Type

All intervention participants use the flow type: `micro_health_intervention`

## API Endpoints

### Participant Management

#### 1. Enroll Participant

`POST /intervention/participants`

Enrolls a new participant in the micro health intervention study.

**Request Body:**

```json
{
    "phone_number": "+1234567890",
    "name": "John Doe",
    "timezone": "America/New_York",
    "daily_prompt_time": "10:00"
}
```

**Required Fields:**

- `phone_number`: Valid phone number in international format

**Optional Fields:**

- `name`: Participant's name
- `timezone`: Timezone for scheduling (defaults to "UTC")
- `daily_prompt_time`: Time for daily prompts in HH:MM format (defaults to "10:00")

**Response:**

```json
{
    "status": "ok",
    "result": {
        "id": "p_1234567890abcdef",
        "phone_number": "+1234567890",
        "name": "John Doe",
        "timezone": "America/New_York",
        "status": "active",
        "enrolled_at": "2024-01-15T10:00:00Z",
        "daily_prompt_time": "10:00",
        "weekly_reset": "2024-01-22T10:00:00Z",
        "created_at": "2024-01-15T10:00:00Z",
        "updated_at": "2024-01-15T10:00:00Z"
    }
}
```

**Error Responses:**

- `400 Bad Request`: Invalid request data or phone number already enrolled
- `409 Conflict`: Participant with this phone number already exists

#### 2. List Participants

`GET /intervention/participants`

Retrieves all enrolled participants.

**Response:**

```json
{
    "status": "ok",
    "result": [
        {
            "id": "p_1234567890abcdef",
            "phone_number": "+1234567890",
            "status": "active",
            ...
        }
    ]
}
```

#### 3. Get Participant

`GET /intervention/participants/{id}`

Retrieves details for a specific participant.

**Response:**

```json
{
    "status": "ok",
    "result": {
        "id": "p_1234567890abcdef",
        "phone_number": "+1234567890",
        "name": "John Doe",
        "status": "active",
        ...
    }
}
```

**Error Responses:**

- `404 Not Found`: Participant does not exist

#### 4. Delete Participant

`DELETE /intervention/participants/{id}`

Removes a participant from the study. This will also delete all their responses and flow state.

**Response:**

```json
{
    "status": "ok",
    "result": null
}
```

**Error Responses:**

- `404 Not Found`: Participant does not exist

### Response Processing

#### 5. Process Response

`POST /intervention/participants/{id}/responses`

Records a participant's response and processes it according to the intervention flow logic.

**Request Body:**

```json
{
    "response_text": "1",
    "context": "WhatsApp message"
}
```

**Required Fields:**

- `response_text`: The participant's actual response

**Optional Fields:**

- `context`: Additional context about how the response was received

**Response:**

```json
{
    "status": "ok",
    "result": {
        "id": "r_abcdef1234567890",
        "participant_id": "p_1234567890abcdef",
        "state": "COMMITMENT_PROMPT",
        "response_text": "1",
        "response_type": "commitment",
        "timestamp": "2024-01-15T10:05:00Z"
    }
}
```

**Error Responses:**

- `400 Bad Request`: Missing response_text
- `404 Not Found`: Participant does not exist

### State Management

#### 6. Advance State

`POST /intervention/participants/{id}/advance`

Manually advances a participant to a specific state in the intervention flow.

**Request Body:**

```json
{
    "to_state": "FEELING_PROMPT",
    "reason": "Manual advancement for testing"
}
```

**Required Fields:**

- `to_state`: Target state (must be valid intervention state)

**Optional Fields:**

- `reason`: Reason for manual advancement

**Valid States:**

- `ORIENTATION`
- `COMMITMENT_PROMPT`
- `FEELING_PROMPT`
- `RANDOM_ASSIGNMENT`
- `HABIT_REMINDER`
- `FOLLOW_UP`
- `COMPLETE`

**Response:**

```json
{
    "status": "ok",
    "result": {
        "participant_id": "p_1234567890abcdef",
        "from_state": "COMMITMENT_PROMPT",
        "to_state": "FEELING_PROMPT",
        "reason": "Manual advancement for testing",
        "advanced_at": "2024-01-15T10:10:00Z"
    }
}
```

**Error Responses:**

- `400 Bad Request`: Invalid state or missing to_state
- `404 Not Found`: Participant does not exist

#### 7. Reset Participant

`POST /intervention/participants/{id}/reset`

Resets a participant's flow state back to the beginning (ORIENTATION).

**Response:**

```json
{
    "status": "ok",
    "result": {
        "participant_id": "p_1234567890abcdef",
        "reset_to": "ORIENTATION",
        "reset_at": "2024-01-15T10:15:00Z"
    }
}
```

**Error Responses:**

- `404 Not Found`: Participant does not exist

### History & Analytics

#### 8. Get Participant History

`GET /intervention/participants/{id}/history`

Retrieves a participant's complete history including their current state and all responses.

**Response:**

```json
{
    "status": "ok",
    "result": {
        "participant": {
            "id": "p_1234567890abcdef",
            "phone_number": "+1234567890",
            "status": "active",
            ...
        },
        "current_state": "FEELING_PROMPT",
        "responses": [
            {
                "id": "r_abcdef1234567890",
                "state": "COMMITMENT_PROMPT",
                "response_text": "1",
                "response_type": "commitment",
                "timestamp": "2024-01-15T10:05:00Z"
            }
        ],
        "response_count": 1
    }
}
```

**Error Responses:**

- `404 Not Found`: Participant does not exist

#### 9. Weekly Summary Trigger

`POST /intervention/weekly-summary`

Triggers weekly summary processing for all eligible participants.

**Response:**

```json
{
    "status": "ok",
    "result": {
        "participants_processed": 5,
        "triggered_at": "2024-01-15T10:00:00Z"
    }
}
```

#### 10. Intervention Statistics

`GET /intervention/stats`

Retrieves comprehensive statistics about the intervention study.

**Response:**

```json
{
    "status": "ok",
    "result": {
        "total_participants": 25,
        "participants_by_status": {
            "active": 20,
            "paused": 2,
            "completed": 2,
            "withdrawn": 1
        },
        "participants_by_state": {
            "ORIENTATION": 5,
            "COMMITMENT_PROMPT": 8,
            "FEELING_PROMPT": 4,
            "HABIT_REMINDER": 6,
            "COMPLETE": 2
        },
        "total_responses": 127,
        "responses_by_type": {
            "commitment": 45,
            "feeling": 32,
            "completion": 28,
            "followup": 22
        },
        "completion_rate": 8.0,
        "average_response_time_minutes": 0.0
    }
}
```

## Participant Status Values

- `active`: Participant is actively enrolled and receiving prompts
- `paused`: Participant is temporarily paused (not receiving prompts)
- `completed`: Participant has completed the intervention
- `withdrawn`: Participant has withdrawn from the study

## Response Types

Based on the current state when the response was given:

- `commitment`: Response to commitment prompt
- `feeling`: Response to feeling check
- `completion`: Response to habit completion check
- `followup`: Response to follow-up questions
- `general`: Other responses

## Flow States

The intervention follows these states in sequence:

1. **ORIENTATION**: Welcome message and initial setup
2. **COMMITMENT_PROMPT**: Daily commitment check
3. **FEELING_PROMPT**: Feeling assessment
4. **RANDOM_ASSIGNMENT**: Internal state for randomization
5. **HABIT_REMINDER**: Reminder and completion check
6. **FOLLOW_UP**: Follow-up questions based on response
7. **COMPLETE**: Intervention completed

## Error Handling

All endpoints return standard HTTP status codes:

- `200 OK`: Success
- `201 Created`: Resource created successfully
- `400 Bad Request`: Invalid request data
- `404 Not Found`: Resource not found
- `409 Conflict`: Resource already exists
- `500 Internal Server Error`: Server error

Error responses include a descriptive message:

```json
{
    "status": "error",
    "message": "Participant with this phone number already enrolled"
}
```

## Integration with Stateful Generation

These APIs are designed to work with the stateful generation system:

1. **Flow Type**: All participants use `micro_health_intervention` flow type
2. **State Management**: The flow state is managed through the `StateManager` interface
3. **Message Generation**: Messages are generated using the `MicroHealthInterventionGenerator`
4. **Timer Management**: Time-based prompts and timeouts are handled through the `Timer` interface

## Usage Examples

### Enrolling a Participant

```bash
curl -X POST http://localhost:8080/intervention/participants \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+1234567890",
    "name": "Alice Smith",
    "timezone": "America/New_York",
    "daily_prompt_time": "09:00"
  }'
```

### Processing a Response

```bash
curl -X POST http://localhost:8080/intervention/participants/p_1234567890abcdef/responses \
  -H "Content-Type: application/json" \
  -d '{
    "response_text": "1",
    "context": "WhatsApp"
  }'
```

### Getting Statistics

```bash
curl http://localhost:8080/intervention/stats
```

This API design provides comprehensive management capabilities for the Micro Health Intervention while maintaining separation from the regular send/schedule endpoints and following RESTful best practices.
