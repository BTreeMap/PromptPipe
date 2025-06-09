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
