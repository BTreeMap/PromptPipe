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
* A handful of **timers** (feeling time‐out, completion time‐out, “did‐you‐get‐a‐chance” time‐out).
* A **randomization function** that flips a coin (e.g. `Math.random() < 0.5`) to choose Immediate vs. Reflective.
* A **scheduler** that (a) sends the daily prompts at a fixed time, (b) checks if a participant ever types “Ready,” and (c) triggers the weekly summary seven days after enrollment (or in rolling 7‐day windows).

Below is a breakdown of every state/transition. We name each “state” and then describe:

* **When it is entered** (what event causes you to transition into this state).
* **What the system sends** (the exact message content).
* **What inputs you are waiting for** (timeouts, specific user replies).
* **How to interpret those inputs** (which next state to jump to).

---

### 1. Enrollment & Orientation State

**State Name:** `ORIENTATION`
**Entry Condition:**

* Participant is newly enrolled (no prior history).

**Action When Entered:**

1. Send a single “Welcome” message once. For example:

   > “Hi $Name$, 🌱 Welcome to our Healthy Habits study!
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

1. Send exactly:

   > **“You committed to trying a quick habit today—ready to go?**
   >
   > 1. 🚀 Let’s do it!
   > 2. ⏳ Not yet”\*\*

 Use whichever chat API or WhatsApp template is appropriate, but the user must see “Reply ‘1’” for “Let’s do it” or “Reply ‘2’” for “Not yet.”

2. Persistently record:

   * `lastCommitmentDate = <today’s date>`
   * Clear out any previous state related to Day N’s flow so we can start fresh (e.g. `hasRespondedToCommitment = false`).

3. Start waiting for a user response or a secondary trigger:

   * **Wait for user input “1” or “2.”**
   * If the user does not reply within, say, `COMMITMENT_TIMEOUT = 12 hours` (or until 11:59 PM local time), automatically treat as if they replied “2 = Not yet.” However, the flow guidance says that if “Not yet,” we simply end the flow for today. That means we should send **no further messages** until tomorrow’s scheduler.

**Possible User Inputs / Next Transitions:**

* **If user sends “1”** (exactly “1” or the emoji button that equals “Let’s do it”):

  * Record `hasRespondedToCommitment = true`.
  * Transition immediately to **`FEELING_PROMPT`**.

* **If user sends “2” or any other “Not yet” keyword:**

  * Record `hasRespondedToCommitment = false_for_today`.
  * Transition immediately to **`END_OF_DAY`** for today (i.e. do not ask anything else today, wait until next day’s `COMMITMENT_PROMPT`).

* **If no reply within `COMMITMENT_TIMEOUT`:**

  * Implicitly assume “Not yet.”
  * Go to `END_OF_DAY`.

---

### 3. Feeling Prompt State

**State Name:** `FEELING_PROMPT`
**Entry Condition:**

* `hasRespondedToCommitment = true` from `COMMITMENT_PROMPT`.

**Action When Entered:**

1. Immediately send:

   > **“How do you feel about this first step?**
   >
   > 1. 😊 Excited
   > 2. 🤔 Curious
   > 3. 😃 Motivated
   > 4. 📖 Need info
   > 5. ⚖️ Not sure”\*\*

2. Clear any previous “feeling” flags for today. e.g.:

   * `feelingResponse = null`
   * `feelingTimerStarted = true` (we just started the timer).

3. Start two parallel waits:

   * **Wait for user reply** (must be one of {“1”, “2”, “3”, “4”, “5”}).
   * **Wait for a “lag timer” to expire** (e.g. `FEELING_TIMEOUT = 15 minutes`)
   * **Watch for an on‐demand “Ready” override** (if they type “Ready” again, which means “I want the prompt immediately,” disregard any remaining wait time and proceed to random assignment immediately).

**Possible Inputs / Next Transitions:**

* **If user sends a valid number (“1”–“5”) before any timer expires:**

  * Store `feelingResponse = [that number]`.
  * Cancel `FEELING_TIMEOUT` timer.
  * Immediately transition to **`RANDOM_ASSIGNMENT`**.

* **If user sends “Ready” at any time (instead of a “1–5”):**

  * Cancel any pending timers.
  * Treat this exactly as if they had replied one of the feelings codes (we don’t care which emotion, only that they want the prompt now). So set `feelingResponse = “on_demand”` (or leave it null if you prefer).
  * Transition immediately to **`RANDOM_ASSIGNMENT`**.

* **If `FEELING_TIMEOUT` (15 minutes) fires first:**

  * Set `feelingResponse = “timed_out”`.
  * Transition immediately to **`RANDOM_ASSIGNMENT`**.

---

### 4. Random Assignment State

**State Name:** `RANDOM_ASSIGNMENT`
**Entry Condition:**

* We just exited `FEELING_PROMPT` because either (a) user picked “1−5,” (b) user typed “Ready,” or (c) feeling‐timer expired.

**Action When Entered:**

1. Compute a random boolean:

   * `flowAssignmentToday = (random() < 0.5) ? "IMMEDIATE" : "REFLECTIVE"`.
   * Immediately persist `flowAssignmentToday` for today’s session.

2. Do not send any message here; we simply forward control to the next state based on `flowAssignmentToday`.

**Next Transition:**

* If `flowAssignmentToday == "IMMEDIATE"`, go to **`SEND_INTERVENTION_IMMEDIATE`**.
* If `flowAssignmentToday == "REFLECTIVE"`, go to **`SEND_INTERVENTION_REFLECTIVE`**.

---

### 5A. Send Intervention: Immediate Action Flow

**State Name:** `SEND_INTERVENTION_IMMEDIATE`
**Entry Condition:**

* `flowAssignmentToday = "IMMEDIATE"` from `RANDOM_ASSIGNMENT`.

**Action When Entered:**

1. Send a short, directive prompt (the “one‐minute micro habit”). For example (replace with your actual habit text):

   > **“Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you’re done, reply ‘Done.’”**

2. Set up for the completion‐check:

   * Clear/initialize:

     * `completionResponseReceived = false`
     * `completionTimerStarted = true`
   * Start a **completion timer** (e.g. `COMPLETION_TIMEOUT = 30 minutes`). This timer means “if they don’t say ‘Done’ or ‘No’ within 30 minutes from this moment, treat as no‐reply.”

3. Wait for user input or timeout.

**Possible Inputs / Next Transitions:**

* **If user replies “Done”** within `COMPLETION_TIMEOUT`:

  * Record `completionResponse = “done”`.
  * Cancel `COMPLETION_TIMEOUT` timer.
  * Transition to **`REINFORCEMENT_FOLLOWUP`**.

* **If user replies “No”** (exact literal “No” or a button that means “No, I choose not to do it”) within `COMPLETION_TIMEOUT`:

  * Record `completionResponse = “no”`.
  * Cancel `COMPLETION_TIMEOUT` timer.
  * Transition to **`DID_YOU_GET_A_CHANCE`**.

* **If user sends any other text (not “Done” or “No”)**:

  * Either treat as “No” (if the implementer wants to interpret anything other than “Done” as “No”), or politely re‐prompt. In the published study design, only “Yes” or “No” matter. We recommend:

    * If text != {“Done”, “No”}, ignore it (optionally store it as “other message”). Keep waiting until 30 minutes are up or the user eventually replies “Done” or “No.”

* **If `COMPLETION_TIMEOUT` expires** (no “Done” or “No”):

  * Record `completionResponse = “no_reply”`.
  * Transition to **`DID_YOU_GET_A_CHANCE`**.

---

### 5B. Send Intervention: Reflective Flow

**State Name:** `SEND_INTERVENTION_REFLECTIVE`
**Entry Condition:**

* `flowAssignmentToday = "REFLECTIVE"` from `RANDOM_ASSIGNMENT`.

**Action When Entered:**

1. Send a short, reflective prompt that still asks the participant to do the one‐minute micro habit. For example:

   > **“Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply ‘Done.’”**

2. Set up the completion‐check exactly as in the Immediate flow:

   * `completionResponseReceived = false`
   * `completionTimerStarted = true`
   * Start timer `COMPLETION_TIMEOUT = 30 minutes`.

3. Wait for user input or timeout.

**Possible Inputs / Next Transitions:**

* Exactly the same as in **`SEND_INTERVENTION_IMMEDIATE`**:

  * “Done” → → `REINFORCEMENT_FOLLOWUP`.
  * “No” → → `DID_YOU_GET_A_CHANCE`.
  * Timeout (30 min) → → `DID_YOU_GET_A_CHANCE`.
  * Any other text → ignore until “Done”/“No” or timeout.

*(In short, the only difference between Immediate vs. Reflective is the wording of the message you send. After sending, both do exactly the same completion logic.)*

---

### 6. Reinforcement Follow‐Up (Completion = Yes)

**State Name:** `REINFORCEMENT_FOLLOWUP`
**Entry Condition:**

* `completionResponse = “done”` from either `SEND_INTERVENTION_IMMEDIATE` or `SEND_INTERVENTION_REFLECTIVE`.

**Action When Entered:**

1. Immediately send a short “Great job!” message:

   > **“Great job! 🎉”** (Optionally add a personalized note, e.g. “You just completed your habit in under one minute—keep it up!”).

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

1. Send:

   > **“Did you get a chance to try it? (Yes/No)”**

2. Clear/initialize:

   * `gotChanceResponse = null`
   * Start a timer `GOT_CHANCE_TIMEOUT = 15 minutes`.

3. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user sends “Yes”** (or “yes,” case‐insensitive):

  * Set `gotChanceResponse = true`.
  * Cancel `GOT_CHANCE_TIMEOUT`.
  * Transition to **`CONTEXT_QUESTION`** (Step 8).

* **If user sends “No”** (case‐insensitive):

  * Set `gotChanceResponse = false`.
  * Cancel `GOT_CHANCE_TIMEOUT`.
  * Transition to **`BARRIER_REASON_NO_CHANCE`** (Step 9).

* **If `GOT_CHANCE_TIMEOUT` expires with no reply:**

  * Set `gotChanceResponse = “no_reply”`.
  * Transition to **`IGNORED_PATH`** (Step 10).

* **If user sends anything else** (e.g. free‐text not “Yes/No”):

  * Option A: Attempt to parse out “Yes” or “No” keywords. If you detect either, handle accordingly.
  * Option B: If you cannot parse, keep waiting until `GOT_CHANCE_TIMEOUT`. (In practice, the study text implies “If they don’t respond at all, go to Ignored.”)

---

### 8. Context Question (They Tried = Yes)

**State Name:** `CONTEXT_QUESTION`
**Entry Condition:**

* `gotChanceResponse = true` from `DID_YOU_GET_A_CHANCE`.

**Action When Entered:**

1. Send:

   > **“You did it! What was happening around you? Reply with a number:**
   >
   > 1. Alone & focused
   > 2. With others around
   > 3. In a distracting place
   > 4. Busy & stressed”\*\*

2. Initialize:

   * `contextResponse = null`
   * `contextTimerStarted = true`
   * Start `CONTEXT_TIMEOUT = 15 minutes` (if they don’t answer which context, we will skip to weekly summary).

3. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user sends “1”, “2”, “3”, or “4”**:

  * Set `contextResponse = [1–4]`.
  * Cancel `CONTEXT_TIMEOUT`.
  * Transition to **`MOOD_QUESTION`** (Step 9).

* **If `CONTEXT_TIMEOUT` expires** (no valid code):

  * Leave `contextResponse = null`.
  * Transition directly to **`END_OF_DAY`** (skip mood and barrier steps).

* **If user sends anything else**:

  * Optionally parse if they text their own free answer. In the published protocol, they should pick 1–4. If they send something else, either ignore or interpret as “4 = Busy & stressed.” After 15 minutes, if no clear 1–4, skip ahead.

---

### 9. Mood Question (Only if Context Provided)

**State Name:** `MOOD_QUESTION`
**Entry Condition:**

* `contextResponse ∈ {1,2,3,4}` from `CONTEXT_QUESTION`.

**Action When Entered:**

1. Send:

   > **“What best describes your mood before doing this?**
   > 🙂 Relaxed  |  😐 Neutral  |  😫 Stressed”\*\*

2. Initialize:

   * `moodResponse = null`
   * `moodTimerStarted = true`
   * Start `MOOD_TIMEOUT = 15 minutes`.

3. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user sends one of “Relaxed,” “Neutral,” or “Stressed”** (or “1,” “2,” “3,” if you prefer numeric codes):

  * Set `moodResponse` accordingly.
  * Cancel `MOOD_TIMEOUT`.
  * Transition to **`BARRIER_CHECK_AFTER_CONTEXT_MOOD`** (Step 10).

* **If `MOOD_TIMEOUT` expires** (no valid reply within 15 min):

  * Set `moodResponse = null`.
  * Transition directly to **`END_OF_DAY`** (skip barrier check).

* **If user sends any other text**:

  * Optionally parse “yes”/“no” words to convert, but best practice is to ignore anything not exactly “Relaxed/Neutral/Stressed.” After 15 min, skip ahead.

---

### 10. Barrier Check After Context & Mood

**State Name:** `BARRIER_CHECK_AFTER_CONTEXT_MOOD`
**Entry Condition:**

* `moodResponse ∈ {“Relaxed”, “Neutral”, “Stressed”}` from `MOOD_QUESTION`.

**Action When Entered:**

1. Send:

   > **“Did something make this easier or harder today? What was it?”**

   * This is a free‐text prompt—participants can type anything.

2. Initialize:

   * `barrierDetailResponse = null`
   * `barrierDetailTimerStarted = true`
   * Start `BARRIER_DETAIL_TIMEOUT = 30 minutes`.

3. Wait for any user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user sends any text** (free form):

  * Record `barrierDetailResponse = [that text]`.
  * Transition to **`END_OF_DAY`**.

* **If `BARRIER_DETAIL_TIMEOUT` expires** (no reply within 30 min):

  * Leave `barrierDetailResponse = null`.
  * Transition to **`END_OF_DAY`**.

*(Note: Once barrierQuestion is asked, there are no further questions. Even if they fail to reply, we still mark the end of the day’s flow.)*

---

### 11. Barrier Reason: “No Chance to Try” Path

**State Name:** `BARRIER_REASON_NO_CHANCE`
**Entry Condition:**

* `gotChanceResponse = false` from `DID_YOU_GET_A_CHANCE`.

**Action When Entered:**

1. Send:

   > **“Could you let me know why you couldn’t do it this time? Reply by typing, a quick audio, or a short video!”**

2. Then (immediately, in the same message or as a follow‐up) present multiple‐choice options:

   > **“Option B: Response Options:**
   >
   > 1. I didn’t have enough time
   > 2. I didn’t understand the task
   > 3. I didn’t feel motivated to do it
   > 4. Other (please specify)”\*\*

3. Initialize:

   * `barrierReasonResponse = null`
   * `barrierReasonTimerStarted = true`
   * Start `BARRIER_REASON_TIMEOUT = 30 minutes`.

4. Wait for user reply or timeout.

**Possible Inputs / Next Transitions:**

* **If user replies with “1,” “2,” “3,” or “4”** (or any free reply if they choose “Other”):

  * Record `barrierReasonResponse = [selected option or free text]`.
  * Transition directly to **`END_OF_DAY`**.

* **If `BARRIER_REASON_TIMEOUT` expires** (no reply within 30 min):

  * Leave `barrierReasonResponse = null`.
  * Transition to **`END_OF_DAY`**.

*(No further questions are asked once we collect a barrier reason or time out.)*

---

### 12. Ignored Path (No “Did You Get a Chance?” Reply)

**State Name:** `IGNORED_PATH`
**Entry Condition:**

* `gotChanceResponse = “no_reply”` from `DID_YOU_GET_A_CHANCE`.

**Action When Entered:**

1. Send a two‐part message:

   1. **“What kept you from doing it today? Reply with one word, a quick audio, or a short video!”**
   2. **“Building awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day.”**

2. Mark `ignoredReminderSent = true`.

3. No timers needed here; after sending those two lines, immediately transition to **`END_OF_DAY`**.

*(There are no further questions for someone who never responded to “Did you get a chance?”)*

---

### 13. End‐of‐Day State

**State Name:** `END_OF_DAY`
**Entry Condition:**

* Reached from any of these preceding states:

  * `REINFORCEMENT_FOLLOWUP`
  * `BARRIER_CHECK_AFTER_CONTEXT_MOOD`
  * `BARRIER_REASON_NO_CHANCE`
  * `IGNORED_PATH`
  * `CONTEXT_QUESTION` timed out
  * `MOOD_QUESTION` timed out

**Action When Entered:**

1. Mark `dayFlowCompleted = true`.
2. No further messages are sent until either:

   * The **daily scheduler** re‐fires at 00:00 AM local or the chosen “prompt hour” tomorrow → it will next run `COMMITMENT_PROMPT` again.
   * Or the **weekly summary scheduler** fires (if 7 days have elapsed since enrollment or last weekly summary).
   * Or the participant types “Ready” (which will override and immediately trigger the next day’s prompts).

**Note:**

* If the participant sends an out‐of‐band message (anything that does not match any of these recognized inputs) once they’re in `END_OF_DAY`, ignore or optionally reply with a generic “We’re all set for today; see you tomorrow!”

---

### 14. Weekly Summary State

**State Name:** `WEEKLY_SUMMARY`
**Entry Condition:**

* It has been exactly **7 days** since the last time we sent a weekly summary (or since enrollment, for the first one).
* Alternatively, your scheduler can check at midnight each day: “Has it been exactly 7 days since `weekStartDate`? If yes, fire weekly summary.”

**Action When Entered:**

1. Compute:

   * `timesCompletedThisWeek =` the count of all days in the past seven that had `completionResponse = “done”`.

2. Send:

   > **“Great job this week! 🎉 You completed your habit `\[timesCompletedThisWeek\]` times in the past 7 days! 🙌 Keep up the momentum—small actions add up!”**

3. Reset:

   * `timesCompletedThisWeek = 0`
   * `weekStartDate = today` (so the next summary occurs seven days from now).

4. Transition back to **`END_OF_DAY`** (await tomorrow’s daily scheduler).

---

## Putting It All in Sequence

Below is a bullet‐point view of how a participant’s day might unfold. Each time you see a label in all caps (like `FEELING_PROMPT`), that refers to one of the states above.

1. **Daily Scheduler triggers `COMMITMENT_PROMPT` at 10 AM local time.**

   1. If the participant types **“Ready”** earlier (after previous day is done), cancel the scheduled 10 AM send and immediately run `COMMITMENT_PROMPT`.

2. **State = `COMMITMENT_PROMPT`.**

   * Sent “You committed to trying … 1=Yes | 2=Not yet.”
   * If no reply by 10 PM or “2” arrives → end for today.
   * If “1” arrives → go to `FEELING_PROMPT`.

3. **State = `FEELING_PROMPT`.**

   * Sent “How do you feel? 1–5.”
   * If any “1–5” arrives → go to `RANDOM_ASSIGNMENT`.
   * If “Ready” arrives → set feelingResponse = on\_demand → go to `RANDOM_ASSIGNMENT`.
   * If 15 minutes elapse → feelingResponse = timed\_out → go to `RANDOM_ASSIGNMENT`.

4. **State = `RANDOM_ASSIGNMENT`.**

   * Generate `flowAssignmentToday` = “IMMEDIATE” or “REFLECTIVE” at random (50/50).
   * If “IMMEDIATE” → go to `SEND_INTERVENTION_IMMEDIATE`.
   * If “REFLECTIVE” → go to `SEND_INTERVENTION_REFLECTIVE`.

5. **State = `SEND_INTERVENTION_IMMEDIATE` or `SEND_INTERVENTION_REFLECTIVE`.**

   * Send either the immediate‐action text or the reflection‐first text (whichever branch).
   * Start a 30 min `COMPLETION_TIMEOUT`.
   * Wait for “Done” or “No,” else time out.
   * If “Done” → go to `REINFORCEMENT_FOLLOWUP`.
   * If “No” or timeout → go to `DID_YOU_GET_A_CHANCE`.

6. **State = `REINFORCEMENT_FOLLOWUP`** (if user replied “Done”).

   * Send “Great job!”
   * Increment `timesCompletedToday`.
   * Go to `END_OF_DAY`.

7. **State = `DID_YOU_GET_A_CHANCE`** (if user said “No” or never replied to the intervention).

   * Send “Did you get a chance to try it? (Yes/No)”
   * Start a 15 min `GOT_CHANCE_TIMEOUT`.
   * If “Yes” arrives → go to `CONTEXT_QUESTION`.
   * If “No” arrives → go to `BARRIER_REASON_NO_CHANCE`.
   * If 15 min expire → go to `IGNORED_PATH`.

8. **State = `CONTEXT_QUESTION`** (if they said “Yes, I got a chance”).

   * Send “You did it! What was happening around you? 1–4.”
   * Start a 15 min `CONTEXT_TIMEOUT`.
   * If user picks 1–4 → go to `MOOD_QUESTION`.
   * If timeout → go to `END_OF_DAY`.

9. **State = `MOOD_QUESTION`** (if they answered 1–4).

   * Send “What best describes your mood? Relaxed / Neutral / Stressed.”
   * Start a 15 min `MOOD_TIMEOUT`.
   * If user replies → go to `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.
   * If timeout → go to `END_OF_DAY`.

10. **State = `BARRIER_CHECK_AFTER_CONTEXT_MOOD`** (if they answered mood).

    * Send “Did something make this easier or harder today? What was it?” (free text).
    * Start a 30 min `BARRIER_DETAIL_TIMEOUT`.
    * If user replies before timeout → go to `END_OF_DAY`.
    * If timeout → go to `END_OF_DAY`.

11. **State = `BARRIER_REASON_NO_CHANCE`** (if they said “No, I didn’t get a chance”).

    * Send “Why couldn’t you do it? 1–4 (or free text).”
    * Start a 30 min `BARRIER_REASON_TIMEOUT`.
    * If user replies → go to `END_OF_DAY`.
    * If timeout → go to `END_OF_DAY`.

12. **State = `IGNORED_PATH`** (if they never replied “Yes”/“No” to “Did you get a chance?”).

    * Send two messages:

      1. “What kept you from doing it today? Reply with one word or audio/video.”
      2. “Building awareness takes time! Try watching the video again or set a small goal for tonight.”
    * Immediately go to `END_OF_DAY`.

13. **State = `END_OF_DAY`.**

    * Do nothing until tomorrow’s scheduler (or “Ready” override).

14. **Weekly Summary Scheduler** (runs daily at midnight, for example):

    * If `today - weekStartDate ≥ 7 days`:

      * Compute `timesCompletedThisWeek = sum of daily done’s`.
      * Send “Great job this week! You completed your habit `[timesCompletedThisWeek]` times in the past 7 days.”
      * Reset `timesCompletedThisWeek = 0`, set `weekStartDate = today`.
      * Return to `END_OF_DAY`.

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
   * `barrierReasonResponse` (integer 1–4 or free‐text string or null)
   * `barrierDetailResponse` (free‐text or null)

2. **Counters and Timestamps:**

   * `lastCommitmentDate` (date)
   * `timesCompletedToday` (integer, either 0 or 1)
   * `timesCompletedThisWeek` (integer)
   * `weekStartDate` (date)

3. **Timers:**

   * `COMMITMENT_TIMEOUT` (optional, e.g. same‐day 12 hr)
   * `FEELING_TIMEOUT` (e.g. 15 min)
   * `COMPLETION_TIMEOUT` (30 min)
   * `GOT_CHANCE_TIMEOUT` (15 min)
   * `CONTEXT_TIMEOUT` (15 min)
   * `MOOD_TIMEOUT` (15 min)
   * `BARRIER_DETAIL_TIMEOUT` (30 min)
   * `BARRIER_REASON_TIMEOUT` (30 min)

Timers can be implemented as expirations in a job queue or as “expires\_at” UTC timestamps stored in the participant record, plus a background worker that scans for expired timers every minute.

---

## Example “If/Else” Logic in Pseudodescriptive Form

Below is an abridged, step‐by‐step “if/else”-style description for one day. You will see how each state leads to the next:

1. **(Scheduler fires at 10:00 AM or user types “Ready”)**

   * If `hasSeenOrientation = false`, send `ORIENTATION`, set `hasSeenOrientation = true`. Skip directly to end‐of‐day (no other prompts today).
   * Else (hasSeenOrientation = true) → `COMMITMENT_PROMPT`.

2. **`COMMITMENT_PROMPT`:**

   * Send “Ready?” message.
   * Wait for “1” or “2” or `COMMITMENT_TIMEOUT`.
   * If “1” → `hasRespondedToCommitment = true` → go to `FEELING_PROMPT`.
   * Else (user typed “2” or timed out) → `hasRespondedToCommitment = false` → go to `END_OF_DAY`.

3. **`FEELING_PROMPT`:**

   * Send emotion question.
   * Wait for “1–5” or “Ready” or `FEELING_TIMEOUT`.
   * If “1–5” or “Ready” → set `feelingResponse` appropriately → go to `RANDOM_ASSIGNMENT`.
   * If timeout → set `feelingResponse = “timed_out”` → go to `RANDOM_ASSIGNMENT`.

4. **`RANDOM_ASSIGNMENT`:**

   * Flip a coin. If heads → `flowAssignmentToday = "IMMEDIATE"` → go to `SEND_INTERVENTION_IMMEDIATE`.
   * If tails → `flowAssignmentToday = "REFLECTIVE"` → go to `SEND_INTERVENTION_REFLECTIVE`.

5. **`SEND_INTERVENTION_IMMEDIATE` or `SEND_INTERVENTION_REFLECTIVE`:**

   * Send the appropriate text (action vs. reflection).
   * Wait `COMPLETION_TIMEOUT` for “Done” or “No.”
   * If “Done” → go to `REINFORCEMENT_FOLLOWUP`.
   * If “No” or timeout → go to `DID_YOU_GET_A_CHANCE`.

6. **`REINFORCEMENT_FOLLOWUP`:**

   * Send “Great job!”
   * Increment `timesCompletedToday` (for weekly summary).
   * Go to `END_OF_DAY`.

7. **`DID_YOU_GET_A_CHANCE`:**

   * Send “Did you get a chance?”
   * Wait 15 min (`GOT_CHANCE_TIMEOUT`) for “Yes” or “No.”
   * If “Yes” → go to `CONTEXT_QUESTION`.
   * If “No” → go to `BARRIER_REASON_NO_CHANCE`.
   * If timeout → go to `IGNORED_PATH`.

8. **`CONTEXT_QUESTION`:**

   * Send “What was happening around you? 1–4.”
   * Wait 15 min.
   * If user picks 1–4 → go to `MOOD_QUESTION`.
   * Else (timeout or invalid) → go to `END_OF_DAY`.

9. **`MOOD_QUESTION`:**

   * Send “What best describes your mood? Relaxed/Neutral/Stressed.”
   * Wait 15 min.
   * If user replies correctly → go to `BARRIER_CHECK_AFTER_CONTEXT_MOOD`.
   * Else → go to `END_OF_DAY`.

10. **`BARRIER_CHECK_AFTER_CONTEXT_MOOD`:**

    * Send free‐text “Did something make it easier/harder?”
    * Wait 30 min.
    * After any reply or timeout → go to `END_OF_DAY`.

11. **`BARRIER_REASON_NO_CHANCE`:**

    * Send “Why couldn’t you do it? 1–4 or other.”
    * Wait 30 min.
    * After any reply or timeout → go to `END_OF_DAY`.

12. **`IGNORED_PATH`:**

    * Send “What kept you from doing it? … building awareness takes time.”
    * Go to `END_OF_DAY`.

13. **`END_OF_DAY`:**

    * Wait until next day’s `COMMITMENT_PROMPT` or “Ready” override, or until the weekly summary triggers.

14. **Weekly Summary (background job runs each midnight):**

    * If `today - weekStartDate ≥ 7 days`, compute `timesCompletedThisWeek`, send summary message, reset counters, set `weekStartDate = today`.
    * Return to `END_OF_DAY`.

---

## Key Implementation Details

1. **“On‐Demand” Override (“Ready”):**

   * At any time *after* a previous day’s flow has ended, if the system sees an incoming text exactly equal to “Ready” (case‐insensitive), forcibly start that participant’s `COMMITMENT_PROMPT` state immediately—regardless of the daily scheduled time.
   * Once you do that, cancel any previously scheduled daily prompt for that participant (to avoid duplicating).

2. **Timers and Cancellations:**

   * If a participant replies early (e.g. sends “1” to skip the rest of the commitment wait), you must cancel any outstanding timers (e.g. the 12 hr `COMMITMENT_TIMEOUT` or 15 min `FEELING_TIMEOUT`).
   * Implement timers either as “expires\_at” fields plus a polling worker or as a true job in a task queue that calls back to your code when the timeout is hit.

3. **Persistent Data Store:**

   * Each participant’s conversation must be stored in a database table keyed by `participantId` (and perhaps by `date`).
   * Every message you send should be logged (with a timestamp) along with any user reply you receive, plus the time you recorded it. This enables you to compute the weekly summary.\`

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

Below is how all the states link to each other. You can use this as a “roadmap” when writing your code.

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

This specification enumerates every state, every timer, and every conditional transition you need to implement the exact same logic as in Figures 1 and 2 from the WhatsApp study. A developer can now map each “state” to a function or method in their code, wire up timers or job‐queue events for each timeout, and route incoming WhatsApp messages or button clicks into these state machines. From there, you have enough information to send precisely the right prompts, collect exactly the right numerical or free‐text responses, randomize appropriately, and generate a weekly summary.
