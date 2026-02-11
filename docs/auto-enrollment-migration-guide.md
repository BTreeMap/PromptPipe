# Migration Guide: Auto-Enrollment Feature

## For Existing Deployments

This guide helps you understand how the auto-enrollment feature affects existing PromptPipe deployments and how to migrate if desired.

## Default Behavior

**The auto-enrollment feature is DISABLED by default.** This means:

- Existing deployments will continue to work exactly as before
- No configuration changes are required
- Users must still be manually enrolled via the API

## Impact Assessment

### No Impact Scenarios

The auto-enrollment feature has **NO IMPACT** if:

- You don't enable the `AUTO_ENROLL_NEW_USERS` flag
- You continue using manual enrollment via API
- You have custom enrollment workflows

### Potential Impact Scenarios

The auto-enrollment feature may affect your deployment if:

- You want to reduce manual enrollment overhead
- You want to allow any user to start a conversation immediately
- You have high-volume user onboarding needs

## Enabling Auto-Enrollment

### Step 1: Review Security Implications

Before enabling auto-enrollment, consider:

1. **Access Control**: Any phone number can now initiate a conversation
2. **Resource Usage**: More participants = more database and LLM usage
3. **Data Privacy**: Empty profiles are created automatically
4. **Rate Limiting**: Consider implementing rate limits (future enhancement)

### Step 2: Update Configuration

Choose one of the following methods:

**Method A: Environment Variable (Recommended)**

```bash
# Add to your environment or .env file
AUTO_ENROLL_NEW_USERS=true
```

**Method B: Command Line Flag**

```bash
# Add to your startup command
./build/promptpipe --auto-enroll-new-users=true
```

**Method C: Systemd Service (for production)**

```ini
# /etc/systemd/system/promptpipe.service
[Service]
Environment="AUTO_ENROLL_NEW_USERS=true"
ExecStart=/usr/local/bin/promptpipe
```

### Step 3: Test in Staging

Before deploying to production:

1. Enable auto-enrollment in a staging environment
2. Send a test message from an unenrolled phone number
3. Verify the participant is created with empty profile
4. Verify the conversation flow responds via the intake/feedback modules
5. Check logs for auto-enrollment events

### Step 4: Monitor After Deployment

After enabling in production, monitor:

1. **Enrollment Rate**: Check participant creation logs
2. **Database Growth**: Monitor participant table size
3. **LLM Usage**: Track conversation API calls
4. **Error Rates**: Watch for auto-enrollment failures

## Rollback Plan

If you need to disable auto-enrollment:

### Quick Rollback

```bash
# Option 1: Unset environment variable
unset AUTO_ENROLL_NEW_USERS

# Option 2: Set to false explicitly
export AUTO_ENROLL_NEW_USERS=false

# Option 3: Remove from .env file
# Just delete or comment out the line

# Restart PromptPipe
systemctl restart promptpipe  # or your restart method
```

### Cleanup (Optional)

If you want to remove auto-enrolled participants:

```bash
# Identify auto-enrolled participants
# They will have participant IDs starting with "p_"
# and empty Name, Gender, Ethnicity, Background fields

# Use the DELETE /conversation/participants/{id} API endpoint
# to remove unwanted participants
```

## Database Considerations

### Storage Impact

Auto-enrollment creates new participant records. Estimate impact:

```
Size per participant: ~500 bytes (empty profile)
1000 auto-enrollments = ~500 KB
10000 auto-enrollments = ~5 MB
```

Storage impact is minimal, but consider cleanup policies for inactive participants.

### Indexing

The existing phone number index handles auto-enrollment efficiently:

```sql
-- Already exists in database schema
CREATE INDEX idx_conversation_participants_phone ON conversation_participants(phone_number);
```

No additional indexes are required.

## Monitoring Queries

### Count Auto-Enrolled Participants

```sql
-- Count participants with empty profiles (likely auto-enrolled)
SELECT COUNT(*) FROM conversation_participants 
WHERE name = '' AND gender = '' AND ethnicity = '' AND background = '';
```

### Recent Auto-Enrollments

```sql
-- View recent auto-enrollments (empty profiles created recently)
SELECT id, phone_number, enrolled_at, status 
FROM conversation_participants 
WHERE name = '' AND gender = '' AND ethnicity = '' AND background = ''
ORDER BY enrolled_at DESC 
LIMIT 100;
```

### Auto-Enrollment Rate

```sql
-- Calculate daily auto-enrollment rate
SELECT 
    DATE(enrolled_at) as date,
    COUNT(*) as total_enrollments,
    SUM(CASE WHEN name = '' AND gender = '' AND ethnicity = '' AND background = '' THEN 1 ELSE 0 END) as auto_enrollments
FROM conversation_participants 
WHERE enrolled_at >= NOW() - INTERVAL '7 days'
GROUP BY DATE(enrolled_at)
ORDER BY date DESC;
```

## Best Practices

### 1. Gradual Rollout

Enable auto-enrollment gradually:

1. Start with a small user group
2. Monitor for 24-48 hours
3. Expand to larger groups
4. Full rollout after successful testing

### 2. User Communication

Inform users about the change:

- Update documentation
- Send email notifications
- Update onboarding materials
- Add FAQ entries

### 3. Logging and Alerting

Set up monitoring for:

- High auto-enrollment rates (possible spam)
- Auto-enrollment failures (system issues)
- Database growth (capacity planning)

### 4. Regular Cleanup

Consider implementing:

- Inactive participant removal (e.g., no messages in 90 days)
- Scheduled database maintenance
- Profile completion prompts

## Troubleshooting

### Auto-Enrollment Not Working

**Symptom**: New users still receive default message instead of conversation flow

**Solutions**:

1. Verify environment variable is set: `echo $AUTO_ENROLL_NEW_USERS`
2. Check logs for auto-enrollment attempts
3. Verify conversation flow is initialized
4. Ensure store (database) is accessible

### Too Many Auto-Enrollments

**Symptom**: Unexpected spike in participant count

**Solutions**:

1. Review phone number validation logs
2. Check for spam or bot activity
3. Consider implementing rate limiting (future enhancement)
4. Temporarily disable auto-enrollment

### Empty Profiles Not Being Filled

**Symptom**: Participants stay with empty profiles

**Expected Behavior**: This is normal. Auto-enrollment creates empty profiles by design. Users can fill their profiles through the conversation flow or API updates.

## Support

For issues or questions about auto-enrollment:

1. Check logs for error messages
2. Review the auto-enrollment feature documentation
3. Check GitHub issues for known problems
4. Create a new issue with detailed logs and steps to reproduce

## Future Enhancements

Planned improvements to auto-enrollment:

1. **Configurable Rate Limiting**: Prevent spam/abuse
2. **Whitelist/Blacklist**: Control which numbers can auto-enroll
3. **Custom Welcome Messages**: Per-deployment welcome messages
4. **Auto-Profile Enrichment**: Pre-fill profiles from external data
5. **Analytics Dashboard**: Track auto-enrollment metrics

Stay tuned for updates!
