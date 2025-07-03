-- SQL migration for PromptPipe SQLite tables
CREATE TABLE IF NOT EXISTS receipts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL,
    time INTEGER NOT NULL
);

-- SQL migration for incoming responses
CREATE TABLE IF NOT EXISTS responses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sender TEXT NOT NULL,
    body TEXT NOT NULL,
    time INTEGER NOT NULL
);

-- SQL migration for flow states
CREATE TABLE IF NOT EXISTS flow_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    participant_id TEXT NOT NULL,
    flow_type TEXT NOT NULL,
    current_state TEXT NOT NULL,
    state_data TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(participant_id, flow_type)
);

-- SQL migration for intervention participants
CREATE TABLE IF NOT EXISTS intervention_participants (
    id TEXT PRIMARY KEY,
    phone_number TEXT NOT NULL UNIQUE,
    name TEXT,
    timezone TEXT,
    status TEXT NOT NULL,
    enrolled_at DATETIME NOT NULL,
    daily_prompt_time TEXT NOT NULL,
    weekly_reset DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
    timestamp DATETIME NOT NULL,
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
    enrolled_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for conversation participant lookups
CREATE INDEX IF NOT EXISTS idx_conversation_participants_phone ON conversation_participants(phone_number);
CREATE INDEX IF NOT EXISTS idx_conversation_participants_status ON conversation_participants(status);
