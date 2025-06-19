// Package messaging provides response handling functionality for stateful interactions.
package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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
func CreateInterventionHook(participantID, phoneNumber string, stateManager StateManager, msgService Service) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("InterventionHook processing response", "participantID", participantID, "from", from, "responseText", responseText)

		// Get current state
		currentState, err := stateManager.GetCurrentState(ctx, participantID, "micro_health_intervention")
		if err != nil {
			slog.Error("InterventionHook failed to get current state", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to get current state: %w", err)
		}

		// Handle "Ready" override at any time (except during active flows)
		responseTextLower := strings.ToLower(strings.TrimSpace(responseText))
		if responseTextLower == "ready" {
			// Check if we're in END_OF_DAY state or orientation (can accept "Ready")
			if currentState == "ORIENTATION" || currentState == "END_OF_DAY" || currentState == "" {
				slog.Info("InterventionHook handling Ready override", "participantID", participantID)

				// Transition to COMMITMENT_PROMPT immediately
				if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "COMMITMENT_PROMPT"); err != nil {
					slog.Error("InterventionHook failed to transition to COMMITMENT_PROMPT", "error", err, "participantID", participantID)
					return false, fmt.Errorf("failed to transition state: %w", err)
				}

				// Send commitment prompt
				commitmentMsg := "You committed to trying a quick habit today—ready to go?\n1. 🚀 Let's do it!\n2. ⏳ Not yet\n(Reply with '1' or '2')"
				if err := msgService.SendMessage(ctx, from, commitmentMsg); err != nil {
					slog.Error("InterventionHook failed to send commitment prompt", "error", err, "participantID", participantID)
					return false, fmt.Errorf("failed to send commitment prompt: %w", err)
				}

				return true, nil
			}
		}

		// Handle responses based on current state
		switch currentState {
		case "COMMITMENT_PROMPT":
			return handleCommitmentResponse(ctx, participantID, responseText, stateManager, msgService)
		case "FEELING_PROMPT":
			return handleFeelingResponse(ctx, participantID, responseText, stateManager, msgService)
		case "SEND_INTERVENTION_IMMEDIATE", "SEND_INTERVENTION_REFLECTIVE":
			return handleCompletionResponse(ctx, participantID, responseText, stateManager, msgService)
		case "DID_YOU_GET_A_CHANCE":
			return handleDidYouGetAChanceResponse(ctx, participantID, responseText, stateManager, msgService)
		case "CONTEXT_QUESTION":
			return handleContextResponse(ctx, participantID, responseText, stateManager, msgService)
		case "MOOD_QUESTION":
			return handleMoodResponse(ctx, participantID, responseText, stateManager, msgService)
		case "BARRIER_CHECK_AFTER_CONTEXT_MOOD":
			return handleBarrierResponse(ctx, participantID, responseText, stateManager, msgService)
		case "END_OF_DAY":
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
		handler = rh.createTimeoutWrapper(handler, prompt.To, timeoutDuration)
	}

	if err := rh.RegisterHook(prompt.To, handler); err != nil {
		slog.Error("Failed to auto-register response handler", "error", err, "type", prompt.Type, "to", prompt.To)
		return false
	}

	slog.Info("Auto-registered response handler", "type", prompt.Type, "to", prompt.To, "timeout", timeoutDuration)
	return true
}

// createTimeoutWrapper wraps a response handler with timeout logic.
func (rh *ResponseHandler) createTimeoutWrapper(handler ResponseAction, recipient string, timeout time.Duration) ResponseAction {
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
	GetCurrentState(ctx context.Context, participantID, flowType string) (string, error)
	SetCurrentState(ctx context.Context, participantID, flowType, state string) error
	GetStateData(ctx context.Context, participantID, flowType, key string) (string, error)
	SetStateData(ctx context.Context, participantID, flowType, key, value string) error
	TransitionState(ctx context.Context, participantID, flowType, fromState, toState string) error
}

// Helper functions for handling specific state responses...
// (These would be implemented to handle the micro health intervention logic)

func handleCommitmentResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	switch responseText {
	case "1":
		// User chose "Let's do it!" - proceed to feeling prompt
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "FEELING_PROMPT"); err != nil {
			return false, err
		}

		// TODO: Send feeling prompt message - need participant phone number
		// For now, just transition state
		return true, nil
	case "2":
		// User chose "Not yet" - end for today
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "END_OF_DAY"); err != nil {
			return false, err
		}
		return true, nil
	default:
		// Invalid response - ask them to try again
		return false, nil
	}
}

func handleFeelingResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	// Check if it's a valid feeling response (1-5)
	if responseText >= "1" && responseText <= "5" {
		// Store the feeling response
		if err := stateManager.SetStateData(ctx, participantID, "micro_health_intervention", "feelingResponse", responseText); err != nil {
			return false, err
		}

		// Transition to random assignment
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "RANDOM_ASSIGNMENT"); err != nil {
			return false, err
		}

		// TODO: Implement random assignment logic and send appropriate intervention
		return true, nil
	}

	return false, nil
}

func handleCompletionResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.ToLower(strings.TrimSpace(responseText))

	switch responseText {
	case "done":
		// User completed the habit - send reinforcement
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "REINFORCEMENT_FOLLOWUP"); err != nil {
			return false, err
		}

		// TODO: Send reinforcement message and transition to END_OF_DAY
		return true, nil
	case "no":
		// User explicitly said no - ask if they got a chance
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "DID_YOU_GET_A_CHANCE"); err != nil {
			return false, err
		}

		// TODO: Send "did you get a chance" message
		return true, nil
	default:
		// Invalid response for completion check
		return false, nil
	}
}

func handleDidYouGetAChanceResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	switch responseText {
	case "1":
		// Yes, they got a chance - ask context question
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "CONTEXT_QUESTION"); err != nil {
			return false, err
		}
		// TODO: Send context question
		return true, nil
	case "2":
		// No, they didn't get a chance - ask barrier reason
		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "BARRIER_REASON_NO_CHANCE"); err != nil {
			return false, err
		}
		// TODO: Send barrier reason question
		return true, nil
	default:
		return false, nil
	}
}

func handleContextResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	if responseText >= "1" && responseText <= "4" {
		// Valid context response - store it and move to mood question
		if err := stateManager.SetStateData(ctx, participantID, "micro_health_intervention", "contextResponse", responseText); err != nil {
			return false, err
		}

		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "MOOD_QUESTION"); err != nil {
			return false, err
		}

		// TODO: Send mood question
		return true, nil
	}

	return false, nil
}

func handleMoodResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	responseText = strings.TrimSpace(responseText)

	if responseText >= "1" && responseText <= "3" {
		// Valid mood response - store it and move to barrier check
		if err := stateManager.SetStateData(ctx, participantID, "micro_health_intervention", "moodResponse", responseText); err != nil {
			return false, err
		}

		if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "BARRIER_CHECK_AFTER_CONTEXT_MOOD"); err != nil {
			return false, err
		}

		// TODO: Send barrier check question
		return true, nil
	}

	return false, nil
}

func handleBarrierResponse(ctx context.Context, participantID, responseText string, stateManager StateManager, msgService Service) (bool, error) {
	// Any text response is valid for barrier details - store and end
	if err := stateManager.SetStateData(ctx, participantID, "micro_health_intervention", "barrierResponse", responseText); err != nil {
		return false, err
	}

	if err := stateManager.SetCurrentState(ctx, participantID, "micro_health_intervention", "END_OF_DAY"); err != nil {
		return false, err
	}

	return true, nil
}
