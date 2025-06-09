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
