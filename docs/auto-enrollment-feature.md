# Auto-Enrollment Feature for New Users

## Overview

This feature enables automatic enrollment of new users into the conversation flow when they send their first message to the system. When enabled, users who are not yet enrolled will be automatically registered with an empty profile, and their first message will be handled by the conversation flow (intake/feedback modules).

## Configuration

### Environment Variable

```bash
AUTO_ENROLL_NEW_USERS=true
```

- **Default**: `false` (disabled by default)
- **Type**: Boolean
- **Description**: When set to `true`, any user who sends a message without being enrolled will be automatically enrolled in the conversation flow.

### Command Line Flag

```bash
./build/promptpipe --auto-enroll-new-users=true
```

- **Flag**: `--auto-enroll-new-users`
- **Default**: `false`
- **Description**: Overrides the environment variable setting.

## How It Works

### Flow Diagram

```
User sends message
       ↓
Response Handler receives message
       ↓
Auto-enrollment enabled? → No → Process normally
       ↓ Yes
       ↓
User already enrolled? → Yes → Process normally
       ↓ No
       ↓
Create empty participant profile
       ↓
Set conversation state to ACTIVE
       ↓
Register conversation hook
       ↓
Process message with conversation flow
```

### Implementation Details

1. **Message Reception**: When a user sends a message, the `ResponseHandler.ProcessResponse` method is called.
   - Auto-enrollment is only triggered via the messaging service response handler (not via the `POST /response` API).

2. **Auto-Enrollment Check**: Before processing the message, if `autoEnrollNewUsers` is enabled, the system checks if the user is already enrolled.

3. **Profile Creation**: If the user is not enrolled, the system creates a new `ConversationParticipant` with:
   - Auto-generated participant ID (format: `p_<random_hex>`)
   - Phone number (canonicalized)
   - Empty profile fields (Name, Gender, Ethnicity, Background)
   - Status: `ACTIVE`
   - Current timestamp for EnrolledAt, CreatedAt, UpdatedAt

4. **State Initialization**: The conversation flow state is initialized to `CONVERSATION_ACTIVE`.

5. **Hook Registration**: A persistent conversation hook is registered for the participant, enabling the conversation flow to handle all subsequent messages.

6. **First Message**: The user's first message is processed by the conversation flow. The flow determines the reply based on the intake/feedback state; no extra welcome message is sent outside that flow.

## Code Changes

### Files Modified

1. **`cmd/PromptPipe/main.go`**
   - Added `AutoEnrollNewUsers` field to `Config` struct
   - Added `autoEnrollNewUsers` field to `Flags` struct
   - Added environment variable parsing in `loadEnvironmentConfig`
   - Added command line flag parsing in `parseCommandLineFlags`
   - Added option passing in `buildAPIOptions`

2. **`internal/api/api.go`**
   - Added `AutoEnrollNewUsers` field to `Opts` struct
   - Added `autoEnrollNewUsers` field to `Server` struct
   - Created `WithAutoEnrollNewUsers` option function
   - Updated `createAndConfigureServer` to pass flag to ResponseHandler

3. **`internal/messaging/response_handler.go`**
   - Added `autoEnrollNewUsers` field to `ResponseHandler` struct
   - Updated `NewResponseHandler` signature to accept `autoEnrollNewUsers` parameter
   - Implemented `autoEnrollIfNeeded` method for auto-enrollment logic
   - Updated `ProcessResponse` to call auto-enrollment before processing messages

4. **Test Files**
   - Updated all test files to pass `false` for `autoEnrollNewUsers` parameter to maintain existing test behavior

## Usage Examples

### Enable Auto-Enrollment

**Using Environment Variable:**

```bash
export AUTO_ENROLL_NEW_USERS=true
./build/promptpipe
```

**Using Command Line Flag:**

```bash
./build/promptpipe --auto-enroll-new-users=true
```

**Using .env File:**

```env
AUTO_ENROLL_NEW_USERS=true
```

### Disable Auto-Enrollment (Default)

```bash
# Don't set the environment variable or explicitly set to false
export AUTO_ENROLL_NEW_USERS=false
./build/promptpipe
```

## User Experience

### With Auto-Enrollment Enabled

1. **User** sends first message: "Hello"
2. **System** automatically:
   - Creates participant profile with empty fields
   - Enrolls user in conversation flow
   - Initializes conversation state
3. **Conversation Flow** responds based on intake/feedback logic
4. **User** receives the flow response and conversation begins

### With Auto-Enrollment Disabled (Default)

1. **User** sends first message: "Hello"
2. **System** responds with default message: "📝 Your message has been recorded. Thank you for your response!"
3. **User** must be manually enrolled via API to join conversation flow

## Security Considerations

- Auto-enrollment is **disabled by default** to prevent unauthorized access
- Phone numbers are validated and canonicalized before enrollment
- All enrollment operations are logged for audit purposes
- Empty profiles are created to minimize data collection until user provides information

## Monitoring and Logging

The system logs the following events related to auto-enrollment:

- `ResponseHandler auto-enrolled new participant` - Successful auto-enrollment
- `ResponseHandler autoEnrollIfNeeded: participant already enrolled` - Skip for existing users
- `ResponseHandler autoEnrollIfNeeded: failed to check existing participant` - Error checking enrollment
- `ResponseHandler autoEnrollIfNeeded: save failed` - Error saving participant
- `ResponseHandler auto-enrollment complete, conversation flow will handle first message` - Ready to process

## Backward Compatibility

This feature is fully backward compatible:

- Default behavior is unchanged (auto-enrollment disabled)
- Existing tests continue to pass with `autoEnrollNewUsers=false`
- Manual enrollment via API continues to work as before
- No database schema changes required

## Future Enhancements

Potential improvements for this feature:

1. **Configurable Welcome Message**: Allow customization of auto-enrollment welcome message
2. **Rate Limiting**: Prevent abuse by limiting auto-enrollments per time period
3. **Whitelist/Blacklist**: Support phone number filtering for auto-enrollment
4. **Pre-populated Profiles**: Option to pre-fill profile data from external sources
5. **Analytics**: Track auto-enrollment rates and user engagement metrics
