// Package messaging provides response handling functionality for stateful interactions.
package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// ResponseAction defines a hook function that processes a participant's response.
// It receives the participant's canonical phone number, response text, and timestamp.
// It should return true if the response was handled, false otherwise.
type ResponseAction func(ctx context.Context, from, responseText string, timestamp int64) (handled bool, err error)

// ResponseHandler manages stateful response processing by maintaining a map of
// recipient -> action hooks and routing incoming responses appropriately.
type ResponseHandler struct {
	// hooks maps canonicalized phone numbers to response action functions
	hooks map[string]ResponseAction
	// mu protects concurrent access to the hooks map
	mu sync.RWMutex
	// msgService is used to send default responses when no hook is registered
	msgService Service
	// defaultMessage is sent when no hook handles a response
	defaultMessage string
}

// NewResponseHandler creates a new ResponseHandler with the given messaging service.
func NewResponseHandler(msgService Service) *ResponseHandler {
	return &ResponseHandler{
		hooks:          make(map[string]ResponseAction),
		msgService:     msgService,
		defaultMessage: "📝 Your message has been recorded. Thank you for your response!",
	}
}

// RegisterHook registers a response action for a specific participant.
// The recipient should be a canonicalized phone number (e.g., "1234567890").
func (rh *ResponseHandler) RegisterHook(recipient string, action ResponseAction) error {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Error("ResponseHandler RegisterHook validation failed", "error", err, "recipient", recipient)
		return fmt.Errorf("invalid recipient: %w", err)
	}

	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.hooks[canonicalRecipient] = action

	slog.Debug("ResponseHandler hook registered", "recipient", canonicalRecipient)
	return nil
}

// UnregisterHook removes a response action for a specific participant.
func (rh *ResponseHandler) UnregisterHook(recipient string) error {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Error("ResponseHandler UnregisterHook validation failed", "error", err, "recipient", recipient)
		return fmt.Errorf("invalid recipient: %w", err)
	}

	rh.mu.Lock()
	defer rh.mu.Unlock()
	delete(rh.hooks, canonicalRecipient)

	slog.Debug("ResponseHandler hook unregistered", "recipient", canonicalRecipient)
	return nil
}

// IsHookRegistered checks if a hook is registered for the given recipient.
func (rh *ResponseHandler) IsHookRegistered(recipient string) bool {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Debug("ResponseHandler IsHookRegistered validation failed", "error", err, "recipient", recipient)
		return false
	}

	rh.mu.RLock()
	defer rh.mu.RUnlock()
	_, exists := rh.hooks[canonicalRecipient]
	return exists
}

// ProcessResponse processes an incoming response by checking for registered hooks
// and executing them, or sending a default response if no hook is found.
func (rh *ResponseHandler) ProcessResponse(ctx context.Context, response models.Response) error {
	// Validate and canonicalize the sender
	canonicalFrom, err := rh.msgService.ValidateAndCanonicalizeRecipient(response.From)
	if err != nil {
		slog.Error("ResponseHandler ProcessResponse validation failed", "error", err, "from", response.From)
		return fmt.Errorf("invalid sender: %w", err)
	}

	slog.Debug("ResponseHandler processing response", "from", canonicalFrom, "body_length", len(response.Body))

	// Check for registered hook
	rh.mu.RLock()
	action, hasHook := rh.hooks[canonicalFrom]
	rh.mu.RUnlock()

	if hasHook {
		slog.Debug("ResponseHandler executing hook", "from", canonicalFrom)
		handled, err := action(ctx, canonicalFrom, response.Body, response.Time)
		if err != nil {
			slog.Error("ResponseHandler hook execution failed", "error", err, "from", canonicalFrom)
			// Send error response to user
			errorMsg := "⚠️ We encountered an issue processing your response. Please try again or contact support."
			if sendErr := rh.msgService.SendMessage(ctx, canonicalFrom, errorMsg); sendErr != nil {
				slog.Error("ResponseHandler failed to send error message", "error", sendErr, "from", canonicalFrom)
			}
			return fmt.Errorf("hook execution failed: %w", err)
		}

		if handled {
			slog.Info("ResponseHandler response handled by hook", "from", canonicalFrom)
			return nil
		}
		// Hook exists but didn't handle the response, fall through to default
		slog.Debug("ResponseHandler hook did not handle response", "from", canonicalFrom)
	}

	// No hook registered or hook didn't handle the response - send default message
	slog.Debug("ResponseHandler sending default response", "from", canonicalFrom)
	if err := rh.msgService.SendMessage(ctx, canonicalFrom, rh.defaultMessage); err != nil {
		slog.Error("ResponseHandler failed to send default response", "error", err, "from", canonicalFrom)
		return fmt.Errorf("failed to send default response: %w", err)
	}

	slog.Info("ResponseHandler sent default response", "from", canonicalFrom)
	return nil
}

// SetDefaultMessage sets the default message sent when no hook handles a response.
func (rh *ResponseHandler) SetDefaultMessage(message string) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.defaultMessage = message
	slog.Debug("ResponseHandler default message updated", "message", message)
}

// GetDefaultMessage returns the current default message.
func (rh *ResponseHandler) GetDefaultMessage() string {
	rh.mu.RLock()
	defer rh.mu.RUnlock()
	return rh.defaultMessage
}

// GetHookCount returns the number of currently registered hooks.
func (rh *ResponseHandler) GetHookCount() int {
	rh.mu.RLock()
	defer rh.mu.RUnlock()
	return len(rh.hooks)
}

// ListRegisteredRecipients returns a slice of all recipients with registered hooks.
func (rh *ResponseHandler) ListRegisteredRecipients() []string {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	recipients := make([]string, 0, len(rh.hooks))
	for recipient := range rh.hooks {
		recipients = append(recipients, recipient)
	}
	return recipients
}

// Start begins processing responses from the messaging service.
// This should be called once to start the response processing loop.
func (rh *ResponseHandler) Start(ctx context.Context) {
	slog.Info("ResponseHandler starting response processing")

	go func() {
		defer slog.Info("ResponseHandler stopped response processing")

		for {
			select {
			case response, ok := <-rh.msgService.Responses():
				if !ok {
					slog.Debug("ResponseHandler responses channel closed")
					return
				}

				// Process the response
				if err := rh.ProcessResponse(ctx, response); err != nil {
					slog.Error("ResponseHandler failed to process response", "error", err, "from", response.From)
				}

			case <-ctx.Done():
				slog.Debug("ResponseHandler stopping due to context cancellation")
				return
			}
		}
	}()

	slog.Info("ResponseHandler response processing started")
}

// CreateInterventionHook creates a specialized hook for intervention participants that
// processes responses according to the micro health intervention flow logic.
// It includes timer management for timeout-based state transitions.
func CreateInterventionHook(participantID, phoneNumber string, stateManager StateManager, msgService Service, timer models.Timer) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("InterventionHook processing response", "from", from, "responseText", responseText)

		// For simple operations, we use the stored participantID that was passed during hook creation.
		// Only look up the participant from store when we need additional participant data.

		// Get current state using the participantID from hook creation
		currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
		if err != nil {
			slog.Error("InterventionHook failed to get current state", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to get current state: %w", err)
		}

		// Handle "Ready" override at any time (except during active flows)
		responseTextLower := strings.ToLower(strings.TrimSpace(responseText))
		if responseTextLower == string(models.ResponseReady) {
			// Check if we're in END_OF_DAY state or orientation (can accept "Ready")
			if currentState == models.StateOrientation || currentState == models.StateEndOfDay || currentState == "" {
				slog.Info("InterventionHook handling Ready override", "participantID", participantID)

				// Transition to COMMITMENT_PROMPT immediately
				if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateCommitmentPrompt); err != nil {
					slog.Error("InterventionHook failed to transition to COMMITMENT_PROMPT", "error", err, "participantID", participantID)
					return false, fmt.Errorf("failed to transition state: %w", err)
				}

				// Send commitment prompt using global variable
				if err := msgService.SendMessage(ctx, from, flow.MsgCommitment); err != nil {
					slog.Error("InterventionHook failed to send commitment prompt", "error", err, "participantID", participantID)
					return false, fmt.Errorf("failed to send commitment prompt: %w", err)
				}

				return true, nil
			}
		}

		// Handle responses based on current state
		switch currentState {
		case models.StateCommitmentPrompt:
			return handleCommitmentResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateFeelingPrompt:
			return handleFeelingResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateSendInterventionImmediate, models.StateSendInterventionReflective:
			return handleCompletionResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateDidYouGetAChance:
			return handleDidYouGetAChanceResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateContextQuestion:
			return handleContextResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateMoodQuestion:
			return handleMoodResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateBarrierCheckAfterContextMood:
			return handleBarrierResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateBarrierReasonNoChance:
			return handleBarrierReasonResponse(ctx, participantID, from, responseText, stateManager, msgService, timer)
		case models.StateEndOfDay:
			// In END_OF_DAY, only "Ready" is handled (already processed above)
			// Send polite message for other responses
			endMsg := "We're all set for today; we'll be back tomorrow with your daily prompt."
			if err := msgService.SendMessage(ctx, from, endMsg); err != nil {
				slog.Error("InterventionHook failed to send end-of-day message", "error", err, "participantID", participantID)
			}
			return true, nil
		default:
			slog.Debug("InterventionHook no handler for state", "state", currentState, "participantID", participantID)
			return false, nil // Let default handler take over
		}
	}
}

// CreateBranchHook creates a response handler for branch-type prompts that validates
// user selections and responds appropriately.
func CreateBranchHook(branchOptions []models.BranchOption, msgService Service) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("BranchHook processing response", "from", from, "responseText", responseText)

		responseText = strings.TrimSpace(responseText)

		// Try to parse as a number (1, 2, 3, etc.)
		if len(responseText) == 1 && responseText >= "1" && responseText <= "9" {
			optionIndex := int(responseText[0] - '1') // Convert '1' to 0, '2' to 1, etc.

			if optionIndex >= 0 && optionIndex < len(branchOptions) {
				selectedOption := branchOptions[optionIndex]

				// Send confirmation message with the selected option
				confirmationMsg := fmt.Sprintf("✅ You selected: %s\n\n%s",
					selectedOption.Label, selectedOption.Body)

				if err := msgService.SendMessage(ctx, from, confirmationMsg); err != nil {
					slog.Error("BranchHook failed to send confirmation", "error", err, "from", from)
					return false, fmt.Errorf("failed to send confirmation: %w", err)
				}

				slog.Info("BranchHook handled valid selection", "from", from, "selection", optionIndex+1, "label", selectedOption.Label)
				return true, nil
			}
		}

		// Invalid selection - provide guidance
		var optionsText strings.Builder
		optionsText.WriteString("❓ Please select a valid option by replying with the number:\n\n")
		for i, option := range branchOptions {
			optionsText.WriteString(fmt.Sprintf("%d. %s\n", i+1, option.Label))
		}

		if err := msgService.SendMessage(ctx, from, optionsText.String()); err != nil {
			slog.Error("BranchHook failed to send guidance", "error", err, "from", from)
		}

		slog.Debug("BranchHook handled invalid selection", "from", from, "responseText", responseText)
		return true, nil // We handled it, even though it was invalid
	}
}

// CreateGenAIHook creates a response handler for GenAI-generated prompts that
// acknowledges responses and can optionally trigger follow-up generation.
func CreateGenAIHook(originalPrompt models.Prompt, msgService Service) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("GenAIHook processing response", "from", from, "responseText_length", len(responseText))

		// For GenAI prompts, we provide a thoughtful acknowledgment
		var responseMsg string

		// Categorize the response type and provide appropriate feedback
		responseLen := len(strings.TrimSpace(responseText))
		if responseLen == 0 {
			responseMsg = "📝 I received your message. Thank you for responding!"
		} else if responseLen < 20 {
			responseMsg = "✨ Thanks for your response! Your input helps us provide better assistance."
		} else if responseLen < 100 {
			responseMsg = "💭 Thank you for sharing your thoughts! Your detailed response is valuable."
		} else {
			responseMsg = "📚 Wow, thank you for such a detailed response! Your insights are really helpful."
		}

		if err := msgService.SendMessage(ctx, from, responseMsg); err != nil {
			slog.Error("GenAIHook failed to send acknowledgment", "error", err, "from", from)
			return false, fmt.Errorf("failed to send acknowledgment: %w", err)
		}

		slog.Info("GenAIHook handled response", "from", from, "response_length", responseLen)
		return true, nil
	}
}

// CreateStaticHook creates a response handler for static prompts that provides
// a simple acknowledgment for any responses.
func CreateStaticHook(msgService Service) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("StaticHook processing response", "from", from, "responseText_length", len(responseText))

		// Simple acknowledgment for static prompts
		ackMsg := "👍 Thanks for your response! We've recorded your message."

		if err := msgService.SendMessage(ctx, from, ackMsg); err != nil {
			slog.Error("StaticHook failed to send acknowledgment", "error", err, "from", from)
			return false, fmt.Errorf("failed to send acknowledgment: %w", err)
		}

		slog.Info("StaticHook handled response", "from", from)
		return true, nil
	}
}

// ResponseHandlerFactory creates appropriate response handlers based on prompt type.
type ResponseHandlerFactory struct {
	msgService Service
}

// NewResponseHandlerFactory creates a new factory for response handlers.
func NewResponseHandlerFactory(msgService Service) *ResponseHandlerFactory {
	return &ResponseHandlerFactory{
		msgService: msgService,
	}
}

// CreateHandlerForPrompt creates and returns a response handler appropriate for the given prompt type.
// Returns nil if no specific handler is needed for the prompt type.
func (f *ResponseHandlerFactory) CreateHandlerForPrompt(prompt models.Prompt) ResponseAction {
	switch prompt.Type {
	case models.PromptTypeBranch:
		if len(prompt.BranchOptions) > 0 {
			return CreateBranchHook(prompt.BranchOptions, f.msgService)
		}
		return nil
	case models.PromptTypeGenAI:
		return CreateGenAIHook(prompt, f.msgService)
	case models.PromptTypeStatic:
		// Only create a handler for static prompts if they seem to expect a response
		if strings.Contains(strings.ToLower(prompt.Body), "reply") ||
			strings.Contains(strings.ToLower(prompt.Body), "respond") ||
			strings.Contains(strings.ToLower(prompt.Body), "answer") ||
			strings.Contains(prompt.Body, "?") {
			return CreateStaticHook(f.msgService)
		}
		return nil
	case models.PromptTypeCustom:
		// Custom prompts should register their own handlers
		// The micro health intervention already does this
		return nil
	default:
		return nil
	}
}

// AutoRegisterResponseHandler automatically registers a response handler for a prompt
// if one is needed. Returns true if a handler was registered.
func (rh *ResponseHandler) AutoRegisterResponseHandler(prompt models.Prompt, timeoutDuration time.Duration) bool {
	factory := NewResponseHandlerFactory(rh.msgService)
	handler := factory.CreateHandlerForPrompt(prompt)

	if handler == nil {
		slog.Debug("No response handler needed for prompt type", "type", prompt.Type, "to", prompt.To)
		return false
	}

	// Wrap the handler with timeout logic if specified
	if timeoutDuration > 0 {
		handler = rh.createTimeoutWrapper(handler, timeoutDuration)
	}

	if err := rh.RegisterHook(prompt.To, handler); err != nil {
		slog.Error("Failed to auto-register response handler", "error", err, "type", prompt.Type, "to", prompt.To)
		return false
	}

	slog.Info("Auto-registered response handler", "type", prompt.Type, "to", prompt.To, "timeout", timeoutDuration)
	return true
}

// createTimeoutWrapper wraps a response handler with timeout logic.
func (rh *ResponseHandler) createTimeoutWrapper(handler ResponseAction, timeout time.Duration) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		// Create a context with timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Run the handler with timeout
		done := make(chan struct{})
		var result bool
		var err error

		go func() {
			defer close(done)
			result, err = handler(timeoutCtx, from, responseText, timestamp)
		}()

		select {
		case <-done:
			return result, err
		case <-timeoutCtx.Done():
			slog.Warn("Response handler timed out", "from", from, "timeout", timeout)

			// Send timeout message
			timeoutMsg := "⏰ Response processing timed out. Please try again or contact support if this continues."
			if sendErr := rh.msgService.SendMessage(ctx, from, timeoutMsg); sendErr != nil {
				slog.Error("Failed to send timeout message", "error", sendErr, "from", from)
			}

			return false, fmt.Errorf("response handler timed out after %v", timeout)
		}
	}
}

// SetAutoCleanupTimeout sets up automatic cleanup of response handlers after a specified duration.
// This is useful for preventing memory leaks from long-lived handlers.
func (rh *ResponseHandler) SetAutoCleanupTimeout(recipient string, duration time.Duration) {
	go func() {
		time.Sleep(duration)
		if err := rh.UnregisterHook(recipient); err != nil {
			slog.Debug("Auto-cleanup failed for response handler", "error", err, "recipient", recipient)
		} else {
			slog.Debug("Auto-cleaned up response handler", "recipient", recipient, "after", duration)
		}
	}()
}

// StateManager interface for state operations (to avoid circular imports)
type StateManager interface {
	GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error)
	SetCurrentState(ctx context.Context, participantID string, flowType models.FlowType, state models.StateType) error
	GetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey) (string, error)
	SetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey, value string) error
	TransitionState(ctx context.Context, participantID string, flowType models.FlowType, fromState, toState models.StateType) error
}

// Helper functions for timer management

// cancelExistingTimer cancels any existing timer for the given timer key
func cancelExistingTimer(ctx context.Context, participantID string, stateManager StateManager, timer models.Timer, timerKey models.DataKey) {
	if timerID, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, timerKey); err == nil && timerID != "" {
		if err := timer.Cancel(timerID); err != nil {
			slog.Debug("Failed to cancel timer", "timerID", timerID, "participantID", participantID, "error", err)
		} else {
			slog.Debug("Cancelled existing timer", "timerID", timerID, "participantID", participantID)
		}
		// Clear the timer ID from state
		stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, timerKey, "")
	}
}

// scheduleTimeout schedules a timeout for the given duration and stores the timer ID
func scheduleTimeout(ctx context.Context, participantID string, from string, duration time.Duration, timeoutAction func(), stateManager StateManager, timer models.Timer, timerKey models.DataKey) error {
	// Cancel any existing timer first
	cancelExistingTimer(ctx, participantID, stateManager, timer, timerKey)

	// Schedule new timer
	timerID, err := timer.ScheduleAfter(duration, timeoutAction)
	if err != nil {
		slog.Error("Failed to schedule timeout", "error", err, "participantID", participantID, "duration", duration)
		return err
	}

	// Store timer ID in state
	if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, timerKey, timerID); err != nil {
		slog.Error("Failed to store timer ID", "error", err, "participantID", participantID, "timerID", timerID)
		// Try to cancel the timer since we couldn't store its ID
		timer.Cancel(timerID)
		return err
	}

	slog.Debug("Scheduled timeout", "participantID", participantID, "duration", duration, "timerID", timerID)
	return nil
}

// Timeout handler functions

// handleFeelingTimeout handles timeout when user doesn't respond to feeling prompt within 15 minutes
func handleFeelingTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Feeling prompt timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingTimerID, "")

	// Set feeling response to "timed_out" and proceed to random assignment
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse, "timed_out")
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateRandomAssignment)

	// Perform random assignment and send intervention
	performRandomAssignmentAndSendIntervention(ctx, participantID, from, stateManager, msgService, timer)
}

// handleCompletionTimeout handles timeout when user doesn't respond to intervention completion within 30 minutes
func handleCompletionTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Completion timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionTimerID, "")

	// Set completion response to "no_reply" and move to "did you get a chance"
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionResponse, "no_reply")
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateDidYouGetAChance)

	// Send "did you get a chance" message and schedule timeout
	if err := msgService.SendMessage(ctx, from, flow.MsgDidYouGetAChance); err != nil {
		slog.Error("Failed to send 'did you get a chance' message after timeout", "error", err, "participantID", participantID)
		return
	}

	scheduleTimeout(ctx, participantID, from, flow.DidYouGetAChanceTimeout, func() {
		handleDidYouGetAChanceTimeout(ctx, participantID, from, stateManager, msgService, timer)
	}, stateManager, timer, models.DataKeyDidYouGetAChanceTimerID)
}

// handleDidYouGetAChanceTimeout handles timeout when user doesn't respond to "did you get a chance" within 15 minutes
func handleDidYouGetAChanceTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Did you get a chance timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyDidYouGetAChanceTimerID, "")

	// Set response to "no_reply" and transition to IGNORED_PATH
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyGotChanceResponse, "no_reply")
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateIgnoredPath)

	// Send ignored path messages
	if err := msgService.SendMessage(ctx, from, flow.MsgIgnoredPath); err != nil {
		slog.Error("Failed to send ignored path message", "error", err, "participantID", participantID)
	}

	// Transition to END_OF_DAY
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
}

// handleContextTimeout handles timeout when user doesn't respond to context question within 15 minutes
func handleContextTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Context timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyContextTimerID, "")

	// Skip to END_OF_DAY
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
}

// handleMoodTimeout handles timeout when user doesn't respond to mood question within 15 minutes
func handleMoodTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Mood timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyMoodTimerID, "")

	// Skip to END_OF_DAY
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
}

// handleBarrierCheckTimeout handles timeout when user doesn't respond to barrier detail question within 30 minutes
func handleBarrierCheckTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Barrier check timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierCheckTimerID, "")

	// Transition to END_OF_DAY
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
}

// handleBarrierReasonTimeout handles timeout when user doesn't respond to barrier reason question within 30 minutes
func handleBarrierReasonTimeout(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) {
	slog.Debug("Barrier reason timeout occurred", "participantID", participantID)

	// Clear the timer ID from state
	stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonTimerID, "")

	// Transition to END_OF_DAY
	stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
}

// Helper functions for handling specific intervention state responses

func handleCommitmentResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel commitment timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyCommitmentTimerID)

	switch responseText {
	case "1":
		// User chose "Let's do it!" - proceed to feeling prompt
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateFeelingPrompt); err != nil {
			return false, err
		}

		// Send feeling prompt message using global variable
		if err := msgService.SendMessage(ctx, from, flow.MsgFeeling); err != nil {
			slog.Error("Failed to send feeling prompt", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send feeling prompt: %w", err)
		}

		// Schedule feeling timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.FeelingTimeout, func() {
			handleFeelingTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyFeelingTimerID); err != nil {
			slog.Error("Failed to schedule feeling timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	case "2":
		// User chose "Not yet" - end for today
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay); err != nil {
			return false, err
		}

		// Send end of day message
		if err := msgService.SendMessage(ctx, from, flow.MsgEndOfDay); err != nil {
			slog.Error("Failed to send end of day message", "error", err, "participantID", participantID)
		}

		return true, nil
	default:
		// Invalid response - ask them to try again
		if err := msgService.SendMessage(ctx, from, flow.MsgInvalidCommitmentChoice); err != nil {
			slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
		}
		return true, nil // We handled it, even if invalid
	}
}

func handleFeelingResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel feeling timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyFeelingTimerID)

	// Check if it's a valid feeling response (1-5)
	if responseText >= "1" && responseText <= "5" {
		// Store the feeling response
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse, responseText); err != nil {
			return false, err
		}

		// Transition to random assignment
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateRandomAssignment); err != nil {
			return false, err
		}

		// Perform random assignment and send appropriate intervention
		return performRandomAssignmentAndSendIntervention(ctx, participantID, from, stateManager, msgService, timer)
	}

	// Invalid response - ask them to try again
	if err := msgService.SendMessage(ctx, from, flow.MsgInvalidFeelingChoice); err != nil {
		slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
	}
	return true, nil
}

func performRandomAssignmentAndSendIntervention(ctx context.Context, participantID, from string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	// Simple random assignment using Go 1.20+ recommended approach
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	isImmediate := rng.Float64() < 0.5

	var nextState models.StateType
	var interventionMsg string
	if isImmediate {
		nextState = models.StateSendInterventionImmediate
		interventionMsg = flow.MsgImmediateIntervention
		// Store assignment for tracking
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFlowAssignment, string(models.FlowAssignmentImmediate)); err != nil {
			return false, err
		}
	} else {
		nextState = models.StateSendInterventionReflective
		interventionMsg = flow.MsgReflectiveIntervention
		// Store assignment for tracking
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFlowAssignment, string(models.FlowAssignmentReflective)); err != nil {
			return false, err
		}
	}

	// Transition to intervention state
	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, nextState); err != nil {
		return false, err
	}

	// Send intervention message using the phone number (from), not participantID
	if err := msgService.SendMessage(ctx, from, interventionMsg); err != nil {
		slog.Error("Failed to send intervention message", "error", err, "participantID", participantID)
		return false, fmt.Errorf("failed to send intervention message: %w", err)
	}

	// Schedule completion timeout
	if err := scheduleTimeout(ctx, participantID, from, flow.CompletionTimeout, func() {
		handleCompletionTimeout(ctx, participantID, from, stateManager, msgService, timer)
	}, stateManager, timer, models.DataKeyCompletionTimerID); err != nil {
		slog.Error("Failed to schedule completion timeout", "error", err, "participantID", participantID)
	}

	return true, nil
}

func handleCompletionResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.ToLower(strings.TrimSpace(responseText))

	// Cancel completion timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyCompletionTimerID)

	switch responseText {
	case string(models.ResponseDone):
		// User completed the habit - send reinforcement
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateReinforcementFollowup); err != nil {
			return false, err
		}

		// Send reinforcement message
		if err := msgService.SendMessage(ctx, from, flow.MsgReinforcement); err != nil {
			slog.Error("Failed to send reinforcement message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send reinforcement message: %w", err)
		}

		// Transition to end of day after reinforcement
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay); err != nil {
			slog.Error("Failed to transition to END_OF_DAY after reinforcement", "error", err, "participantID", participantID)
		}

		return true, nil
	case string(models.ResponseNo):
		// User explicitly said no - ask if they got a chance
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateDidYouGetAChance); err != nil {
			return false, err
		}

		// Send "did you get a chance" message
		if err := msgService.SendMessage(ctx, from, flow.MsgDidYouGetAChance); err != nil {
			slog.Error("Failed to send 'did you get a chance' message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send followup message: %w", err)
		}

		// Schedule did you get a chance timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.DidYouGetAChanceTimeout, func() {
			handleDidYouGetAChanceTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyDidYouGetAChanceTimerID); err != nil {
			slog.Error("Failed to schedule did you get a chance timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	default:
		// Invalid response for completion check - provide guidance
		retryMsg := "Please reply 'Done' if you completed the habit, or 'No' if you didn't."
		if err := msgService.SendMessage(ctx, from, retryMsg); err != nil {
			slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
		}
		return true, nil
	}
}

func handleDidYouGetAChanceResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel did you get a chance timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyDidYouGetAChanceTimerID)

	switch responseText {
	case "1":
		// Yes, they got a chance - ask context question
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateContextQuestion); err != nil {
			return false, err
		}

		// Send context question
		if err := msgService.SendMessage(ctx, from, flow.MsgContext); err != nil {
			slog.Error("Failed to send context message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send context message: %w", err)
		}

		// Schedule context timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.ContextTimeout, func() {
			handleContextTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyContextTimerID); err != nil {
			slog.Error("Failed to schedule context timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	case "2":
		// No, they didn't get a chance - ask barrier reason
		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateBarrierReasonNoChance); err != nil {
			return false, err
		}

		// Send barrier reason question
		if err := msgService.SendMessage(ctx, from, flow.MsgBarrierReason); err != nil {
			slog.Error("Failed to send barrier reason message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send barrier reason message: %w", err)
		}

		// Schedule barrier reason timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.BarrierReasonTimeout, func() {
			handleBarrierReasonTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyBarrierReasonTimerID); err != nil {
			slog.Error("Failed to schedule barrier reason timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	default:
		// Invalid response - provide guidance
		if err := msgService.SendMessage(ctx, from, flow.MsgInvalidDidYouGetAChance); err != nil {
			slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
		}
		return true, nil
	}
}

func handleContextResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel context timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyContextTimerID)

	if responseText >= "1" && responseText <= "4" {
		// Valid context response - store it and move to mood question
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyContextResponse, responseText); err != nil {
			return false, err
		}

		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateMoodQuestion); err != nil {
			return false, err
		}

		// Send mood question
		if err := msgService.SendMessage(ctx, from, flow.MsgMood); err != nil {
			slog.Error("Failed to send mood message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send mood message: %w", err)
		}

		// Schedule mood timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.MoodTimeout, func() {
			handleMoodTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyMoodTimerID); err != nil {
			slog.Error("Failed to schedule mood timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	}

	// Invalid response - provide guidance
	if err := msgService.SendMessage(ctx, from, flow.MsgInvalidContextChoice); err != nil {
		slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
	}
	return true, nil
}

func handleMoodResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel mood timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyMoodTimerID)

	if responseText >= "1" && responseText <= "3" {
		// Valid mood response - store it and move to barrier check
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyMoodResponse, responseText); err != nil {
			return false, err
		}

		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateBarrierCheckAfterContextMood); err != nil {
			return false, err
		}

		// Send barrier detail question (free text)
		if err := msgService.SendMessage(ctx, from, flow.MsgBarrierDetail); err != nil {
			slog.Error("Failed to send barrier detail message", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to send barrier detail message: %w", err)
		}

		// Schedule barrier check timeout
		if err := scheduleTimeout(ctx, participantID, from, flow.BarrierCheckTimeout, func() {
			handleBarrierCheckTimeout(ctx, participantID, from, stateManager, msgService, timer)
		}, stateManager, timer, models.DataKeyBarrierCheckTimerID); err != nil {
			slog.Error("Failed to schedule barrier check timeout", "error", err, "participantID", participantID)
		}

		return true, nil
	}

	// Invalid response - provide guidance
	if err := msgService.SendMessage(ctx, from, flow.MsgInvalidMoodChoice); err != nil {
		slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
	}
	return true, nil
}

func handleBarrierResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	// Cancel barrier check timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyBarrierCheckTimerID)

	// Any text response is valid for barrier details - store and end
	if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierResponse, responseText); err != nil {
		return false, err
	}

	if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay); err != nil {
		return false, err
	}

	// Send acknowledgment and end day message
	endMsg := "Thank you for sharing. Your insights help us improve the program. Have a good rest of your day! 🌟"
	if err := msgService.SendMessage(ctx, from, endMsg); err != nil {
		slog.Error("Failed to send end of day acknowledgment", "error", err, "participantID", participantID)
	}

	return true, nil
}

func handleBarrierReasonResponse(ctx context.Context, participantID, from, responseText string, stateManager StateManager, msgService Service, timer models.Timer) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Cancel barrier reason timeout since user responded
	cancelExistingTimer(ctx, participantID, stateManager, timer, models.DataKeyBarrierReasonTimerID)

	if responseText >= "1" && responseText <= "4" {
		// Valid barrier reason response - store and end
		if err := stateManager.SetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyBarrierReasonResponse, responseText); err != nil {
			return false, err
		}

		if err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay); err != nil {
			return false, err
		}

		// Send acknowledgment and end day message
		endMsg := "Thank you for letting us know. We understand that things come up. Tomorrow is a new opportunity! 💪"
		if err := msgService.SendMessage(ctx, from, endMsg); err != nil {
			slog.Error("Failed to send end of day acknowledgment", "error", err, "participantID", participantID)
		}

		return true, nil
	}

	// Invalid response - provide guidance
	if err := msgService.SendMessage(ctx, from, flow.MsgInvalidBarrierChoice); err != nil {
		slog.Error("Failed to send retry message", "error", err, "participantID", participantID)
	}
	return true, nil
}
