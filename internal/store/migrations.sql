-- SQL migration for PromptPipe receipts table
CREATE TABLE IF NOT EXISTS receipts (
    id SERIAL PRIMARY KEY,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL,
    time BIGINT NOT NULL
);
