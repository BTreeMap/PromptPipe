Guide for using Twilio and testing functionality
================

1.  Go to [Twilio’s free trial signup
    page](https://www.twilio.com/docs/usage/tutorials/how-to-use-your-free-trial-account)
    and create a free trial account.
    ([twilio.com](https://www.twilio.com/docs/usage/tutorials/how-to-use-your-free-trial-account))

2.  After signup, in the Twilio Console navigate to: **Messaging → Try
    it out → “Send a WhatsApp message”** to activate the WhatsApp
    Sandbox.
    ([twilio.com](https://www.twilio.com/docs/whatsapp/sandbox))

    - Note the Sandbox phone number (e.g., `+1 234 567 8901`) and the
      unique join *keyword/code*.  

    - Use the code by sending a WhatsApp message from your phone to the
      sandbox number with:

          join <your-keyword>

    - Once you send that message, your WhatsApp number is linked to the
      sandbox.
      ([outrightcrm.com](https://www.outrightcrm.com/blog/twilio-whatsapp-api-guide/))

3.  Set the inbound webhook for sandbox messages.

    - In the Console under the sandbox settings, locate **“When a
      message comes in”** and paste your public URL endpoint (e.g., from
      ngrok) such as:

          https://<your-ngrok-id>.ngrok.io/twilio/webhook

    - Save it. Now Twilio will POST inbound WhatsApp messages to your
      `…/twilio/webhook` route.

4.  Update your `.env` file with Twilio credentials and set
    USE_TWILIO=true to run Twilio mode:

    ``` env
    USE_TWILIO=true
    TWILIO_ACCOUNT_SID=ACxxxxxxxxxxxxxxxxxxxx
    TWILIO_AUTH_TOKEN=your_auth_token
    TWILIO_FROM_NUMBER=whatsapp:+12345678901   # the sandbox “From” number
    ```

5.  Run PromptPipe in the fashion described in the here
    ([https://github.com/BTreeMap/PromptPipe](https://www.twilio.com/docs/whatsapp/sandbox))

6.  To test outbound messaging to your phone do the following. In a
    separate terminal (recall your phone number must have been
    registered in the Twilio sandbox)

``` bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{"to":"whatsapp:+<YOUR PHONE NUMBER>","type":"static","body":"Hello World"}'
```

If working you should in this terminal

``` bash
{"status":"ok","message":"Message sent successfully"}     
```

and messages indicating the message has been processed successfully in
the PromptPipe 7.To test outbound messaging do the following. Run your
local server and expose it via ngrok (this guide assumes ngrok, but any
equivalent tool works).:  
`bash    ngrok http 8080` Copy the HTTPS URL from ngrok (or whichever
you are using) and make sure it matches the webhook URL you set in step
3.  
On your phone or other Whatsapp client (the one you joined in step 2),
send a WhatsApp message to the sandbox number. There should be a
response from PromptPipe

TROUBLESHOOTING: This code assumes you are using the go.mod file that
comes with this branch. If not you will need to ensure that you have the
Twilio SDK added to go.mod. To do this run the following in PromptPipe
root directory
`bash    github.com/twilio/twilio-go@latest    go mod tidy` —

### Notes on the Twilio update

I’ve added tests for the new components (along with test):

`internal/twiliowhatsapp/` new package for Twilio WhatsApp client
similar to `internal/whatsapp/`

`internal/messaging/twilio_service.go` implements a Twilio services
similar to `internal/messaging/whatsapp_service.go`

The conditional route registration in api.go Modified to
cmd/promptpipe/main.go — logic to use Twilio if USE_TWILIO=true.

My additions will produce errors when running the old tests in main
while in Twilio mode. This is unavoidable without modifying the tests
since the Twilio client doesn’t setup all the functionality needed for
the existing tests. -In my testing, I found that outbound messages
always come with a poll. I believe this is working as intended given my
understanding of how the codebase works. If it is not let me know I can
attempt to fix it. —

If you like, I can **format this as a one-page PDF or markdown
checklist** so your partner can just follow it step-by-step and tick
things off. Would you like that?
