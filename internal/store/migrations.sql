-- SQL migration for PromptPipe receipts table
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
