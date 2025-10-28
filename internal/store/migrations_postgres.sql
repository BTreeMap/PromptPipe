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
    timezone TEXT,
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

-- SQL migration for active timers (for persistence and recovery)
CREATE TABLE IF NOT EXISTS active_timers (
    id TEXT PRIMARY KEY,
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    timer_type TEXT NOT NULL, -- 'once', 'recurring'
    state_type TEXT,
    data_key TEXT,
    callback_type TEXT NOT NULL, -- 'scheduled_prompt', 'feedback_initial', 'feedback_followup', 'state_transition', 'reminder', 'auto_feedback'
    callback_params JSONB, -- JSON with callback-specific parameters
    scheduled_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP, -- For one-time timers
    original_delay_seconds INTEGER, -- Original delay for one-time timers
    schedule_json JSONB, -- JSON for recurring timers (Schedule object)
    next_run TIMESTAMP, -- Next execution time for recurring timers
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for timer lookups and cleanup
CREATE INDEX IF NOT EXISTS idx_active_timers_participant ON active_timers(participant_id);
CREATE INDEX IF NOT EXISTS idx_active_timers_flow_type ON active_timers(flow_type);
CREATE INDEX IF NOT EXISTS idx_active_timers_callback_type ON active_timers(callback_type);
CREATE INDEX IF NOT EXISTS idx_active_timers_expires_at ON active_timers(expires_at);
CREATE INDEX IF NOT EXISTS idx_active_timers_next_run ON active_timers(next_run);

-- Schema evolution: Add timezone column to existing conversation_participants table
-- Note: PostgreSQL supports "IF NOT EXISTS" starting from version 9.6
-- For older versions, this will fail if column exists (handled in Go migration code)
ALTER TABLE conversation_participants ADD COLUMN IF NOT EXISTS timezone TEXT;
