# PromptPipe API Reference

All endpoints accept and return JSON. The service does not implement authentication or rate limiting.

Phone numbers are **canonicalized** by stripping non-numeric characters. Stored values (and API responses) use the canonical form (digits only). Inputs with fewer than 6 digits are rejected.

## Response envelopes

Most endpoints return a standard envelope:

```json
{ "status": "ok", "result": { "...": "..." } }
```

Errors use:

```json
{ "status": "error", "message": "Human-readable error" }
```

**Exceptions** (no envelope):

- `GET /health`
- `GET /timers`
- `GET /timers/{id}`
- `DELETE /timers/{id}`

## Data models

### Prompt

```json
{
  "to": "15551234567",
  "schedule": {
    "minute": 0,
    "hour": 9,
    "weekday": 1,
    "timezone": "America/Toronto"
  },
  "type": "static | genai | branch | conversation | custom",
  "state": "string (custom flows only)",
  "body": "string (static/custom)",
  "system_prompt": "string (genai)",
  "user_prompt": "string (genai/conversation)",
  "branch_options": [{ "label": "string", "body": "string" }]
}
```

Validation rules:

- `to` is required and must contain at least 6 digits after canonicalization.
- `type` defaults to `"static"` when omitted.
- `static` prompts require `body` (max 4096 chars).
- `genai` prompts require both `system_prompt` and `user_prompt`.
- `conversation` prompts require `user_prompt` (system prompt is loaded from files).
- `branch` prompts require `branch_options` with 2–10 options. Each label ≤ 100 chars, each body ≤ 1000 chars.
- `custom` prompts require a custom generator to be registered in code.

### Schedule

```json
{
  "minute": 0,
  "hour": 9,
  "day": 1,
  "month": 1,
  "weekday": 1,
  "timezone": "America/Toronto"
}
```

Each field is optional; omitted fields mean “any”. `timezone` must be a valid IANA time zone name (e.g., `America/Toronto`). If no timezone is set, scheduling uses **UTC**.

The simple timer calculates the **next** run starting from the next day (it does not backfill “today” for recurring schedules). Invalid schedule values yield a server error from `/schedule`.

### BranchOption

```json
{ "label": "Option label", "body": "Message sent when selected" }
```

### Receipt

```json
{
  "to": "15551234567",
  "status": "sent | delivered | read | failed | cancelled",
  "time": 1739100000
}
```

### Response

```json
{
  "from": "15551234567",
  "body": "User reply",
  "time": 1739100000
}
```

`time` is set by the server when calling `POST /response`.

### TimerInfo

```json
{
  "id": "sched_abcd1234...",
  "type": "once | recurring",
  "scheduled_at": "2026-02-09T04:00:00Z",
  "expires_at": "2026-02-09T04:10:00Z",
  "schedule": { "hour": 9, "minute": 0 },
  "next_run": "2026-02-10T09:00:00Z",
  "remaining": "5m0s",
  "description": "..."
}
```

`expires_at` is only set for `once` timers; `schedule` and `next_run` are set for `recurring` timers.

### ConversationParticipant

```json
{
  "id": "p_...",
  "phone_number": "15551234567",
  "name": "Optional name",
  "gender": "Optional",
  "ethnicity": "Optional",
  "background": "Optional",
  "timezone": "",
  "status": "active | paused | completed | withdrawn",
  "enrolled_at": "2026-02-09T04:00:00Z",
  "created_at": "2026-02-09T04:00:00Z",
  "updated_at": "2026-02-09T04:00:00Z"
}
```

The API validates `timezone`, but the current store implementation does **not** persist it.

## Endpoints

### POST /send

Send a prompt immediately.

**Request:** `Prompt` (schedule is ignored).

`type=genai` requires `OPENAI_API_KEY`; otherwise generation will fail.

**Response:** `200 OK`

```json
{ "status": "ok", "message": "Message sent successfully" }
```

Errors:

- `400` invalid JSON, recipient, or prompt validation.
- `500` failure generating content or sending message.

### POST /schedule

Schedule a prompt using a `schedule` object. If no schedule is provided, `DEFAULT_SCHEDULE` is used when configured.

**Request:** `Prompt`

**Response:** `201 Created`

```json
{
  "status": "ok",
  "message": "Scheduled successfully",
  "result": "sched_abcd1234..."
}
```

Errors:

- `400` invalid JSON, invalid recipient, invalid prompt, missing schedule without default, or GenAI client missing for `type=genai`.
- `500` failure creating the schedule (including invalid schedule values).

### GET /receipts

**Response:** `200 OK`

```json
{ "status": "ok", "result": [ { "to": "...", "status": "sent", "time": 1739100000 } ] }
```

### POST /response

Store an incoming response record. The server sets `time` to `now`.

**Request:** `Response`

**Response:** `201 Created`

```json
{ "status": "ok", "message": "Response recorded successfully" }
```

### GET /responses

**Response:** `200 OK`

```json
{ "status": "ok", "result": [ { "from": "...", "body": "...", "time": 1739100000 } ] }
```

### GET /stats

**Response:** `200 OK`

```json
{
  "status": "ok",
  "result": {
    "total_responses": 12,
    "responses_per_sender": { "15551234567": 5 },
    "avg_response_length": 32.5
  }
}
```

### GET /health

**Response:** `200 OK`

```json
{ "status": "healthy", "timestamp": "2026-02-09T04:00:00Z", "active_participants": 5 }
```

If the participant count fails, the API returns `503` with `status: "degraded"` and an `error` field.

### Timers

#### GET /timers

```json
{ "timers": [ { "id": "...", "type": "recurring", "remaining": "..." } ], "count": 1 }
```

#### GET /timers/{id}

Returns a `TimerInfo` object.

#### DELETE /timers/{id}

```json
{ "message": "Timer cancelled successfully", "timerID": "...", "canceled": true }
```

### Conversation participants

#### POST /conversation/participants

Enroll a participant and send the first conversation message.

```json
{
  "phone_number": "15551234567",
  "name": "Optional",
  "gender": "Optional",
  "ethnicity": "Optional",
  "background": "Optional",
  "timezone": "America/Toronto"
}
```

**Response:** `201 Created`

```json
{ "status": "ok", "message": "Conversation participant enrolled successfully", "result": { ... } }
```

#### GET /conversation/participants

Lists participants:

```json
{ "status": "ok", "result": [ { ... } ] }
```

#### GET /conversation/participants/{id}

```json
{ "status": "ok", "result": { ... } }
```

#### PUT /conversation/participants/{id}

Partial update payload (all optional):

```json
{
  "name": "New name",
  "gender": "Optional",
  "ethnicity": "Optional",
  "background": "Optional",
  "timezone": "America/Toronto",
  "status": "paused"
}
```

`timezone` is validated but not persisted by the current store implementation.

#### DELETE /conversation/participants/{id}

Returns:

```json
{ "status": "ok", "message": "Participant deleted successfully" }
```
