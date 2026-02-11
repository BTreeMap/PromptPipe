-- SQL migration for PromptPipe PostgreSQL tables
CREATE TABLE IF NOT EXISTS receipts (
    id SERIAL PRIMARY KEY,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL,
    time BIGINT NOT NULL
);

-- SQL migration for incoming responses
CREATE TABLE IF NOT EXISTS responses (
    id SERIAL PRIMARY KEY,
    sender TEXT NOT NULL,
    body TEXT NOT NULL,
    time BIGINT NOT NULL
);

-- SQL migration for flow states
CREATE TABLE IF NOT EXISTS flow_states (
    id SERIAL PRIMARY KEY,
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    current_state TEXT NOT NULL,
    state_data JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(participant_id, flow_type)
);

-- SQL migration for intervention participants
CREATE TABLE IF NOT EXISTS intervention_participants (
    id TEXT PRIMARY KEY,
    phone_number TEXT NOT NULL UNIQUE,
    name TEXT,
    timezone TEXT,
    status TEXT NOT NULL,
    enrolled_at TIMESTAMP NOT NULL,
    daily_prompt_time TEXT NOT NULL,
    weekly_reset TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Index for phone number lookups
CREATE INDEX IF NOT EXISTS idx_intervention_participants_phone ON intervention_participants(phone_number);
CREATE INDEX IF NOT EXISTS idx_intervention_participants_status ON intervention_participants(status);

-- SQL migration for intervention responses
CREATE TABLE IF NOT EXISTS intervention_responses (
    id TEXT PRIMARY KEY,
    participant_id TEXT NOT NULL,
    state TEXT NOT NULL,
    response_text TEXT NOT NULL,
    response_type TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    FOREIGN KEY (participant_id) REFERENCES intervention_participants(id) ON DELETE CASCADE
);

-- Index for participant response lookups
CREATE INDEX IF NOT EXISTS idx_intervention_responses_participant ON intervention_responses(participant_id);
CREATE INDEX IF NOT EXISTS idx_intervention_responses_timestamp ON intervention_responses(timestamp);

-- SQL migration for conversation participants
CREATE TABLE IF NOT EXISTS conversation_participants (
    id TEXT PRIMARY KEY,
    phone_number TEXT NOT NULL UNIQUE,
    name TEXT,
    gender TEXT,
    ethnicity TEXT,
    background TEXT,
    status TEXT NOT NULL,
    enrolled_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Index for conversation participant lookups
CREATE INDEX IF NOT EXISTS idx_conversation_participants_phone ON conversation_participants(phone_number);
CREATE INDEX IF NOT EXISTS idx_conversation_participants_status ON conversation_participants(status);

-- SQL migration for registered hooks
CREATE TABLE IF NOT EXISTS registered_hooks (
    phone_number TEXT PRIMARY KEY,
    hook_type TEXT NOT NULL,
    parameters JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Index for hook type lookups
CREATE INDEX IF NOT EXISTS idx_registered_hooks_type ON registered_hooks(hook_type);

-- Durable jobs table (replaces in-memory timers)
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    run_at TIMESTAMP NOT NULL,
    payload_json JSONB,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    last_error TEXT,
    locked_at TIMESTAMP,
    dedupe_key TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_run_at ON jobs(status, run_at);
CREATE INDEX IF NOT EXISTS idx_jobs_kind ON jobs(kind);
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_dedupe_key ON jobs(dedupe_key) WHERE dedupe_key IS NOT NULL AND status NOT IN ('done', 'canceled');

-- Outbox messages table (restart-safe outgoing sends)
CREATE TABLE IF NOT EXISTS outbox_messages (
    id TEXT PRIMARY KEY,
    participant_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload_json JSONB,
    status TEXT NOT NULL DEFAULT 'queued',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    dedupe_key TEXT,
    locked_at TIMESTAMP,
    last_error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_status_next ON outbox_messages(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_outbox_participant ON outbox_messages(participant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_dedupe_key ON outbox_messages(dedupe_key) WHERE dedupe_key IS NOT NULL AND status NOT IN ('sent', 'canceled');

-- Inbound message dedup table
CREATE TABLE IF NOT EXISTS inbound_dedup (
    message_id TEXT PRIMARY KEY,
    participant_id TEXT NOT NULL,
    received_at TIMESTAMP NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_inbound_dedup_participant ON inbound_dedup(participant_id);
