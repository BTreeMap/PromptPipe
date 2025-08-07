// Package messaging provides response handling functionality for stateful interactions.
package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
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
	// store is used for validating active participants and persisting hooks
	store store.Store
	// hookRegistry provides factory functions for recreating hooks from persistence
	hookRegistry *HookRegistry
	// stateManager is used for creating hooks that require state management
	stateManager flow.StateManager
	// timer is used for creating hooks that require timer functionality
	timer models.Timer
}

// NewResponseHandler creates a new ResponseHandler with the given messaging service.
func NewResponseHandler(msgService Service, store store.Store) *ResponseHandler {
	return &ResponseHandler{
		hooks:          make(map[string]ResponseAction),
		msgService:     msgService,
		defaultMessage: "📝 Your message has been recorded. Thank you for your response!",
		store:          store,
		hookRegistry:   NewHookRegistry(),
	}
}

// RegisterHook registers a response action for a specific participant.
// The recipient should be a canonicalized phone number (e.g., "1234567890").
func (rh *ResponseHandler) RegisterHook(recipient string, action ResponseAction) error {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Error("ResponseHandler.RegisterHook: validation failed", "error", err, "recipient", recipient)
		return fmt.Errorf("invalid recipient: %w", err)
	}

	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.hooks[canonicalRecipient] = action

	slog.Debug("ResponseHandler.RegisterHook: hook registered", "recipient", canonicalRecipient)
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
	case models.PromptTypeConversation:
		// Conversation prompts register their own handlers during participant enrollment
		// Return nil since no automatic handler registration is needed
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
func (rh *ResponseHandler) AutoRegisterResponseHandler(prompt models.Prompt) bool {
	factory := NewResponseHandlerFactory(rh.msgService)
	handler := factory.CreateHandlerForPrompt(prompt)

	if handler == nil {
		slog.Debug("No response handler needed for prompt type", "type", prompt.Type, "to", prompt.To)
		return false
	}

	if err := rh.RegisterHook(prompt.To, handler); err != nil {
		slog.Error("Failed to auto-register response handler", "error", err, "type", prompt.Type, "to", prompt.To)
		return false
	}

	slog.Info("Auto-registered response handler", "type", prompt.Type, "to", prompt.To)
	return true
}

// SetDependencies sets the state manager and timer for hook creation
// This should be called during server initialization
func (rh *ResponseHandler) SetDependencies(stateManager flow.StateManager, timer models.Timer) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.stateManager = stateManager
	rh.timer = timer
	slog.Debug("ResponseHandler dependencies set", "hasStateManager", stateManager != nil, "hasTimer", timer != nil)
}

// RegisterPersistentHook registers a response action that will be persisted to the database
func (rh *ResponseHandler) RegisterPersistentHook(recipient string, hookType models.HookType, params map[string]string) error {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Error("ResponseHandler RegisterPersistentHook validation failed", "error", err, "recipient", recipient)
		return fmt.Errorf("invalid recipient: %w", err)
	}

	// Create the hook using the registry
	action, err := rh.hookRegistry.CreateHook(hookType, params, rh.stateManager, rh.msgService, rh.timer)
	if err != nil {
		slog.Error("ResponseHandler RegisterPersistentHook creation failed", "error", err, "recipient", canonicalRecipient, "hookType", hookType)
		return fmt.Errorf("failed to create hook: %w", err)
	}

	// Register the hook in memory
	rh.mu.Lock()
	rh.hooks[canonicalRecipient] = action
	rh.mu.Unlock()

	// Persist the hook to database
	registeredHook := models.RegisteredHook{
		PhoneNumber: canonicalRecipient,
		HookType:    hookType,
		Parameters:  params,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := rh.store.SaveRegisteredHook(registeredHook); err != nil {
		// Remove from memory if database save failed
		rh.mu.Lock()
		delete(rh.hooks, canonicalRecipient)
		rh.mu.Unlock()

		slog.Error("ResponseHandler RegisterPersistentHook persistence failed", "error", err, "recipient", canonicalRecipient)
		return fmt.Errorf("failed to persist hook: %w", err)
	}

	slog.Debug("ResponseHandler persistent hook registered", "recipient", canonicalRecipient, "hookType", hookType)
	return nil
}

// RecoverPersistentHooks restores hooks from the database using the HookRegistry
// This method implements best-effort recovery: it will warn but not error if recreation fails
func (rh *ResponseHandler) RecoverPersistentHooks(ctx context.Context) error {
	slog.Info("Starting persistent hook recovery")

	// Get all registered hooks from the database
	registeredHooks, err := rh.store.ListRegisteredHooks()
	if err != nil {
		slog.Error("Failed to list registered hooks from database", "error", err)
		return fmt.Errorf("failed to list registered hooks: %w", err)
	}

	if len(registeredHooks) == 0 {
		slog.Info("No persistent hooks found in database")
		return nil
	}

	var recoveredCount int
	var failedCount int

	for _, registeredHook := range registeredHooks {
		slog.Debug("Attempting to recover hook",
			"phoneNumber", registeredHook.PhoneNumber,
			"hookType", registeredHook.HookType)

		// Attempt to recreate the hook using the registry
		action, err := rh.hookRegistry.CreateHook(
			registeredHook.HookType,
			registeredHook.Parameters,
			rh.stateManager,
			rh.msgService,
			rh.timer,
		)

		if err != nil {
			slog.Warn("Failed to recreate hook during recovery - skipping",
				"error", err,
				"phoneNumber", registeredHook.PhoneNumber,
				"hookType", registeredHook.HookType,
				"parameters", registeredHook.Parameters)
			failedCount++
			continue
		}

		// Register the recreated hook in memory
		rh.mu.Lock()
		rh.hooks[registeredHook.PhoneNumber] = action
		rh.mu.Unlock()

		recoveredCount++
		slog.Debug("Successfully recovered hook",
			"phoneNumber", registeredHook.PhoneNumber,
			"hookType", registeredHook.HookType)
	}

	slog.Info("Persistent hook recovery completed",
		"total", len(registeredHooks),
		"recovered", recoveredCount,
		"failed", failedCount)

	// Don't return error even if some hooks failed to recover
	// This is best-effort recovery as per requirements
	return nil
}

// CleanupStaleHooks removes hooks from the database for participants who are no longer active
// This should be called periodically to maintain database hygiene
func (rh *ResponseHandler) CleanupStaleHooks(ctx context.Context) error {
	slog.Info("Starting stale hook cleanup")

	// Get all registered hooks from the database
	registeredHooks, err := rh.store.ListRegisteredHooks()
	if err != nil {
		slog.Error("Failed to list registered hooks for cleanup", "error", err)
		return fmt.Errorf("failed to list registered hooks: %w", err)
	}

	if len(registeredHooks) == 0 {
		slog.Debug("No hooks to cleanup")
		return nil
	}

	// Get lists of active participants from conversation flow
	conversationParticipants, err := rh.store.ListConversationParticipants()
	if err != nil {
		slog.Error("Failed to list conversation participants for cleanup", "error", err)
		return fmt.Errorf("failed to list conversation participants: %w", err)
	}

	// Create a map of active phone numbers for quick lookup
	activePhones := make(map[string]bool)

	// Add active conversation participants
	for _, participant := range conversationParticipants {
		if participant.Status == models.ConversationStatusActive {
			canonical, err := rh.msgService.ValidateAndCanonicalizeRecipient(participant.PhoneNumber)
			if err != nil {
				slog.Warn("Failed to canonicalize conversation participant phone during cleanup",
					"error", err, "phone", participant.PhoneNumber, "id", participant.ID)
				continue
			}
			activePhones[canonical] = true
		}
	}

	var cleanedCount int

	// Remove hooks for participants who are no longer active
	for _, hook := range registeredHooks {
		if !activePhones[hook.PhoneNumber] {
			if err := rh.store.DeleteRegisteredHook(hook.PhoneNumber); err != nil {
				slog.Warn("Failed to delete stale hook from database",
					"error", err, "phoneNumber", hook.PhoneNumber)
				continue
			}

			// Also remove from memory if present
			rh.mu.Lock()
			delete(rh.hooks, hook.PhoneNumber)
			rh.mu.Unlock()

			cleanedCount++
			slog.Debug("Cleaned up stale hook", "phoneNumber", hook.PhoneNumber, "hookType", hook.HookType)
		}
	}

	slog.Info("Stale hook cleanup completed",
		"total_hooks", len(registeredHooks),
		"cleaned", cleanedCount)

	return nil
}

// UnregisterPersistentHook removes a persistent hook from both memory and database
func (rh *ResponseHandler) UnregisterPersistentHook(recipient string) error {
	// Validate and canonicalize recipient
	canonicalRecipient, err := rh.msgService.ValidateAndCanonicalizeRecipient(recipient)
	if err != nil {
		slog.Error("ResponseHandler UnregisterPersistentHook validation failed", "error", err, "recipient", recipient)
		return fmt.Errorf("invalid recipient: %w", err)
	}

	// Remove from memory
	rh.mu.Lock()
	delete(rh.hooks, canonicalRecipient)
	rh.mu.Unlock()

	// Remove from database
	if err := rh.store.DeleteRegisteredHook(canonicalRecipient); err != nil {
		slog.Error("Failed to delete persistent hook from database", "error", err, "recipient", canonicalRecipient)
		return fmt.Errorf("failed to delete persistent hook: %w", err)
	}

	slog.Debug("ResponseHandler persistent hook unregistered", "recipient", canonicalRecipient)
	return nil
}

// ValidateAndCleanupHooks removes hooks for participants who are no longer active.
// This should be called during startup and optionally periodically to prevent memory leaks
// while ensuring active users can always respond.
func (rh *ResponseHandler) ValidateAndCleanupHooks(ctx context.Context) error {
	slog.Info("Starting response handler validation and cleanup")

	rh.mu.Lock()
	defer rh.mu.Unlock()

	var removedCount int
	var errorCount int

	// Get lists of active participants from conversation flow
	conversationParticipants, err := rh.store.ListConversationParticipants()
	if err != nil {
		slog.Error("Failed to list conversation participants for validation", "error", err)
		errorCount++
	}

	// Create a map of active phone numbers for quick lookup
	activePhones := make(map[string]bool)

	// Add active conversation participants
	for _, participant := range conversationParticipants {
		if participant.Status == models.ConversationStatusActive {
			canonical, err := rh.msgService.ValidateAndCanonicalizeRecipient(participant.PhoneNumber)
			if err != nil {
				slog.Warn("Failed to canonicalize conversation participant phone",
					"error", err, "phone", participant.PhoneNumber, "id", participant.ID)
				continue
			}
			activePhones[canonical] = true
		}
	}

	// Remove hooks for participants who are no longer active
	for phoneNumber := range rh.hooks {
		if !activePhones[phoneNumber] {
			delete(rh.hooks, phoneNumber)
			removedCount++
			slog.Debug("Removed hook for inactive participant", "phone", phoneNumber)
		}
	}

	slog.Info("Response handler validation completed",
		"removed_hooks", removedCount,
		"remaining_hooks", len(rh.hooks),
		"errors", errorCount)

	if errorCount > 0 {
		return fmt.Errorf("validation completed with %d errors", errorCount)
	}

	return nil
}

// GetActiveParticipantCount returns the number of active participants across all flow types.
func (rh *ResponseHandler) GetActiveParticipantCount(ctx context.Context) (int, error) {
	var total int

	// Count active conversation participants
	conversationParticipants, err := rh.store.ListConversationParticipants()
	if err != nil {
		return 0, fmt.Errorf("failed to list conversation participants: %w", err)
	}

	for _, participant := range conversationParticipants {
		if participant.Status == models.ConversationStatusActive {
			total++
		}
	}

	return total, nil
}

// IsParticipantActive checks if a participant is active based on their phone number.
func (rh *ResponseHandler) IsParticipantActive(ctx context.Context, phoneNumber string) (bool, error) {
	canonical, err := rh.msgService.ValidateAndCanonicalizeRecipient(phoneNumber)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize phone number: %w", err)
	}

	// Check conversation participants
	conversationParticipant, err := rh.store.GetConversationParticipantByPhone(canonical)
	if err != nil {
		return false, fmt.Errorf("failed to get conversation participant: %w", err)
	}
	if conversationParticipant != nil && conversationParticipant.Status == models.ConversationStatusActive {
		return true, nil
	}

	return false, nil
}

// CreateConversationHook creates a specialized hook for conversation prompts that
// processes responses according to the conversation flow logic and maintains history.
func CreateConversationHook(participantID string, msgService Service) ResponseAction {
	return func(ctx context.Context, from, responseText string, timestamp int64) (bool, error) {
		slog.Debug("ConversationHook processing response", "from", from, "responseText", responseText, "participantID", participantID)

		// Add phone number to context for intervention tool
		ctx = context.WithValue(ctx, flow.GetPhoneNumberContextKey(), from)

		// Get the conversation flow generator from the flow registry
		generator, exists := flow.Get(models.PromptTypeConversation)
		if !exists {
			slog.Error("ConversationHook: conversation flow generator not registered")
			return false, fmt.Errorf("conversation flow generator not registered")
		}

		// Cast to conversation flow
		conversationFlow, ok := generator.(*flow.ConversationFlow)
		if !ok {
			slog.Error("ConversationHook: invalid generator type for conversation flow")
			return false, fmt.Errorf("invalid generator type for conversation flow")
		}

		// Process the response through the conversation flow
		aiResponse, err := conversationFlow.ProcessResponse(ctx, participantID, responseText)
		if err != nil {
			slog.Error("ConversationHook flow processing failed", "error", err, "participantID", participantID)
			return false, fmt.Errorf("failed to process response through conversation flow: %w", err)
		}

		// Send the AI response to the user
		if aiResponse != "" {
			if err := msgService.SendMessage(ctx, from, aiResponse); err != nil {
				slog.Error("ConversationHook failed to send AI response", "error", err, "participantID", participantID)
				return false, fmt.Errorf("failed to send AI response: %w", err)
			}
			slog.Info("ConversationHook sent AI response", "participantID", participantID, "responseLength", len(aiResponse))
		}

		return true, nil
	}
}
