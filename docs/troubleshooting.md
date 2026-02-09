# Troubleshooting

## “State directory already in use”

PromptPipe enforces a lock at `{STATE_DIR}/promptpipe.lock`. If another instance is running, startup will fail with a lock error.

Steps:

1. Ensure no other PromptPipe process is running.
2. If the lock is stale, remove it:

```bash
rm /var/lib/promptpipe/promptpipe.lock
```

## WhatsApp login / QR codes

Use the following flags during login:

- `--qr-output /path/to/qr.txt` to write a QR code to a file.
- `--numeric-code` to output a numeric login code instead of QR.

## GenAI not configured

GenAI prompts require `OPENAI_API_KEY`. If missing:

- `POST /schedule` with `type=genai` returns `400` (“GenAI client not configured”).
- `POST /send` with `type=genai` will fail to generate content.

## SQLite foreign keys warning

The WhatsApp client warns if the WhatsApp SQLite DSN does not enable foreign keys. Prefer:

```
file:/path/to/whatsmeow.db?_foreign_keys=on
```

## Invalid timezone

`schedule.timezone` and `conversation` enrollment/update `timezone` values must be valid IANA names (e.g., `America/Toronto`). Invalid timezones cause `400` errors.
