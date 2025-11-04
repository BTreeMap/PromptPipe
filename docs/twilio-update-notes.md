Notes on the Twilio update
================

I’ve added the following new components (along with tests):

`internal/twiliowhatsapp/` new package for Twilio WhatsApp client
similar to `internal/whatsapp/`

`internal/messaging/twilio_service.go` implements a Twilio services
similar to `internal/messaging/whatsapp_service.go`

I added A conditional route registration in api.go to work with Twilio node

I modified cmd/promptpipe/main.go — logic to use Twilio if USE_TWILIO=true.

My additions will produce errors when running the old tests in main
while in Twilio mode. This is unavoidable without modifying the tests
since the Twilio client doesn’t setup all the functionality needed for
the existing tests. -In my testing, I found that outbound messages
always come with a poll. I believe this is working as intended given my
understanding of how the codebase works. If it is not let me know I can
attempt to fix it. —
