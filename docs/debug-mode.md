# Debug Mode for ChatGPT API Calls

This feature enables debugging of ChatGPT API calls by logging all parameters and responses to JSON files. When debug mode is enabled, each API call creates a separate JSON log file in the state directory with detailed information about the request and response.

## Configuration

Debug mode can be enabled in two ways:

### Environment Variable

```bash
export PROMPTPIPE_DEBUG=true
```

Accepted values for `PROMPTPIPE_DEBUG`:

- `true`, `1`, `yes`, `on` (case insensitive) - Enable debug mode
- `false`, `0`, `no`, `off` (case insensitive) - Disable debug mode
- Any other value defaults to `false` with a warning

### Command Line Flag

```bash
./promptpipe --debug
```

The command line flag takes precedence over the environment variable.

## Debug Log Files

When debug mode is enabled, API call logs are stored in:

```text
{STATE_DIR}/debug/
```

Each log file is named using the format:

```text
YYYY-MM-DD.HH-MM-SS.{16-character-random-hex}.json
```

Example filename: `2025-07-22.14-30-45.a1b2c3d4e5f6789a.json`

## Log File Structure

Each debug log file contains a JSON object with the following fields:

```json
{
  "timestamp": "2025-07-22T14:30:45Z",
  "method": "GeneratePromptWithContext",
  "model": "gpt-4o-mini",
  "params": {
    "model": "gpt-4o-mini",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant"
      },
      {
        "role": "user", 
        "content": "Hello, how are you?"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 1000
  },
  "response": {
    "choices": [
      {
        "message": {
          "content": "I'm doing well, thank you for asking!"
        }
      }
    ]
  },
  "error": null
}
```

### Fields Description

- **timestamp**: UTC timestamp when the API call was made (RFC3339 format)
- **method**: The GenAI client method that was called
  - `GeneratePromptWithContext` - Simple system/user prompt generation
  - `GenerateWithMessages` - Multi-message conversation generation
  - `GenerateWithTools` - Generation with tool/function calling capability
  - `GenerateThinkingWithMessages` - Structured JSON output separating internal thinking and user-facing content
  - `GenerateThinkingWithTools` - Structured thinking + content while enabling tool/function calls
- **model**: The OpenAI model used for the generation
- **params**: Complete parameters sent to the OpenAI API
- **response**: Complete response received from the OpenAI API
- **error**: Error message if the API call failed, otherwise `null`

## Structured Thinking Capture

When debug mode is active, the system surfaces the model's internal reasoning ("thinking") for agent modules (Coordinator, Intake, Feedback, Prompt Generator) without altering user-facing replies. This is accomplished via new GenAI methods that instruct the model to emit JSON of the form:

```json
{"thinking": "brief reasoning", "content": "final user reply"}
```

Key properties:

- Always enabled (no toggle) to maintain consistent prompting schemas and avoid drift between debug and non-debug operation.
- The thinking field is only delivered to developers via separate debug messages (prefixed with 🐛) and never sent to end users directly.
- Tool-enabled generations embed the same JSON content while still returning native tool call objects.
- If JSON parsing fails, the raw content is treated as the user reply and thinking is left empty (non-fatal fallback).

Displayed debug example (WhatsApp/SMS):

```text
🐛 DEBUG: Coordinator thinking (round 2): Deciding to call save_user_profile because motivation missing; will gather motivation next.
```

## Use Cases

Debug mode is useful for:

1. **Troubleshooting**: Inspect exact parameters sent to OpenAI API
2. **Cost Analysis**: Track token usage and API calls
3. **Performance Analysis**: Understand response times and patterns
4. **Development**: Debug prompt engineering and model behavior
5. **Auditing**: Keep records of all AI interactions

## Security Considerations

⚠️ **Important**: Debug log files contain:

- Complete conversation history
- System prompts (which may contain sensitive instructions)
- User inputs (which may contain personal information)
- AI responses

**Recommendations:**

- Only enable debug mode in development/testing environments
- Regularly clean up debug log files
- Ensure proper file permissions on the state directory
- Consider data retention policies for debug logs

## Example Usage

### Enable debug mode and run PromptPipe

```bash
# Method 1: Environment variable
export PROMPTPIPE_DEBUG=true
./promptpipe

# Method 2: Command line flag
./promptpipe --debug

# Method 3: Both (flag takes precedence)
export PROMPTPIPE_DEBUG=false
./promptpipe --debug  # Debug mode will be enabled
```

### Test with a GenAI prompt

```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "+1234567890",
    "type": "genai",
    "system_prompt": "You are a helpful assistant",
    "user_prompt": "Hello, how are you?"
  }'
```

After the request, check for debug files:

```bash
ls -la /var/lib/promptpipe/debug/
```

### View debug log

```bash
cat /var/lib/promptpipe/debug/2025-07-22.14-30-45.a1b2c3d4e5f6789a.json | jq .
```

## Disabling Debug Mode

To disable debug mode:

```bash
# Method 1: Unset environment variable
unset PROMPTPIPE_DEBUG

# Method 2: Set to false
export PROMPTPIPE_DEBUG=false

# Method 3: Remove --debug flag
./promptpipe  # without --debug flag
```

## Implementation Details

- Debug logging only occurs when both debug mode is enabled AND a state directory is configured
- Random hex strings prevent filename collisions when multiple API calls happen simultaneously
- Debug directory is created automatically with permissions 0755
- Failed debug logging operations are logged as warnings but don't affect API functionality
- Debug logging has minimal performance impact on API calls
- Structured thinking is appended as an extra system instruction; maintaining it permanently avoids prompt divergence, so there is intentionally no runtime toggle to disable it.
