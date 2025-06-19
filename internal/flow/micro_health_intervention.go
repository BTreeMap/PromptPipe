package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// canonicalizeResponse standardizes user responses by trimming whitespace and converting to lowercase.
// This ensures consistent processing of user input across all intervention flows.
func canonicalizeResponse(response string) string {
	return strings.ToLower(strings.TrimSpace(response))
}

// Simple message constants for micro health intervention flow.
const (
	MsgOrientation      = "Hi! 🌱 Welcome to our Healthy Habits study!\nHere's how it works: You will receive messages on a schedule, or type 'Ready' anytime to get a prompt. Your input is important."
	MsgRandomAssignment = "Based on your profile, we're assigning you to a personalized track. Please wait for your next message."
	MsgComplete         = "🎉 Congratulations! You've completed the micro health intervention. Thank you for participating!"

	// Other intervention messages
	MsgBarrierDetail           = "Did something make this easier or harder today? What was it?"
	MsgImmediateIntervention   = "Great! Right now, stand up and do three gentle shoulder rolls, then take three slow, full breaths. When you're done, reply 'Done.'"
	MsgReflectiveIntervention  = "Before you begin, pause for a moment: When was the last time you noticed your posture? Take 30 seconds to think about where your shoulders are right now. After that, stand up and do a gentle shoulder roll—then reply 'Done.'"
	MsgReinforcement           = "Great job! 🎉 You just completed your habit in under one minute—keep it up!"
	MsgIgnoredPath             = "What kept you from doing it today? Reply with one word, a quick audio, or a short video!\n\nBuilding awareness takes time! Try watching the video again or setting a small goal to reflect on this habit at the end of the day."
	MsgEndOfDay                = "That's okay! We'll check back with you tomorrow. Take care! 🌙"
	MsgInvalidCommitmentChoice = "Please reply with '1' for 'Let's do it!' or '2' for 'Not yet'."
	MsgInvalidFeelingChoice    = "Please reply with '1' through '5' to indicate your feeling."
	MsgInvalidDidYouGetAChance = "Please reply with '1' for 'Yes' or '2' for 'No'."
	MsgInvalidContextChoice    = "Please reply with '1' through '4' to describe your context."
	MsgInvalidMoodChoice       = "Please reply with '1' through '3' to describe your mood."
	MsgInvalidBarrierChoice    = "Please reply with '1' through '4' to indicate the barrier reason."
)

// Timeout duration constants based on micro health intervention documentation
const (
	CommitmentTimeout       = 12 * time.Hour   // COMMITMENT_PROMPT timeout
	FeelingTimeout          = 15 * time.Minute // FEELING_PROMPT timeout
	CompletionTimeout       = 30 * time.Minute // SEND_INTERVENTION_* timeout
	DidYouGetAChanceTimeout = 15 * time.Minute // DID_YOU_GET_A_CHANCE timeout
	ContextTimeout          = 15 * time.Minute // CONTEXT_QUESTION timeout
	MoodTimeout             = 15 * time.Minute // MOOD_QUESTION timeout
	BarrierCheckTimeout     = 15 * time.Minute // BARRIER_CHECK_AFTER_CONTEXT_MOOD timeout
	BarrierReasonTimeout    = 15 * time.Minute // BARRIER_REASON_NO_CHANCE timeout
)

// Pre-generated message strings from structured data - initialized in init()
var (
	MsgCommitment       string
	MsgFeeling          string
	MsgHabitReminder    string
	MsgFollowUp         string
	MsgDidYouGetAChance string
	MsgContext          string
	MsgMood             string
	MsgBarrierReason    string
)

// Global structured branch messages - initialized in init()
var (
	CommitmentMessage       *models.Branch
	FeelingMessage          *models.Branch
	HabitReminderMessage    *models.Branch
	FollowUpMessage         *models.Branch
	DidYouGetAChanceMessage *models.Branch
	ContextMessage          *models.Branch
	MoodMessage             *models.Branch
	BarrierReasonMessage    *models.Branch
)

// init initializes all structured branch messages and generates string versions
func init() {
	CommitmentMessage = &models.Branch{
		Body: "You committed to trying a quick habit today—ready to go?",
		Options: []models.BranchOption{
			{Label: "🚀 Let's do it!", Body: "Continue"},
			{Label: "⏳ Not yet", Body: "Let's try again tomorrow"},
		},
	}

	FeelingMessage = &models.Branch{
		Body: "How do you feel about this first step?",
		Options: []models.BranchOption{
			{Label: "😊 Excited", Body: "Great energy!"},
			{Label: "🤔 Curious", Body: "Perfect mindset!"},
			{Label: "😃 Motivated", Body: "Let's channel that motivation!"},
			{Label: "📖 Need info", Body: "We'll guide you through it!"},
			{Label: "⚖️ Not sure", Body: "That's completely normal!"},
		},
	}

	HabitReminderMessage = &models.Branch{
		Body: "⏰ Reminder: It's time for your healthy habit! How did it go?",
		Options: []models.BranchOption{
			{Label: "✅ Completed", Body: "Excellent work!"},
			{Label: "⏳ Will do later", Body: "We'll check back with you!"},
			{Label: "❌ Skipped today", Body: "No worries, tomorrow is a new day!"},
		},
	}

	FollowUpMessage = &models.Branch{
		Body: "Great progress! 📈 How are you feeling about your habit journey?",
		Options: []models.BranchOption{
			{Label: "😊 Going well", Body: "Keep up the great work!"},
			{Label: "🤔 Mixed feelings", Body: "That's normal - progress isn't always linear!"},
			{Label: "😓 Struggling", Body: "We're here to support you!"},
		},
	}

	DidYouGetAChanceMessage = &models.Branch{
		Body: "Did you get a chance to try it?",
		Options: []models.BranchOption{
			{Label: "Yes", Body: "Great! Let's explore more."},
			{Label: "No", Body: "Let's understand what happened."},
		},
	}

	ContextMessage = &models.Branch{
		Body: "You did it! What was happening around you?",
		Options: []models.BranchOption{
			{Label: "Alone & focused", Body: "Perfect environment!"},
			{Label: "With others around", Body: "Great despite distractions!"},
			{Label: "In a distracting place", Body: "Impressive focus!"},
			{Label: "Busy & stressed", Body: "Amazing that you made time!"},
		},
	}

	MoodMessage = &models.Branch{
		Body: "What best describes your mood before doing this?",
		Options: []models.BranchOption{
			{Label: "🙂 Relaxed", Body: "Perfect state for building habits!"},
			{Label: "😐 Neutral", Body: "A calm approach works well!"},
			{Label: "😫 Stressed", Body: "Great that you prioritized self-care!"},
		},
	}

	BarrierReasonMessage = &models.Branch{
		Body: "Could you let me know why you couldn't do it this time?",
		Options: []models.BranchOption{
			{Label: "I didn't have enough time", Body: "Time management is key - let's work on that!"},
			{Label: "I didn't understand the task", Body: "Let's clarify the instructions!"},
			{Label: "I didn't feel motivated to do it", Body: "Motivation fluctuates - that's normal!"},
			{Label: "Other (please specify)", Body: "Please share more details."},
		},
	}

	// Generate string versions from structured data
	MsgCommitment = CommitmentMessage.Generate()
	MsgFeeling = FeelingMessage.Generate()
	MsgHabitReminder = HabitReminderMessage.Generate()
	MsgFollowUp = FollowUpMessage.Generate()
	MsgDidYouGetAChance = DidYouGetAChanceMessage.Generate()
	MsgContext = ContextMessage.Generate()
	MsgMood = MoodMessage.Generate()
	MsgBarrierReason = BarrierReasonMessage.Generate()
}

// MicroHealthInterventionGenerator implements a custom, stateful micro health intervention flow.
type MicroHealthInterventionGenerator struct {
	stateManager StateManager
	timer        models.Timer
}

// NewMicroHealthInterventionGenerator creates a new generator with dependencies.
func NewMicroHealthInterventionGenerator(stateManager StateManager, timer models.Timer) *MicroHealthInterventionGenerator {
	slog.Debug("Creating MicroHealthInterventionGenerator with dependencies")
	return &MicroHealthInterventionGenerator{
		stateManager: stateManager,
		timer:        timer,
	}
}

// SetDependencies injects dependencies into the generator.
func (g *MicroHealthInterventionGenerator) SetDependencies(deps Dependencies) {
	slog.Debug("MicroHealthInterventionGenerator SetDependencies called")
	g.stateManager = deps.StateManager
	g.timer = deps.Timer
}

// Generate selects the next message based on the current state in p.State.
func (g *MicroHealthInterventionGenerator) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("MicroHealthIntervention Generate invoked", "state", p.State, "to", p.To)

	// For simple message generation, dependencies are not required
	// Dependencies are only needed for stateful operations like state transitions and timers
	switch p.State {
	case "", models.StateOrientation:
		slog.Debug("MicroHealthIntervention state orientation", "to", p.To)
		return MsgOrientation, nil
	case models.StateCommitmentPrompt:
		slog.Debug("MicroHealthIntervention state commitment prompt", "to", p.To)
		return MsgCommitment, nil
	case models.StateFeelingPrompt:
		slog.Debug("MicroHealthIntervention state feeling prompt", "to", p.To)
		return MsgFeeling, nil
	case models.StateRandomAssignment:
		slog.Debug("MicroHealthIntervention state random assignment", "to", p.To)
		return MsgRandomAssignment, nil
	case models.StateSendInterventionImmediate:
		slog.Debug("MicroHealthIntervention state send intervention immediate", "to", p.To)
		return MsgImmediateIntervention, nil
	case models.StateSendInterventionReflective:
		slog.Debug("MicroHealthIntervention state send intervention reflective", "to", p.To)
		return MsgReflectiveIntervention, nil
	case models.StateReinforcementFollowup:
		slog.Debug("MicroHealthIntervention state reinforcement followup", "to", p.To)
		return MsgReinforcement, nil
	case models.StateDidYouGetAChance:
		slog.Debug("MicroHealthIntervention state did you get a chance", "to", p.To)
		return MsgDidYouGetAChance, nil
	case models.StateContextQuestion:
		slog.Debug("MicroHealthIntervention state context question", "to", p.To)
		return MsgContext, nil
	case models.StateMoodQuestion:
		slog.Debug("MicroHealthIntervention state mood question", "to", p.To)
		return MsgMood, nil
	case models.StateBarrierCheckAfterContextMood:
		slog.Debug("MicroHealthIntervention state barrier check", "to", p.To)
		return MsgBarrierDetail, nil
	case models.StateBarrierReasonNoChance:
		slog.Debug("MicroHealthIntervention state barrier reason", "to", p.To)
		return MsgBarrierReason, nil
	case models.StateIgnoredPath:
		slog.Debug("MicroHealthIntervention state ignored path", "to", p.To)
		return MsgIgnoredPath, nil
	case models.StateEndOfDay:
		slog.Debug("MicroHealthIntervention state end of day", "to", p.To)
		return MsgEndOfDay, nil
	case models.StateHabitReminder:
		slog.Debug("MicroHealthIntervention state habit reminder", "to", p.To)
		return MsgHabitReminder, nil
	case models.StateFollowUp:
		slog.Debug("MicroHealthIntervention state follow up", "to", p.To)
		return MsgFollowUp, nil
	case models.StateComplete:
		slog.Debug("MicroHealthIntervention state complete", "to", p.To)
		return MsgComplete, nil
	default:
		slog.Error("MicroHealthIntervention unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf("unsupported micro health intervention state '%s'", p.State)
	}
}

// ProcessResponse handles participant responses and manages state transitions.
// This method requires dependencies to be properly initialized.
func (g *MicroHealthInterventionGenerator) ProcessResponse(ctx context.Context, participantID, response string) error {
	// Validate dependencies for stateful operations
	if g.stateManager == nil || g.timer == nil {
		slog.Error("MicroHealthIntervention dependencies not initialized for state operations")
		return fmt.Errorf("generator dependencies not properly initialized for state operations")
	}

	// Get current state
	currentState, err := g.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		slog.Error("MicroHealthIntervention ProcessResponse: failed to get current state", "error", err, "participantID", participantID)
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// If no state exists, this might be the first interaction
	if currentState == "" {
		currentState = models.StateOrientation
	}

	slog.Debug("MicroHealthIntervention ProcessResponse", "participantID", participantID, "response", response, "currentState", currentState)

	// Handle "Ready" override - can be sent at any time to trigger immediate intervention
	if canonicalizeResponse(response) == "ready" && currentState == models.StateEndOfDay {
		return g.transitionToState(ctx, participantID, models.StateCommitmentPrompt)
	}

	// Process response based on current state
	switch currentState {
	case models.StateOrientation:
		// After orientation, move to commitment prompt
		return g.transitionToState(ctx, participantID, models.StateCommitmentPrompt)

	case models.StateCommitmentPrompt:
		return g.processCommitmentResponse(ctx, participantID, response)

	case models.StateFeelingPrompt:
		return g.processFeelingResponse(ctx, participantID, response)

	case models.StateSendInterventionImmediate, models.StateSendInterventionReflective:
		return g.processInterventionResponse(ctx, participantID, response)

	case models.StateDidYouGetAChance:
		return g.processDidYouGetAChanceResponse(ctx, participantID, response)

	case models.StateContextQuestion:
		return g.processContextResponse(ctx, participantID, response)

	case models.StateMoodQuestion:
		return g.processMoodResponse(ctx, participantID, response)

	case models.StateBarrierCheckAfterContextMood:
		return g.processBarrierDetailResponse(ctx, participantID, response)

	case models.StateBarrierReasonNoChance:
		return g.processBarrierReasonResponse(ctx, participantID, response)

	case models.StateEndOfDay:
		// Ignore most responses when day is complete, except "Ready"
		if canonicalizeResponse(response) == "ready" {
			return g.transitionToState(ctx, participantID, models.StateCommitmentPrompt)
		}
		slog.Debug("MicroHealthIntervention ignoring response in END_OF_DAY state", "participantID", participantID, "response", response)
		return nil

	default:
		slog.Warn("MicroHealthIntervention ProcessResponse: unhandled state", "state", currentState, "participantID", participantID)
		return fmt.Errorf("unhandled state: %s", currentState)
	}
}

// transitionToState safely transitions to a new state with logging
func (g *MicroHealthInterventionGenerator) transitionToState(ctx context.Context, participantID string, newState models.StateType) error {
	err := g.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, newState)
	if err != nil {
		slog.Error("MicroHealthIntervention failed to transition state", "error", err, "participantID", participantID, "newState", newState)
		return fmt.Errorf("failed to transition to state %s: %w", newState, err)
	}
	slog.Info("MicroHealthIntervention state transition", "participantID", participantID, "newState", newState)
	return nil
}

// processCommitmentResponse handles responses to the commitment prompt
func (g *MicroHealthInterventionGenerator) processCommitmentResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel any existing commitment timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCommitmentTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	switch response {
	case "1", "🚀 let's do it!":
		// Store positive response and move to feeling prompt
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCommitmentTimerID, "")
		if err != nil {
			return err
		}

		// Schedule feeling prompt timeout
		timerID, err := g.timer.ScheduleAfter(FeelingTimeout, func() {
			g.handleFeelingTimeout(ctx, participantID)
		})
		if err == nil {
			g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingTimerID, timerID)
		}

		return g.transitionToState(ctx, participantID, models.StateFeelingPrompt)

	case "2", "⏳ not yet":
		// Store negative response and end day
		return g.transitionToState(ctx, participantID, models.StateEndOfDay)

	default:
		// Invalid response - could send error message or ignore
		slog.Warn("MicroHealthIntervention invalid commitment response", "participantID", participantID, "response", response)
		return nil // Don't transition state, wait for valid response
	}
}

// processFeelingResponse handles responses to the feeling prompt
func (g *MicroHealthInterventionGenerator) processFeelingResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel feeling timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	// Handle "Ready" override
	if response == "ready" {
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse, "on_demand")
		if err != nil {
			return err
		}
		return g.processRandomAssignment(ctx, participantID)
	}

	// Validate feeling response (1-5)
	if response >= "1" && response <= "5" {
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse, response)
		if err != nil {
			return err
		}
		return g.processRandomAssignment(ctx, participantID)
	}

	slog.Warn("MicroHealthIntervention invalid feeling response", "participantID", participantID, "response", response)
	return nil // Don't transition, wait for valid response
}

// processRandomAssignment performs random assignment and transitions to appropriate intervention
func (g *MicroHealthInterventionGenerator) processRandomAssignment(ctx context.Context, participantID string) error {
	// Random assignment: 50/50 chance for immediate vs reflective
	var assignment models.FlowAssignment
	if time.Now().UnixNano()%2 == 0 {
		assignment = models.FlowAssignmentImmediate
	} else {
		assignment = models.FlowAssignmentReflective
	}

	// Store assignment
	err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFlowAssignment, string(assignment))
	if err != nil {
		return err
	}

	// Schedule completion timeout
	timerID, err := g.timer.ScheduleAfter(CompletionTimeout, func() {
		g.handleCompletionTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionTimerID, timerID)
	}

	// Transition to appropriate intervention state
	if assignment == models.FlowAssignmentImmediate {
		return g.transitionToState(ctx, participantID, models.StateSendInterventionImmediate)
	}
	return g.transitionToState(ctx, participantID, models.StateSendInterventionReflective)
}

// processInterventionResponse handles responses to intervention prompts
func (g *MicroHealthInterventionGenerator) processInterventionResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel completion timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	switch response {
	case "done":
		// Successful completion - store response and move to reinforcement
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionResponse, string(models.ResponseDone))
		if err != nil {
			return err
		}
		return g.transitionToState(ctx, participantID, models.StateReinforcementFollowup)

	case "no":
		// Did not complete - store response and move to "did you get a chance"
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionResponse, string(models.ResponseNo))
		if err != nil {
			return err
		}
		return g.scheduleDidYouGetAChance(ctx, participantID)

	default:
		slog.Warn("MicroHealthIntervention invalid intervention response", "participantID", participantID, "response", response)
		return nil // Don't transition, wait for "done" or "no"
	}
}

// scheduleDidYouGetAChance transitions to did you get a chance state with timeout
func (g *MicroHealthInterventionGenerator) scheduleDidYouGetAChance(ctx context.Context, participantID string) error {
	// Schedule timeout for "did you get a chance" question
	timerID, err := g.timer.ScheduleAfter(DidYouGetAChanceTimeout, func() {
		g.handleDidYouGetAChanceTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyDidYouGetAChanceTimerID, timerID)
	}

	return g.transitionToState(ctx, participantID, models.StateDidYouGetAChance)
}

// processDidYouGetAChanceResponse handles responses to "did you get a chance" question
func (g *MicroHealthInterventionGenerator) processDidYouGetAChanceResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyDidYouGetAChanceTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	switch response {
	case "1", "yes":
		// They did try - store response and move to context question
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyGotChanceResponse, "true")
		if err != nil {
			return err
		}
		return g.scheduleContextQuestion(ctx, participantID)

	case "2", "no":
		// They didn't try - store response and move to barrier reason
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyGotChanceResponse, "false")
		if err != nil {
			return err
		}
		return g.scheduleBarrierReason(ctx, participantID)

	default:
		slog.Warn("MicroHealthIntervention invalid did you get a chance response", "participantID", participantID, "response", response)
		return nil
	}
}

// scheduleContextQuestion transitions to context question with timeout
func (g *MicroHealthInterventionGenerator) scheduleContextQuestion(ctx context.Context, participantID string) error {
	timerID, err := g.timer.ScheduleAfter(ContextTimeout, func() {
		g.handleContextTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyContextTimerID, timerID)
	}

	return g.transitionToState(ctx, participantID, models.StateContextQuestion)
}

// processContextResponse handles responses to context question
func (g *MicroHealthInterventionGenerator) processContextResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyContextTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	if response >= "1" && response <= "4" {
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyContextResponse, response)
		if err != nil {
			return err
		}
		return g.scheduleMoodQuestion(ctx, participantID)
	}

	slog.Warn("MicroHealthIntervention invalid context response", "participantID", participantID, "response", response)
	return nil
}

// scheduleMoodQuestion transitions to mood question with timeout
func (g *MicroHealthInterventionGenerator) scheduleMoodQuestion(ctx context.Context, participantID string) error {
	timerID, err := g.timer.ScheduleAfter(MoodTimeout, func() {
		g.handleMoodTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyMoodTimerID, timerID)
	}

	return g.transitionToState(ctx, participantID, models.StateMoodQuestion)
}

// processMoodResponse handles responses to mood question
func (g *MicroHealthInterventionGenerator) processMoodResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyMoodTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	if response >= "1" && response <= "3" {
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyMoodResponse, response)
		if err != nil {
			return err
		}
		return g.scheduleBarrierCheck(ctx, participantID)
	}

	slog.Warn("MicroHealthIntervention invalid mood response", "participantID", participantID, "response", response)
	return nil
}

// scheduleBarrierCheck transitions to barrier check with timeout
func (g *MicroHealthInterventionGenerator) scheduleBarrierCheck(ctx context.Context, participantID string) error {
	timerID, err := g.timer.ScheduleAfter(BarrierCheckTimeout, func() {
		g.handleBarrierCheckTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierCheckTimerID, timerID)
	}

	return g.transitionToState(ctx, participantID, models.StateBarrierCheckAfterContextMood)
}

// processBarrierDetailResponse handles free-text responses to barrier detail question
func (g *MicroHealthInterventionGenerator) processBarrierDetailResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierCheckTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	// Store any response (free text)
	err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierResponse, response)
	if err != nil {
		return err
	}

	return g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

// scheduleBarrierReason transitions to barrier reason question with timeout
func (g *MicroHealthInterventionGenerator) scheduleBarrierReason(ctx context.Context, participantID string) error {
	timerID, err := g.timer.ScheduleAfter(BarrierReasonTimeout, func() {
		g.handleBarrierReasonTimeout(ctx, participantID)
	})
	if err == nil {
		g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonTimerID, timerID)
	}

	return g.transitionToState(ctx, participantID, models.StateBarrierReasonNoChance)
}

// processBarrierReasonResponse handles responses to barrier reason question
func (g *MicroHealthInterventionGenerator) processBarrierReasonResponse(ctx context.Context, participantID, response string) error {
	response = canonicalizeResponse(response)

	// Cancel timer
	if timerID, err := g.stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonTimerID); err == nil && timerID != "" {
		g.timer.Cancel(timerID)
	}

	if response >= "1" && response <= "4" {
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonResponse, response)
		if err != nil {
			return err
		}
	} else {
		// Store any free text response
		err := g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonResponse, response)
		if err != nil {
			return err
		}
	}

	return g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

// Timeout handlers

func (g *MicroHealthInterventionGenerator) handleFeelingTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention feeling timeout", "participantID", participantID)
	g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse, "timed_out")
	g.processRandomAssignment(ctx, participantID)
}

func (g *MicroHealthInterventionGenerator) handleCompletionTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention completion timeout", "participantID", participantID)
	g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionResponse, string(models.ResponseNoReply))
	g.scheduleDidYouGetAChance(ctx, participantID)
}

func (g *MicroHealthInterventionGenerator) handleDidYouGetAChanceTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention did you get a chance timeout", "participantID", participantID)
	g.stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyGotChanceResponse, string(models.ResponseNoReply))
	g.transitionToState(ctx, participantID, models.StateIgnoredPath)
}

func (g *MicroHealthInterventionGenerator) handleContextTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention context timeout", "participantID", participantID)
	g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

func (g *MicroHealthInterventionGenerator) handleMoodTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention mood timeout", "participantID", participantID)
	g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

func (g *MicroHealthInterventionGenerator) handleBarrierCheckTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention barrier check timeout", "participantID", participantID)
	g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

func (g *MicroHealthInterventionGenerator) handleBarrierReasonTimeout(ctx context.Context, participantID string) {
	slog.Info("MicroHealthIntervention barrier reason timeout", "participantID", participantID)
	g.transitionToState(ctx, participantID, models.StateEndOfDay)
}

// Note: Removed unsafe global registration - custom generators should be registered
// manually with proper dependency injection in the application startup code.
