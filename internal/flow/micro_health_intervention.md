# Micro Health Intervention Flow

This document describes a structured flow for a micro health intervention study, where participants receive daily prompts to engage in small, manageable health habits. The flow is designed to adapt based on user responses and includes both immediate action and reflective components.

## High‐Level Overview

1. Each **participant** has a persistent “conversation state” that moves through predefined states on a daily schedule (plus “on‐demand” overrides).
2. At the top level, each day’s interaction consists of:

   1. Orientation (sent once at enrollment).
   2. Commitment prompt.
   3. Feeling‐check prompt.
   4. Random assignment → two possible subflows (Immediate vs. Reflective).
   5. Intervention prompt (with either an action‐oriented or reflection‐oriented script).
   6. Completion‐check prompt → branch into:

      * Success path (reinforce + end).
      * Failure/no‐reply path → further triage (“Did you get a chance?”).

        * If “Yes, I did try” → context → mood → barrier check → end.
        * If “No” → barrier‐reason check → end.
        * If no reply at all → “Ignored” path → minimal encouragement → end.
   7. Regardless of which mini‐branch a participant took, after seven days send a weekly summary.

Behind the scenes, you will maintain for each participant:

* A set of **persistent flags** (e.g. `hasSeenOrientation`, `lastCommitmentDate`, `flowAssignmentToday`, `timesCompletedThisWeek`).
* A handful of **timers** (feeling time‐out, completion time‐out, “did‐you‐get‐a‐chance” time‐out, etc.).
* A **randomization function** that flips a coin (e.g. `Math.random() < 0.5`) to choose Immediate vs. Reflective.
* A **scheduler** that (a) sends the daily prompts at a fixed time, (b) checks if a participant ever types “Ready,” and (c) triggers the weekly summary seven days after enrollment (or in rolling 7‐day windows).

Below is a breakdown of every state/transition. We name each “state” and then describe:

* **When it is entered** (what event causes you to transition into this state).
* **What the system sends** (the exact message content, with both Poll and SMS alternatives for multiple‐choice).
* **What inputs you are waiting for** (timeouts, specific user replies).
* **How to interpret those inputs** (which next state to jump to).

---

### 1. Enrollment & Orientation State

**State Name:** `ORIENTATION`
**Entry Condition:**

* Participant is newly enrolled (no prior history).

**Action When Entered:**

1. Send a single “Welcome” message once. For example:

   > “Hi \$Name\$, 🌱 Welcome to our Healthy Habits study!
   > Here’s how it works: You will receive messages based on a scheduled time, but you can request a message anytime you find it convenient. Simply write ‘Ready,’ and we’ll send the prompt right away to fit your schedule. Try them out and let us know your thoughts. Your input is very important.”

2. Immediately set a persistent flag (e.g. `hasSeenOrientation = true`) so we never send this again.

3. Do **not** start any timers in `ORIENTATION`; after sending this welcome, transition right away to the next state for Day 1’s commitment prompt at the moment specified by the daily scheduler (see step 2 below).

**Exit Condition:**

* None required; once sent, the orientation state is “complete.” The next daily scheduler run will detect `hasSeenOrientation = true` and move directly to `COMMITMENT_PROMPT`.

---

### 2. Commitment Prompt State

**State Name:** `COMMITMENT_PROMPT`
**Entry Condition (Normal):**

* It is the participant’s scheduled “daily prompt time” (e.g. 10 AM local).
* OR the user has typed **“Ready”** at any time after 24 hours since their last prompt (on‐demand override).

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll** with exactly these options:

   * **Title**: “You committed to trying a quick habit today—ready to go?”
   * **Option 1** (label): “🚀 Let’s do it!”
   * **Option 2** (label): “⏳ Not yet”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   You committed to trying a quick habit today—ready to go?
   1. 🚀 Let’s do it!
   2. ⏳ Not yet
   (Reply with “1” or “2”)
   ```

3. Persistently record:

   * `lastCommitmentDate = <today’s date>`
   * Clear out any previous state related to Day N’s flow so we can start fresh (e.g. `hasRespondedToCommitment = false`).

4. Start waiting for a user response or a secondary trigger:

   * **Wait for user input “1” or “2.”**
   * If the user does not reply within, say, `COMMITMENT_TIMEOUT = 12 hours` (or until 11:59 PM local time), automatically treat as if they replied “2 = Not yet.” In that case, we send **no further messages** until tomorrow’s scheduler.

**Possible User Inputs / Next Transitions:**

* **If user selects Poll Option 1 (“🚀 Let’s do it!”) or replies “1”** (exactly “1” or the emoji button that maps to “Let’s do it”) **before timeout**:

  * Record `hasRespondedToCommitment = true`.
  * Transition immediately to `FEELING_PROMPT`.

* **If user selects Poll Option 2 (“⏳ Not yet”) or replies “2” or any other “Not yet” keyword**:

  * Record `hasRespondedToCommitment = false_for_today`.
  * Transition immediately to `END_OF_DAY` (for today; do not ask anything else until tomorrow).

* **If `COMMITMENT_TIMEOUT` expires (no reply within 12 hours)**:

  * Implicitly assume “Not yet.”
  * Same as above: `hasRespondedToCommitment = false_for_today` → `END_OF_DAY`.

---

### 3. Feeling Prompt State

**State Name:** `FEELING_PROMPT`
**Entry Condition:**

* `hasRespondedToCommitment = true` from `COMMITMENT_PROMPT`.

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll** with:

   * **Title**: “How do you feel about this first step?”
   * **Option 1**: “😊 Excited”
   * **Option 2**: “🤔 Curious”
   * **Option 3**: “😃 Motivated”
   * **Option 4**: “📖 Need info”
   * **Option 5**: “⚖️ Not sure”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   How do you feel about this first step?
   1. 😊 Excited
   2. 🤔 Curious
   3. 😃 Motivated
   4. 📖 Need info
   5. ⚖️ Not sure
   (Reply with “1”, “2”, “3”, “4”, or “5”)
   ```

3. Clear any previous “feeling” flags for today. For example:

   * `feelingResponse = null`
   * `feelingTimerStarted = true` (we just started the timer).

4. Start two parallel waits:

   * **Wait for user reply** (must be one of {“1”, “2”, “3”, “4”, “5”} if SMS/Other, or a Poll selection if WhatsApp).
   * **Wait for a “lag timer” to expire** ‒ e.g. `FEELING_TIMEOUT = 15 minutes`.
   * Also **watch for an on‐demand “Ready” override** (if they type “Ready”, meaning “send the intervention now,” we disregard any remaining wait time and proceed to random assignment immediately).

**Possible Inputs / Next Transitions:**

* **If user selects Poll Option 1–5 (or replies “1”–“5”) before any timer expires**:

  * Store `feelingResponse = [1..5]`.
  * Cancel `FEELING_TIMEOUT`.
  * Transition to `RANDOM_ASSIGNMENT`.

* **If user sends “Ready” at any time (instead of a “1–5”)**:

  * Cancel any pending timers.
  * Record `feelingResponse = “on_demand”` (or leave it null, since we only need to know they overrode).
  * Transition to `RANDOM_ASSIGNMENT`.

* **If `FEELING_TIMEOUT` (15 minutes) fires first**:

  * Set `feelingResponse = “timed_out”`.
  * Transition to `RANDOM_ASSIGNMENT`.

---

### 4. Random Assignment State

**State Name:** `RANDOM_ASSIGNMENT`
**Entry Condition:**

* We just exited `FEELING_PROMPT` because either (a) user picked “1−5,” (b) user typed “Ready,” or (c) feeling‐timer expired.

**Action When Entered:**

1. Compute a random boolean, for example:

   ```js
   flowAssignmentToday = (Math.random() < 0.5) ? "IMMEDIATE" : "REFLECTIVE";
   ```

   Immediately persist `flowAssignmentToday` for today’s session.

2. Do **not** send any message here; simply forward control to the next state based on `flowAssignmentToday`.

**Next Transition:**

* If `flowAssignmentToday == "IMMEDIATE"`, go to `SEND_INTERVENTION_IMMEDIATE`.
* If `flowAssignmentToday == "REFLECTIVE"`, go to `SEND_INTERVENTION_REFLECTIVE`.

---

### 5A. Send Intervention: Immediate Action Flow

**State Name:** `SEND_INTERVENTION_IMMEDIATE`
**Entry Condition:**

* `flowAssignmentToday = "IMMEDIATE"` from `RANDOM_ASSIGNMENT`.

**Action When Entered:**

1. Send a short, directive prompt (the “one‐minute micro habit”). For example:

   > **Immediate‐Action Message (WhatsApp or SMS):**
   > “Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you’re done, reply ‘Done.’”

2. Set up for the completion‐check:

   * Clear/initialize:

     * `completionResponseReceived = false`
     * `completionTimerStarted = true`
   * Start a **completion timer** (e.g. `COMPLETION_TIMEOUT = 30 minutes`).

     * This timer means “if they don’t say ‘Done’ or ‘No’ within 30 minutes from this moment, treat as no‐reply.”

3. Wait for user input or timeout.

**Possible Inputs / Next Transitions:**

* **If user replies “Done”** within `COMPLETION_TIMEOUT`:

  * Record `completionResponse = "done"`.
  * Cancel `COMPLETION_TIMEOUT`.
  * Transition to `REINFORCEMENT_FOLLOWUP`.

* **If user replies “No”** (exact literal “No”) within `COMPLETION_TIMEOUT`:

  * Record `completionResponse = "no"`.
  * Cancel `COMPLETION_TIMEOUT`.
  * Transition to `DID_YOU_GET_A_CHANCE`.

* **If user sends any other text (not “Done” or “No”)**:

  * Option A: Treat anything other than “Done” as “No,” or
  * Option B: Ignore until “Done”/“No” or timeout.
  * (In the original design, only “Done” vs. “No/no‐reply” matters. We recommend ignoring other texts.)

* **If `COMPLETION_TIMEOUT` expires** (no “Done” or “No”):

  * Record `completionResponse = "no_reply"`.
  * Transition to `DID_YOU_GET_A_CHANCE`.

---

### 5B. Send Intervention: Reflective Flow

**State Name:** `SEND_INTERVENTION_REFLECTIVE`
**Entry Condition:**

* `flowAssignmentToday = "REFLECTIVE"` from `RANDOM_ASSIGNMENT`.

**Action When Entered:**

1. Send a short, reflective prompt that still asks the participant to do the one‐minute micro habit. For example:

   > **Reflective‐Flow Message (WhatsApp or SMS):**
   > “Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply ‘Done.’”

2. Set up the completion‐check exactly as in the Immediate flow:

   * `completionResponseReceived = false`
   * `completionTimerStarted = true`
   * Start timer `COMPLETION_TIMEOUT = 30 minutes`.

3. Wait for user input or timeout.

**Possible Inputs / Next Transitions:**

* Exactly the same logic as in `SEND_INTERVENTION_IMMEDIATE`:

  * “Done” → → `REINFORCEMENT_FOLLOWUP`.
  * “No” → → `DID_YOU_GET_A_CHANCE`.
  * Timeout → → `DID_YOU_GET_A_CHANCE`.
  * Other text → ignore until “Done”/“No” or timeout.

*(In short, the only difference between Immediate vs. Reflective is the wording of the message you send. After sending, both do exactly the same completion logic.)*

---

### 6. Reinforcement Follow‐Up (Completion = Yes)

**State Name:** `REINFORCEMENT_FOLLOWUP`
**Entry Condition:**

* `completionResponse = "done"` from either `SEND_INTERVENTION_IMMEDIATE` or `SEND_INTERVENTION_REFLECTIVE`.

**Action When Entered:**

1. Immediately send a short “Great job!” message (WhatsApp or SMS):

   > **“Great job! 🎉 You just completed your habit in under one minute—keep it up!”**

2. Increment a persistent counter:

   * `timesCompletedToday = timesCompletedToday + 1`.
   * (Later aggregated into `timesCompletedThisWeek` for weekly summary.)

3. Mark `hasBeenReinforcedToday = true` so we don’t double‐send encouragement if they say “Done” again.

4. **End today’s flow**. In other words, no further prompts until the weekly summary (or next day’s scheduler).

   * We do **not** ask context or mood when they replied “Done” immediately. The design says “No extra questions on a successful immediate response.”

**Exit Condition:**

* After sending this message, transition to `END_OF_DAY` state.

---

### 7. “Did You Get a Chance?” (Completion = No or No‐Reply)

**State Name:** `DID_YOU_GET_A_CHANCE`
**Entry Condition:**

* `completionResponse` is either “no” (explicit “No”) or “no\_reply” (timer expired) from either immediate or reflective intervention.

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll** with:

   * **Title**: “Did you get a chance to try it?”
   * **Option 1**: “Yes”
   * **Option 2**: “No”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   Did you get a chance to try it?
   1. Yes
   2. No
   (Reply with “1” or “2”)
   ```

3. Clear/initialize:

   * `gotChanceResponse = null`
   * Start a timer `GOT_CHANCE_TIMEOUT = 15 minutes`.

4. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user selects Poll Option 1 (“Yes”) or replies “1”** before timeout:

  * Set `gotChanceResponse = true`.
  * Cancel `GOT_CHANCE_TIMEOUT`.
  * Transition to `CONTEXT_QUESTION`.

* **If user selects Poll Option 2 (“No”) or replies “2”** before timeout:

  * Set `gotChanceResponse = false`.
  * Cancel `GOT_CHANCE_TIMEOUT`.
  * Transition to `BARRIER_REASON_NO_CHANCE`.

* **If `GOT_CHANCE_TIMEOUT` expires** (no reply within 15 min):

  * Set `gotChanceResponse = "no_reply"`.
  * Transition to `IGNORED_PATH`.

* **If user sends anything else** (free‐text not “Yes/No”):

  * Option A: Attempt to parse out “Yes” or “No” keywords. If you detect either, route accordingly.
  * Option B: If you cannot parse, keep waiting until `GOT_CHANCE_TIMEOUT`. (In practice, if they don’t respond, we go to Ignored.)

---

### 8. Context Question (They Tried = Yes)

**State Name:** `CONTEXT_QUESTION`
**Entry Condition:**

* `gotChanceResponse = true` from `DID_YOU_GET_A_CHANCE`.

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll**:

   * **Title**: “You did it! What was happening around you?”
   * **Option 1**: “Alone & focused”
   * **Option 2**: “With others around”
   * **Option 3**: “In a distracting place”
   * **Option 4**: “Busy & stressed”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   You did it! What was happening around you?
   1. Alone & focused
   2. With others around
   3. In a distracting place
   4. Busy & stressed
   (Reply with “1”, “2”, “3”, or “4”)
   ```

3. Initialize:

   * `contextResponse = null`
   * `contextTimerStarted = true`
   * Start `CONTEXT_TIMEOUT = 15 minutes` (if they don’t answer within 15 minutes, skip to weekly summary).

4. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user selects Poll Option 1–4 or replies “1”–“4”** before timeout:

  * Set `contextResponse = [1..4]`.
  * Cancel `CONTEXT_TIMEOUT`.
  * Transition to `MOOD_QUESTION`.

* **If `CONTEXT_TIMEOUT` expires** (no valid code within 15 minutes):

  * Leave `contextResponse = null`.
  * Transition directly to `END_OF_DAY` (skip mood and barrier steps).

* **If user sends anything else**:

  * Optionally parse if they text a free answer. In the published protocol, they should pick 1–4. If they send something else, either ignore or interpret as “4 = Busy & stressed.” After 15 minutes, if no clear 1–4, skip ahead to `END_OF_DAY`.

---

### 9. Mood Question (Only if Context Provided)

**State Name:** `MOOD_QUESTION`
**Entry Condition:**

* `contextResponse ∈ {1,2,3,4}` from `CONTEXT_QUESTION`.

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll**:

   * **Title**: “What best describes your mood before doing this?”
   * **Option 1**: “🙂 Relaxed”
   * **Option 2**: “😐 Neutral”
   * **Option 3**: “😫 Stressed”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   What best describes your mood before doing this?
   1. 🙂 Relaxed
   2. 😐 Neutral
   3. 😫 Stressed
   (Reply with “1”, “2”, or “3”)
   ```

3. Initialize:

   * `moodResponse = null`
   * `moodTimerStarted = true`
   * Start `MOOD_TIMEOUT = 15 minutes`.

4. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user selects Poll Option 1–3 or replies “1”–“3”** before timeout:

  * Map “1” → Relaxed, “2” → Neutral, “3” → Stressed.
  * Record `moodResponse` accordingly.
  * Cancel `MOOD_TIMEOUT`.
  * Transition to `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.

* **If `MOOD_TIMEOUT` expires** (no valid reply within 15 minutes):

  * Set `moodResponse = null`.
  * Transition directly to `END_OF_DAY` (skip barrier check).

* **If user sends any other text**:

  * Optionally parse “Relaxed”/“Neutral”/“Stressed”; otherwise ignore until timeout. After 15 minutes, skip ahead.

---

### 10. Barrier Check After Context & Mood

**State Name:** `BARRIER_CHECK_AFTER_CONTEXT_MOOD`
**Entry Condition:**

* `moodResponse ∈ {“Relaxed”, “Neutral”, “Stressed”}` from `MOOD_QUESTION`.

**Action When Entered:**

1. **If channel = WhatsApp or SMS/Other →** send a free‐text prompt:

   > **“Did something make this easier or harder today? What was it?”**
   > (Participants can type anything—no Poll is used here.)

2. Initialize:

   * `barrierDetailResponse = null`
   * `barrierDetailTimerStarted = true`
   * Start `BARRIER_DETAIL_TIMEOUT = 30 minutes`.

3. Wait for any user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user sends any text** (free‐form) within 30 minutes:

  * Record `barrierDetailResponse = [that text]`.
  * Transition to `END_OF_DAY`.

* **If `BARRIER_DETAIL_TIMEOUT` expires** (no reply in 30 minutes):

  * Leave `barrierDetailResponse = null`.
  * Transition to `END_OF_DAY`.

*(Note: Once you ask this free‐text barrier question, there are no further daily questions. Whether they reply or not, end the flow for the day.)*

---

### 11. Barrier Reason: “No Chance to Try” Path

**State Name:** `BARRIER_REASON_NO_CHANCE`
**Entry Condition:**

* `gotChanceResponse = false` from `DID_YOU_GET_A_CHANCE`.

**Action When Entered:**

1. **If channel = WhatsApp →** send a **Poll**:

   * **Title**: “Could you let me know why you couldn’t do it this time?”
   * **Option 1**: “I didn’t have enough time”
   * **Option 2**: “I didn’t understand the task”
   * **Option 3**: “I didn’t feel motivated to do it”
   * **Option 4**: “Other (please specify)”

2. **If channel = SMS/Other →** send a plain‐text message:

   ```
   Could you let me know why you couldn’t do it this time?
   1. I didn’t have enough time
   2. I didn’t understand the task
   3. I didn’t feel motivated to do it
   4. Other (please specify)
   (Reply with “1”, “2”, “3”, or “4”)
   ```

3. Initialize:

   * `barrierReasonResponse = null`
   * `barrierReasonTimerStarted = true`
   * Start `BARRIER_REASON_TIMEOUT = 30 minutes`.

4. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user selects Poll Option 1–3 or replies “1”–“3”** before timeout:

  * Record `barrierReasonResponse = [1..3]` (which you might map back to the exact text).
  * Transition → `END_OF_DAY`.

* **If user selects Poll Option 4 (“Other”) or replies “4”** before timeout:

  * Immediately send a follow‐up free‐text prompt (if Poll doesn’t natively allow text in the same step):

    > “Please tell us briefly why…”
  * Wait up to 30 minutes total for a free‐text reply; record whatever they send into `barrierReasonResponse` (as free‐text).
  * Transition → `END_OF_DAY`.

* **If `BARRIER_REASON_TIMEOUT` expires** (no reply in 30 minutes):

  * Leave `barrierReasonResponse = null`.
  * Transition → `END_OF_DAY`.

*(No further questions after Barrier Reason—end the day’s flow.)*

---

### 12. Ignored Path (No “Did You Get a Chance?” Reply)

**State Name:** `IGNORED_PATH`
**Entry Condition:**

* `gotChanceResponse = “no_reply”` from `DID_YOU_GET_A_CHANCE` (i.e. they never answered “Yes” or “No”).

**Action When Entered:**

1. **If channel = WhatsApp or SMS/Other →** send two messages in sequence (free text—no Poll):

   1. “What kept you from doing it today? Reply with one word, a quick audio, or a short video!”
   2. “Building awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day.”

2. Mark `ignoredReminderSent = true`.

3. Immediately transition → `END_OF_DAY`.

*(There are no further daily questions for someone who never responded to “Did you get a chance?”—we simply encourage them and end the day.)*

---

### 13. End‐of‐Day State

**State Name:** `END_OF_DAY`
**Entry Condition:**

* Reached from any of these preceding states:

  * `REINFORCEMENT_FOLLOWUP`
  * `BARRIER_CHECK_AFTER_CONTEXT_MOOD`
  * `BARRIER_REASON_NO_CHANCE`
  * `IGNORED_PATH`
  * Timeout from `CONTEXT_QUESTION`
  * Timeout from `MOOD_QUESTION`

**Action When Entered:**

1. Mark `dayFlowCompleted = true`.
2. No further messages are sent until either:

   * The **daily scheduler** re‐fires at 00:00 AM local (or at the chosen “prompt hour” tomorrow) → it will next run `COMMITMENT_PROMPT`.
   * The **weekly summary scheduler** fires (if 7 days have elapsed since enrollment or last weekly summary).
   * The participant types “Ready” (which will override and immediately trigger the next day’s prompts).

**Note:**

* If the participant sends an out‐of‐band message (anything that does not match any of the recognized inputs) while in `END_OF_DAY`, ignore it or optionally reply with a generic message such as:

  > “We’re all set for today; we’ll be back tomorrow with your daily prompt.”
* Remain in `END_OF_DAY` until one of the three triggers above occurs.

---

### 14. Weekly Summary State

**State Name:** `WEEKLY_SUMMARY`
**Entry Condition:**

* It has been exactly **7 days** since the last time we sent a weekly summary (or since enrollment, for the first one).
* Alternatively, your scheduler can check every midnight: “Has `today – weekStartDate ≥ 7 days`? If yes, fire `WEEKLY_SUMMARY`.”

**Action When Entered:**

1. Compute:

   ```
   timesCompletedThisWeek = count of days in the past 7 where
                            completionResponse == "done"
   ```

2. Send a single message (WhatsApp or SMS):

   > **“Great job this week! 🎉 You completed your habit `[timesCompletedThisWeek]` times in the past 7 days! 🙌 Keep up the momentum—small actions add up!”**

3. Reset:

   * `timesCompletedThisWeek = 0`
   * `weekStartDate = today`  (so the next summary occurs seven days from now).

4. Transition → `END_OF_DAY` (await tomorrow’s daily scheduler).

---

## Putting It All in Sequence

Below is a bullet‐point view of how a participant’s day might unfold. Whenever you see a label in all caps (like `FEELING_PROMPT`), that refers to one of the states above. Wherever a multiple‐choice question appears, note the two alternatives: “(WhatsApp Poll)” vs. “(SMS/Other).”

1. **Daily Scheduler triggers `COMMITMENT_PROMPT` at 10 AM local time.**

   * If the participant types **“Ready”** earlier (after previous day is done), cancel the scheduled 10 AM send and immediately run `COMMITMENT_PROMPT`.

2. **State = `COMMITMENT_PROMPT`.**

   * **(WhatsApp Poll):** “You committed to trying a quick habit today—ready to go? 1=🚀 Let’s do it! 2=⏳ Not yet.”
   * **(SMS/Other):** “You committed to trying a quick habit today—ready to go?

     1. 🚀 Let’s do it!
     2. ⏳ Not yet
        (Reply with ‘1’ or ‘2’)”
   * If no reply by 10 PM (or 12 h timeout) or “2” arrives → `END_OF_DAY`.
   * If “1” arrives → `hasRespondedToCommitment = true` → `FEELING_PROMPT`.

3. **State = `FEELING_PROMPT`.**

   * **(WhatsApp Poll):** “How do you feel about this first step? 1=😊 Excited 2=🤔 Curious 3=😃 Motivated 4=📖 Need info 5=⚖️ Not sure.”
   * **(SMS/Other):** “How do you feel about this first step?

     1. 😊 Excited
     2. 🤔 Curious
     3. 😃 Motivated
     4. 📖 Need info
     5. ⚖️ Not sure
        (Reply with ‘1’, ‘2’, ‘3’, ‘4’, or ‘5’)”
   * Wait up to 15 minutes:

     * If Poll Option 1–5 (or “1”–“5”) arrives → `feelingResponse = [1..5]` → cancel timer → `RANDOM_ASSIGNMENT`.
     * If user sends “Ready” → `feelingResponse = on_demand` → cancel timer → `RANDOM_ASSIGNMENT`.
     * If 15 min expire → `feelingResponse = timed_out` → `RANDOM_ASSIGNMENT`.

4. **State = `RANDOM_ASSIGNMENT`.**

   * Flip a coin → `flowAssignmentToday = "IMMEDIATE"` or `"REFLECTIVE"`.
   * If “IMMEDIATE” → `SEND_INTERVENTION_IMMEDIATE`.
   * If “REFLECTIVE” → `SEND_INTERVENTION_REFLECTIVE`.

5. **State = `SEND_INTERVENTION_IMMEDIATE`** (or `SEND_INTERVENTION_REFLECTIVE`).

   * **Common for both branches (WhatsApp or SMS):**

     * **Immediate‐Action Text:**

       > “Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you’re done, reply ‘Done.’”
     * **Reflective‐Flow Text:**

       > “Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply ‘Done.’”
   * Start a `COMPLETION_TIMEOUT` (30 minutes).
   * Wait for “Done” or “No” or timeout:

     * If “Done” → `completionResponse = done` → cancel timer → `REINFORCEMENT_FOLLOWUP`.
     * If “No” → `completionResponse = no` → cancel timer → `DID_YOU_GET_A_CHANCE`.
     * If timeout → `completionResponse = no_reply` → `DID_YOU_GET_A_CHANCE`.
     * If any other text → ignore until “Done”/“No” or timeout.

6. **State = `REINFORCEMENT_FOLLOWUP`.**

   * Send: “Great job! 🎉 You just completed your habit in under one minute—keep it up!”
   * Increment `timesCompletedToday += 1`.
   * Mark `hasBeenReinforcedToday = true`.
   * → `END_OF_DAY`.

7. **State = `DID_YOU_GET_A_CHANCE`.**

   * **(WhatsApp Poll):** “Did you get a chance to try it? 1=Yes 2=No.”
   * **(SMS/Other):** “Did you get a chance to try it?

     1. Yes
     2. No
        (Reply with ‘1’ or ‘2’)”
   * Start a `GOT_CHANCE_TIMEOUT` (15 minutes).
   * Wait:

     * If Poll Option 1 (“Yes”) or reply “1” → `gotChanceResponse = true` → cancel timer → `CONTEXT_QUESTION`.
     * If Poll Option 2 (“No”) or reply “2” → `gotChanceResponse = false` → cancel timer → `BARRIER_REASON_NO_CHANCE`.
     * If timeout → `gotChanceResponse = no_reply` → `IGNORED_PATH`.
     * If other text → attempt to parse “yes”/“no” or ignore until timeout.

8. **State = `CONTEXT_QUESTION`.**

   * **(WhatsApp Poll):** “You did it! What was happening around you? 1=Alone & focused 2=With others around 3=In a distracting place 4=Busy & stressed.”
   * **(SMS/Other):** “You did it! What was happening around you?

     1. Alone & focused
     2. With others around
     3. In a distracting place
     4. Busy & stressed
        (Reply with ‘1’, ‘2’, ‘3’, or ‘4’)”
   * Start a `CONTEXT_TIMEOUT` (15 minutes).
   * Wait:

     * If Poll Option 1–4 or reply “1”–“4” → `contextResponse = [1..4]` → cancel timer → `MOOD_QUESTION`.
     * If timeout → `contextResponse = null` → `END_OF_DAY`.
     * If other text → attempt to parse or ignore until timeout.

9. **State = `MOOD_QUESTION`.**

   * **(WhatsApp Poll):** “What best describes your mood before doing this? 1=🙂 Relaxed 2=😐 Neutral 3=😫 Stressed.”
   * **(SMS/Other):** “What best describes your mood before doing this?

     1. 🙂 Relaxed
     2. 😐 Neutral
     3. 😫 Stressed
        (Reply with ‘1’, ‘2’, or ‘3’)”
   * Start a `MOOD_TIMEOUT` (15 minutes).
   * Wait:

     * If Poll Option 1–3 or reply “1”–“3” → `moodResponse` accordingly → cancel timer → `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.
     * If timeout → `moodResponse = null` → `END_OF_DAY`.
     * If other text → ignore until timeout.

10. **State = `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.**

    * Send free‐text prompt (WhatsApp or SMS):

      > “Did something make this easier or harder today? What was it?”
    * Start a `BARRIER_DETAIL_TIMEOUT` (30 minutes).
    * Wait:

      * If user sends any text → `barrierDetailResponse = [text]` → `END_OF_DAY`.
      * If timeout → `barrierDetailResponse = null` → `END_OF_DAY`.

11. **State = `BARRIER_REASON_NO_CHANCE`.**

    * **(WhatsApp Poll):** “Could you let me know why you couldn’t do it this time? 1=I didn’t have enough time 2=I didn’t understand the task 3=I didn’t feel motivated to do it 4=Other (please specify).”
    * **(SMS/Other):** “Could you let me know why you couldn’t do it this time?

      1. I didn’t have enough time
      2. I didn’t understand the task
      3. I didn’t feel motivated to do it
      4. Other (please specify)
         (Reply with ‘1’, ‘2’, ‘3’, or ‘4’)”
    * Start a `BARRIER_REASON_TIMEOUT` (30 minutes).
    * Wait:

      * If Poll Option 1–3 or reply “1”–“3” → `barrierReasonResponse = [1..3]` → `END_OF_DAY`.
      * If Poll Option 4 or reply “4” → send follow‐up free‐text prompt (“Please specify why…”) → wait up to 30 minutes for a free‐text reply → record in `barrierReasonResponse` → `END_OF_DAY`.
      * If timeout → `barrierReasonResponse = null` → `END_OF_DAY`.

12. **State = `IGNORED_PATH`.**

    * Send two free‐text messages (WhatsApp or SMS):

      1. “What kept you from doing it today? Reply with one word, a quick audio, or a short video!”
      2. “Building awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day.”
    * Mark `ignoredReminderSent = true`.
    * Immediately → `END_OF_DAY`.

13. **State = `END_OF_DAY`.**

    * Mark `dayFlowCompleted = true`.
    * Wait until next day’s scheduler or “Ready” override or the weekly summary trigger.
    * If the participant sends any out‐of‐band message while in `END_OF_DAY`, either ignore or optionally reply with:

      > “We’re all set for today; we’ll be back tomorrow with your daily prompt.”
    * Remain in `END_OF_DAY` until one of the three triggers occurs.

14. **Weekly Summary Scheduler** (runs daily at midnight, for example):

    * If `today - weekStartDate ≥ 7 days`:

      * Compute `timesCompletedThisWeek =` count of days in the past 7 where `completionResponse == "done"`.
      * Send (WhatsApp or SMS):

        > “Great job this week! 🎉 You completed your habit `[timesCompletedThisWeek]` times in the past 7 days! 🙌 Keep up the momentum—small actions add up!”
      * Reset `timesCompletedThisWeek = 0` and `weekStartDate = today`.
      * → `END_OF_DAY`.

---

## Data Model & Persistence

For each participant, you must at minimum store:

1. **Booleans/Flags:**

   * `hasSeenOrientation` (boolean)
   * `hasRespondedToCommitment` (boolean)
   * `flowAssignmentToday` (string: “IMMEDIATE” or “REFLECTIVE”)
   * `completionResponse` (string: “done,” “no,” “no\_reply”)
   * `gotChanceResponse` (boolean or “no\_reply”)
   * `contextResponse` (integer 1–4 or null)
   * `moodResponse` (string among {“Relaxed,” “Neutral,” “Stressed”} or null)
   * `barrierReasonResponse` (integer 1–4 or free‐text or null)
   * `barrierDetailResponse` (free‐text or null)

2. **Counters and Timestamps:**

   * `lastCommitmentDate` (date)
   * `timesCompletedToday` (integer, either 0 or 1)
   * `timesCompletedThisWeek` (integer)
   * `weekStartDate` (date)

3. **Timers (and/or “expires\_at” fields)**:

   * `COMMITMENT_TIMEOUT` (12 hours)
   * `FEELING_TIMEOUT` (15 minutes)
   * `COMPLETION_TIMEOUT` (30 minutes)
   * `GOT_CHANCE_TIMEOUT` (15 minutes)
   * `CONTEXT_TIMEOUT` (15 minutes)
   * `MOOD_TIMEOUT` (15 minutes)
   * `BARRIER_DETAIL_TIMEOUT` (30 minutes)
   * `BARRIER_REASON_TIMEOUT` (30 minutes)

Timers can be implemented as expirations in a job queue or as “expires\_at” UTC timestamps stored in the participant record, with a background worker that scans for expired timers every minute.

---

## Example “If/Else” Logic in Pseudodescriptive Form

Below is an abridged, step‐by‐step “if/else”-style description for one day. You can see exactly how each state leads to the next, including the channel‐specific Poll vs. SMS logic.

1. **(Scheduler fires at 10:00 AM or user types “Ready”)**

   * If `hasSeenOrientation = false`, send `ORIENTATION`, set `hasSeenOrientation = true`. Skip to `END_OF_DAY` (no other prompts today).
   * Else (`hasSeenOrientation = true`) → `COMMITMENT_PROMPT`.

2. **`COMMITMENT_PROMPT`:**

   * **(WhatsApp Poll):** “You committed to trying… 1=🚀 Let’s do it! 2=⏳ Not yet.”
   * **(SMS/Other):** “You committed to trying…

     1. 🚀 Let’s do it!
     2. ⏳ Not yet
        (Reply with ‘1’ or ‘2’)”
   * Wait for “1” or “2” or 12 hours.
   * If “1” → `hasRespondedToCommitment = true` → `FEELING_PROMPT`.
   * Else (“2” or no reply by timeout) → `hasRespondedToCommitment = false_for_today` → `END_OF_DAY`.

3. **`FEELING_PROMPT`:**

   * **(WhatsApp Poll):** “How do you feel? 1=😊 Excited 2=🤔 Curious 3=😃 Motivated 4=📖 Need info 5=⚖️ Not sure.”
   * **(SMS/Other):** “How do you feel?

     1. 😊 Excited
     2. 🤔 Curious
     3. 😃 Motivated
     4. 📖 Need info
     5. ⚖️ Not sure
        (Reply with ‘1’–‘5’)”
   * Wait 15 minutes or “Ready.”
   * If Poll Option 1–5 or reply “1”–“5” → `feelingResponse = [1..5]` → cancel timer → `RANDOM_ASSIGNMENT`.
   * If “Ready” → `feelingResponse = on_demand` → cancel timer → `RANDOM_ASSIGNMENT`.
   * If timeout → `feelingResponse = timed_out` → `RANDOM_ASSIGNMENT`.

4. **`RANDOM_ASSIGNMENT`:**

   * Flip coin → `flowAssignmentToday = "IMMEDIATE"` or `"REFLECTIVE"`.
   * If “IMMEDIATE” → `SEND_INTERVENTION_IMMEDIATE`.
   * Else (“REFLECTIVE”) → `SEND_INTERVENTION_REFLECTIVE`.

5. **`SEND_INTERVENTION_IMMEDIATE` / `SEND_INTERVENTION_REFLECTIVE`:**

   * **Immediate‐Action or Reflective‐Flow text** (WhatsApp & SMS).
   * Start 30 min `COMPLETION_TIMEOUT`.
   * Wait for “Done” or “No” or timeout.
   * If “Done” → `completionResponse = done` → cancel timer → `REINFORCEMENT_FOLLOWUP`.
   * If “No” → `completionResponse = no` → cancel timer → `DID_YOU_GET_A_CHANCE`.
   * If timeout → `completionResponse = no_reply` → `DID_YOU_GET_A_CHANCE`.
   * Else ignore until one of those three.

6. **`REINFORCEMENT_FOLLOWUP`:**

   * Send “Great job!” (WhatsApp or SMS).
   * Increment `timesCompletedToday += 1`.
   * → `END_OF_DAY`.

7. **`DID_YOU_GET_A_CHANCE`:**

   * **(WhatsApp Poll):** “Did you get a chance to try it? 1=Yes 2=No.”
   * **(SMS/Other):** “Did you get a chance to try it?

     1. Yes
     2. No
        (Reply with ‘1’ or ‘2’)”
   * Start 15 min `GOT_CHANCE_TIMEOUT`.
   * If Poll Option 1 or “1” → `gotChanceResponse = true` → cancel timer → `CONTEXT_QUESTION`.
   * If Poll Option 2 or “2” → `gotChanceResponse = false` → cancel timer → `BARRIER_REASON_NO_CHANCE`.
   * If timeout → `gotChanceResponse = no_reply` → `IGNORED_PATH`.
   * Else ignore until one of those.

8. **`CONTEXT_QUESTION`:**

   * **(WhatsApp Poll):** “You did it! What was happening around you? 1=Alone & focused 2=With others around 3=In a distracting place 4=Busy & stressed.”
   * **(SMS/Other):** “You did it! What was happening around you?

     1. Alone & focused
     2. With others around
     3. In a distracting place
     4. Busy & stressed
        (Reply with ‘1’–‘4’)”
   * Start 15 min `CONTEXT_TIMEOUT`.
   * If Poll Option 1–4 or “1”–“4” → `contextResponse = [1..4]` → cancel timer → `MOOD_QUESTION`.
   * If timeout → `contextResponse = null` → `END_OF_DAY`.
   * Else ignore until one of those.

9. **`MOOD_QUESTION`:**

   * **(WhatsApp Poll):** “What best describes your mood? 1=🙂 Relaxed 2=😐 Neutral 3=😫 Stressed.”
   * **(SMS/Other):** “What best describes your mood?

     1. 🙂 Relaxed
     2. 😐 Neutral
     3. 😫 Stressed
        (Reply with ‘1’–‘3’)”
   * Start 15 min `MOOD_TIMEOUT`.
   * If Poll Option 1–3 or “1”–“3” → `moodResponse` accordingly → cancel timer → `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.
   * If timeout → `moodResponse = null` → `END_OF_DAY`.
   * Else ignore until one of those.

10. **`BARRIER_CHECK_AFTER_CONTEXT_MOOD`:**

    * Send free‐text prompt: “Did something make this easier or harder today? What was it?”
    * Start 30 min `BARRIER_DETAIL_TIMEOUT`.
    * If user types any text → `barrierDetailResponse = [text]` → `END_OF_DAY`.
    * If timeout → `barrierDetailResponse = null` → `END_OF_DAY`.

11. **`BARRIER_REASON_NO_CHANCE`:**

    * **(WhatsApp Poll):** “Why couldn’t you do it this time? 1=I didn’t have enough time 2=I didn’t understand the task 3=I didn’t feel motivated to do it 4=Other.”
    * **(SMS/Other):** “Why couldn’t you do it this time?

      1. I didn’t have enough time
      2. I didn’t understand the task
      3. I didn’t feel motivated to do it
      4. Other (please specify)
         (Reply with ‘1’–‘4’)”
    * Start 30 min `BARRIER_REASON_TIMEOUT`.
    * If Poll Option 1–3 or “1”–“3” → `barrierReasonResponse = [1..3]` → `END_OF_DAY`.
    * If Poll Option 4 or “4” → send “Please specify why…” → wait up to 30 min for free‐text → record in `barrierReasonResponse` → `END_OF_DAY`.
    * If timeout → `barrierReasonResponse = null` → `END_OF_DAY`.

12. **`IGNORED_PATH`:**

    * Send two free‐text messages (no Poll):

      1. “What kept you from doing it today? Reply with one word, a quick audio, or a short video!”
      2. “Building awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day.”
    * `ignoredReminderSent = true`.
    * → `END_OF_DAY`.

13. **`END_OF_DAY`:**

    * Mark `dayFlowCompleted = true`.
    * Wait until next day’s `COMMITMENT_PROMPT` or “Ready” or Weekly Summary.
    * If out‐of‐band message arrives (e.g. “Hello”), optionally reply with: “We’re all set for today; we’ll be back tomorrow with your daily prompt.”
    * Remain in `END_OF_DAY`.

14. **Weekly Summary (background job each midnight):**

    * If `today - weekStartDate ≥ 7 days`, compute `timesCompletedThisWeek` and send:

      > “Great job this week! 🎉 You completed your habit `[timesCompletedThisWeek]` times in the past 7 days! 🙌 Keep up the momentum—small actions add up!”
    * Reset `timesCompletedThisWeek = 0` and `weekStartDate = today`.
    * → `END_OF_DAY`.

---

## Data Model & Persistence

For each participant, you must at minimum store:

1. **Booleans/Flags:**

   * `hasSeenOrientation` (boolean)
   * `hasRespondedToCommitment` (boolean)
   * `flowAssignmentToday` (string: “IMMEDIATE” or “REFLECTIVE”)
   * `completionResponse` (string: “done,” “no,” “no\_reply”)
   * `gotChanceResponse` (boolean or “no\_reply”)
   * `contextResponse` (integer 1–4 or null)
   * `moodResponse` (string among {“Relaxed,” “Neutral,” “Stressed”} or null)
   * `barrierReasonResponse` (integer 1–4 or free‐text or null)
   * `barrierDetailResponse` (free‐text or null)

2. **Counters and Timestamps:**

   * `lastCommitmentDate` (date)
   * `timesCompletedToday` (integer, 0 or 1)
   * `timesCompletedThisWeek` (integer)
   * `weekStartDate` (date)

3. **Timers (or “expires\_at” fields):**

   * `COMMITMENT_TIMEOUT` (12 hours)
   * `FEELING_TIMEOUT` (15 minutes)
   * `COMPLETION_TIMEOUT` (30 minutes)
   * `GOT_CHANCE_TIMEOUT` (15 minutes)
   * `CONTEXT_TIMEOUT` (15 minutes)
   * `MOOD_TIMEOUT` (15 minutes)
   * `BARRIER_DETAIL_TIMEOUT` (30 minutes)
   * `BARRIER_REASON_TIMEOUT` (30 minutes)

Timers can be implemented either via a task‐queue (scheduling a callback when they expire) or by storing an “expires\_at” timestamp in each participant’s record and having a background process poll for expirations every minute.

---

## Example “If/Else” Logic in Pseudodescriptive Form (Summary)

1. **(Scheduler fires or user sends “Ready”)**

   * If `hasSeenOrientation = false`, send `ORIENTATION`; set `hasSeenOrientation = true`; → `END_OF_DAY`.
   * Else → `COMMITMENT_PROMPT`.

2. **`COMMITMENT_PROMPT`:**

   * (WhatsApp Poll or SMS text) “Ready to do the habit today? 1=Yes 2=Not yet.”
   * If “1” → `hasRespondedToCommitment = true` → `FEELING_PROMPT`.
   * Else (“2” or timeout) → `hasRespondedToCommitment = false_for_today` → `END_OF_DAY`.

3. **`FEELING_PROMPT`:**

   * (WhatsApp Poll or SMS) “How do you feel? 1–5.”
   * If Poll Option 1–5 or SMS “1”–“5” → `feelingResponse` → `RANDOM_ASSIGNMENT`.
   * If SMS “Ready” → `feelingResponse = on_demand` → `RANDOM_ASSIGNMENT`.
   * If 15 min timeout → `feelingResponse = timed_out` → `RANDOM_ASSIGNMENT`.

4. **`RANDOM_ASSIGNMENT`:**

   * Flip coin → “IMMEDIATE” or “REFLECTIVE” → corresponding next state.

5. **`SEND_INTERVENTION_IMMEDIATE` / `SEND_INTERVENTION_REFLECTIVE`:**

   * Send action vs. reflection text.
   * Wait 30 min for “Done” or “No” or timeout.
   * If “Done” → `REINFORCEMENT_FOLLOWUP`.
   * If “No” or timeout → `DID_YOU_GET_A_CHANCE`.

6. **`REINFORCEMENT_FOLLOWUP`:**

   * Send “Great job!” → increment `timesCompletedToday` → `END_OF_DAY`.

7. **`DID_YOU_GET_A_CHANCE`:**

   * (WhatsApp Poll or SMS) “Did you get a chance? 1=Yes 2=No.”
   * Wait 15 min.
   * If “1” → `CONTEXT_QUESTION`.
   * If “2” → `BARRIER_REASON_NO_CHANCE`.
   * If timeout → `IGNORED_PATH`.

8. **`CONTEXT_QUESTION`:**

   * (WhatsApp Poll or SMS) “You did it! What was happening around you? 1–4.”
   * Wait 15 min.
   * If “1”–“4” → `MOOD_QUESTION`.
   * Else timeout → `END_OF_DAY`.

9. **`MOOD_QUESTION`:**

   * (WhatsApp Poll or SMS) “What best describes your mood? 1=Relaxed 2=Neutral 3=Stressed.”
   * Wait 15 min.
   * If “1”–“3” → `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.
   * Else timeout → `END_OF_DAY`.

10. **`BARRIER_CHECK_AFTER_CONTEXT_MOOD`:**

    * Free‐text: “Did something make it easier or harder today? What was it?”
    * Wait 30 min.
    * On any reply or timeout → `END_OF_DAY`.

11. **`BARRIER_REASON_NO_CHANCE`:**

    * (WhatsApp Poll or SMS) “Why couldn’t you do it? 1–4 (Other → free‐text).”
    * Wait 30 min.
    * On any reply or timeout → `END_OF_DAY`.

12. **`IGNORED_PATH`:**

    * Free‐text encouragement: two‐part message.
    * → `END_OF_DAY`.

13. **`END_OF_DAY`:**

    * Wait until next day’s scheduler or “Ready” or Weekly Summary.

14. **Weekly Summary (daily midnight job):**

    * If `today - weekStartDate ≥ 7 days`: compute and send week summary → reset counters → `END_OF_DAY`.

---

## Key Implementation Details

1. **On‐Demand Override (“Ready”):**

   * At any time *after* a previous day’s flow has ended, if the system sees an incoming text exactly equal to “Ready” (case‐insensitive), forcibly start that participant’s `COMMITMENT_PROMPT` state immediately—regardless of the daily scheduled time.
   * Once you do that, cancel any previously scheduled daily prompt for that participant (to avoid duplicating).

2. **Timers and Cancellations:**

   * If a participant replies early (e.g. sends “1” to skip the rest of the commitment wait), you must cancel any outstanding timers (e.g. the 12 hr `COMMITMENT_TIMEOUT` or 15 min `FEELING_TIMEOUT`).
   * Implement timers either as “expires\_at” fields plus a polling worker or as a true job in a task queue that calls back to your code when the timeout is hit.

3. **Persistent Data Store:**

   * Each participant’s conversation must be stored in a database table keyed by `participantId` (and perhaps by `date`).
   * Every message you send should be logged (with a timestamp) along with any user reply you receive, plus the time you recorded it. This enables you to compute the weekly summary.

4. **Random Assignment Consistency:**

   * The published design calls for randomizing each day anew (50/50 chance each day).
   * If you want to run it as a true RCT, ensure your random number generator is seeded unpredictably so you don’t accidentally bias the assignment.

5. **Weekly Summary Scheduling:**

   * You can implement a rolling‐window approach: after sending each participant their first weekly summary on Day 7, record `weekStartDate = Day 0`. Then schedule the next summary for Day 14, etc.
   * Or, if you only care about the first week, schedule a one‐time job 7 days after enrollment; after you send it, you can disable further summaries if you only need one.

6. **Edge Cases:**

   * If a participant replies with some random text (“What’s up?”) while in `END_OF_DAY`, you can ignore or politely respond with a generic “Thanks for your message. We’ll be back tomorrow with a prompt.” They should not be accidentally funneled into any intermediate state.
   * If a participant replies multiple times (e.g. says “Done” twice), record the first “Done” as `completionResponse = “done”` and ignore subsequent “Done” messages until tomorrow.

---

## Final State Diagram (for Reference)

Below is how all the states link to each other. You can use this as a “roadmap” when writing your code:

```
ENROLLMENT (ORIENTATION)
       ↓
COMMITMENT_PROMPT ─── “1” ──▶ FEELING_PROMPT ───▶ RANDOM_ASSIGNMENT ──▶
    │                          │
    │ “2” or timeout          if flow=IMMEDIATE         flow=REFLECTIVE
    ↓                          ↓                          ↓
  END_OF_DAY                  SEND_INTERVENTION_IMMEDIATE
                              (or REFLECTIVE) ─┐
                                                  ↓
                                      Wait for Done or No (30 min timeout)
                                                  ↓
                 ┌── If “Done” ─────▶ REINFORCEMENT_FOLLOWUP ──▶ END_OF_DAY
                 │
   ─────────────────▶ If “No” or timeout ──▶ DID_YOU_GET_A_CHANCE ↓
                                                         ↓
                                              ┌──── If “Yes” ──▶ CONTEXT_QUESTION ──▶ MOOD_QUESTION ──▶ BARRIER_CHECK_AFTER_CONTEXT_MOOD ──▶ END_OF_DAY
                                              │
                                              ├──── If “No” ──▶ BARRIER_REASON_NO_CHANCE ──▶ END_OF_DAY
                                              │
                                              └──── If no reply (timeout) ──▶ IGNORED_PATH ──▶ END_OF_DAY

(Separately, every midnight: check if 7 days passed → WEEKLY_SUMMARY → back to END_OF_DAY)
```

---

## Conclusion

This specification enumerates every state, every timer, and every conditional transition you need to implement the exact same logic as in Figures 1 and 2 from the WhatsApp study, with the added clarification that:

* **All multiple‐choice questions** are delivered as **native Polls on WhatsApp** (for a tap‐to‐choose experience), and
* They fall back to **ID‐based numeric replies on SMS/Other** (plain text) if Polls are not available.

A developer can now map each “state” to a function or method in their code, wire up timers or job‐queue events for each timeout, and route incoming WhatsApp Poll results or plain‐text “1”/“2” messages into these state machines. From there, you have everything needed to send precisely the right prompts, collect exactly the right Poll responses or free‐text replies, randomize appropriately, and generate a weekly summary.
