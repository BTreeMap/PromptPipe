// Package flow provides one-minute intervention tool functionality for conversation flows.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// OneMinuteInterventionTool provides LLM tool functionality for initiating health interventions.
type OneMinuteInterventionTool struct {
	stateManager StateManager
	genaiClient  genai.ClientInterface
	msgService   MessagingService
}

// NewOneMinuteInterventionTool creates a new intervention tool instance.
func NewOneMinuteInterventionTool(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService) *OneMinuteInterventionTool {
	return &OneMinuteInterventionTool{
		stateManager: stateManager,
		genaiClient:  genaiClient,
		msgService:   msgService,
	}
}

// GetToolDefinition returns the OpenAI tool definition for initiating interventions.
func (oit *OneMinuteInterventionTool) GetToolDefinition() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: "function",
		Function: shared.FunctionDefinitionParam{
			Name:        "initiate_intervention",
			Description: openai.String("Initiate a personalized health intervention session for the user based on their conversation history and current needs"),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"intervention_focus": map[string]interface{}{
						"type":        "string",
						"description": "Focus or theme for the intervention (e.g., 'stress relief', 'energy boost', 'mindfulness', 'breathing', 'movement', or any custom approach that fits the user's needs)",
					},
					"personalization_notes": map[string]interface{}{
						"type":        "string",
						"description": "Additional notes or specific instructions to personalize the intervention based on the current conversation context and user's situation",
					},
				},
				"required": []string{}, // All parameters are optional
			},
		},
	}
}

// ExecuteOneMinuteIntervention executes the intervention tool call.
func (oit *OneMinuteInterventionTool) ExecuteOneMinuteIntervention(ctx context.Context, participantID string, params models.OneMinuteInterventionToolParams) (*models.ToolResult, error) {
	slog.Info("Executing intervention tool", "participantID", participantID, "interventionFocus", params.InterventionFocus)

	// Get phone number from context
	phoneNumber, ok := GetPhoneNumberFromContext(ctx)
	if !ok {
		slog.Error("OneMinuteInterventionTool: phone number not found in context", "participantID", participantID)
		return &models.ToolResult{
			Success: false,
			Message: "Failed to get participant contact information",
			Error:   "phone number not available in context",
		}, nil
	}

	// Validate parameters
	if err := params.Validate(); err != nil {
		return &models.ToolResult{
			Success: false,
			Message: "Invalid intervention parameters",
			Error:   err.Error(),
		}, nil
	}

	// Get current conversation history
	history, err := oit.getConversationHistory(ctx, participantID)
	if err != nil {
		slog.Error("OneMinuteInterventionTool failed to get conversation history", "error", err, "participantID", participantID)
		return &models.ToolResult{
			Success: false,
			Message: "Failed to retrieve conversation history",
			Error:   err.Error(),
		}, nil
	}

	// Create intervention system prompt
	interventionSystemPrompt := oit.buildInterventionSystemPrompt(params)

	// Build messages for intervention generation
	messages, err := oit.buildInterventionMessages(ctx, participantID, history, interventionSystemPrompt)
	if err != nil {
		slog.Error("OneMinuteInterventionTool failed to build intervention messages", "error", err, "participantID", participantID)
		return &models.ToolResult{
			Success: false,
			Message: "Failed to prepare intervention",
			Error:   err.Error(),
		}, nil
	}

	// Generate intervention start message
	interventionMessage, err := oit.genaiClient.GenerateWithMessages(ctx, messages)
	if err != nil {
		slog.Error("OneMinuteInterventionTool failed to generate intervention message", "error", err, "participantID", participantID)
		return &models.ToolResult{
			Success: false,
			Message: "Failed to generate intervention content",
			Error:   err.Error(),
		}, nil
	}

	// Send the intervention message to the user
	if err := oit.msgService.SendMessage(ctx, phoneNumber, interventionMessage); err != nil {
		slog.Error("OneMinuteInterventionTool failed to send intervention message", "error", err, "participantID", participantID, "phone", phoneNumber)
		return &models.ToolResult{
			Success: false,
			Message: "Failed to send intervention message",
			Error:   err.Error(),
		}, nil
	}

	slog.Info("Intervention message sent successfully", "participantID", participantID, "phone", phoneNumber, "messageLength", len(interventionMessage))

	// Add the intervention start message to conversation history
	interventionMsg := ConversationMessage{
		Role:      "assistant",
		Content:   interventionMessage,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, interventionMsg)

	// Save updated conversation history
	err = oit.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("OneMinuteInterventionTool failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the intervention if we can't save history
	}

	// Store intervention metadata for potential follow-up
	interventionData := map[string]interface{}{
		"focus":      params.InterventionFocus,
		"notes":      params.PersonalizationNotes,
		"started_at": time.Now().Format(time.RFC3339),
	}

	if interventionDataJSON, err := json.Marshal(interventionData); err == nil {
		oit.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, "current_intervention", string(interventionDataJSON))
	}

	successMessage := "✅ Perfect! I've started your health intervention. The message has been sent to guide you through this healthy moment."

	slog.Info("Intervention tool executed successfully", "participantID", participantID, "interventionFocus", params.InterventionFocus)

	return &models.ToolResult{
		Success: true,
		Message: successMessage,
	}, nil
}

// buildInterventionSystemPrompt creates a system prompt for the intervention based on parameters.
func (oit *OneMinuteInterventionTool) buildInterventionSystemPrompt(params models.OneMinuteInterventionToolParams) string {
	basePrompt := `You are now transitioning into a health intervention mode. Your role is to guide the user through a brief, effective, and personalized healthy activity based on their conversation history and current needs.

INTERVENTION GUIDELINES:
1. Keep the intervention focused and achievable - typically a few minutes but flexible based on the user's needs
2. Use the conversation history to personalize the intervention  
3. Be encouraging, gentle, and supportive
4. Provide clear, step-by-step instructions
5. Make it immediately actionable
6. Consider the user's current emotional state and context from the conversation

INTERVENTION STRUCTURE:
- Start with a brief, encouraging introduction
- Provide clear, simple instructions for the activity
- Guide them through the process step by step
- End with a gentle transition back to conversation
- Include a reflection question if appropriate

`

	// Add intervention-specific guidance if provided
	if params.InterventionFocus != "" {
		basePrompt += fmt.Sprintf(`INTERVENTION FOCUS: %s

`, params.InterventionFocus)
	}

	// Add personalization notes if provided
	if params.PersonalizationNotes != "" {
		basePrompt += fmt.Sprintf(`PERSONALIZATION NOTES: %s

`, params.PersonalizationNotes)
	}

	basePrompt += `Remember to be flexible and creative. The goal is to provide a meaningful, healthy moment that fits the user's current context and conversation history.`

	return basePrompt
}

// buildInterventionMessages creates OpenAI message array for intervention generation.
func (oit *OneMinuteInterventionTool) buildInterventionMessages(ctx context.Context, participantID string, history *ConversationHistory, interventionSystemPrompt string) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add intervention system prompt
	messages = append(messages, openai.SystemMessage(interventionSystemPrompt))

	// Get participant background if available
	participantBackground, err := oit.getParticipantBackground(ctx, participantID)
	if err != nil {
		slog.Warn("Failed to get participant background for intervention", "error", err, "participantID", participantID)
	} else if participantBackground != "" {
		messages = append(messages, openai.SystemMessage(participantBackground))
	}

	// Add recent conversation history for context (limit to last 20 messages to manage token usage)
	historyMessages := history.Messages
	const maxHistoryMessages = 20
	if len(historyMessages) > maxHistoryMessages {
		historyMessages = historyMessages[len(historyMessages)-maxHistoryMessages:]
	}

	for _, msg := range historyMessages {
		if msg.Role == "user" {
			messages = append(messages, openai.UserMessage(msg.Content))
		} else if msg.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	// Add intervention initiation prompt
	interventionPrompt := "Please initiate the health intervention now. Create a personalized intervention message based on our conversation history and guide me through a healthy activity."
	messages = append(messages, openai.UserMessage(interventionPrompt))

	return messages, nil
}

// getConversationHistory retrieves and parses conversation history from state storage.
func (oit *OneMinuteInterventionTool) getConversationHistory(ctx context.Context, participantID string) (*ConversationHistory, error) {
	historyJSON, err := oit.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	var history ConversationHistory
	if historyJSON == "" {
		// Return empty history if none exists
		return &ConversationHistory{Messages: []ConversationMessage{}}, nil
	}

	err = json.Unmarshal([]byte(historyJSON), &history)
	if err != nil {
		slog.Error("OneMinuteInterventionTool failed to parse conversation history", "error", err, "participantID", participantID)
		// Return empty history if parsing fails
		return &ConversationHistory{Messages: []ConversationMessage{}}, nil
	}

	return &history, nil
}

// saveConversationHistory saves conversation history to state storage.
func (oit *OneMinuteInterventionTool) saveConversationHistory(ctx context.Context, participantID string, history *ConversationHistory) error {
	// Optionally limit history length to prevent unbounded growth
	const maxHistoryLength = 50 // Keep last 50 messages
	if len(history.Messages) > maxHistoryLength {
		// Keep the most recent messages
		history.Messages = history.Messages[len(history.Messages)-maxHistoryLength:]
		slog.Debug("OneMinuteInterventionTool trimmed history to max length", "participantID", participantID, "maxLength", maxHistoryLength)
	}

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	err = oit.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, string(historyJSON))
	if err != nil {
		return fmt.Errorf("failed to save conversation history: %w", err)
	}

	return nil
}

// getParticipantBackground retrieves participant background information from state storage
func (oit *OneMinuteInterventionTool) getParticipantBackground(ctx context.Context, participantID string) (string, error) {
	// Try to get participant background from state data
	background, err := oit.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyParticipantBackground)
	if err != nil {
		slog.Debug("Error retrieving participant background for intervention", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get participant background: %w", err)
	}

	if background == "" {
		return "", nil
	}

	// Format as a system message
	formatted := fmt.Sprintf("PARTICIPANT BACKGROUND:\n%s", background)
	return formatted, nil
}
