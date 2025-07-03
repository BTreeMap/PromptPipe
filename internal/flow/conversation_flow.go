// Package flow provides conversation flow implementation for persistent conversational interactions.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
)

// ConversationMessage represents a single message in the conversation history.
type ConversationMessage struct {
	Role      string    `json:"role"`      // "user" or "assistant"
	Content   string    `json:"content"`   // message content
	Timestamp time.Time `json:"timestamp"` // when the message was sent
}

// ConversationHistory represents the full conversation history for a participant.
type ConversationHistory struct {
	Messages []ConversationMessage `json:"messages"`
}

// ConversationFlow implements a stateful conversation flow that maintains history and uses GenAI.
type ConversationFlow struct {
	stateManager     StateManager
	genaiClient      *genai.Client
	systemPrompt     string
	systemPromptFile string
}

// NewConversationFlow creates a new conversation flow with dependencies.
func NewConversationFlow(stateManager StateManager, genaiClient *genai.Client, systemPromptFile string) *ConversationFlow {
	slog.Debug("Creating ConversationFlow with dependencies", "systemPromptFile", systemPromptFile)
	return &ConversationFlow{
		stateManager:     stateManager,
		genaiClient:      genaiClient,
		systemPromptFile: systemPromptFile,
	}
}

// SetDependencies injects dependencies into the flow.
func (f *ConversationFlow) SetDependencies(deps Dependencies) {
	slog.Debug("ConversationFlow SetDependencies called")
	f.stateManager = deps.StateManager
	// Note: genaiClient needs to be set separately as it's not part of standard Dependencies
}

// LoadSystemPrompt loads the system prompt from the configured file.
func (f *ConversationFlow) LoadSystemPrompt() error {
	if f.systemPromptFile == "" {
		return fmt.Errorf("system prompt file not configured")
	}

	// Check if file exists
	if _, err := os.Stat(f.systemPromptFile); os.IsNotExist(err) {
		return fmt.Errorf("system prompt file does not exist: %s", f.systemPromptFile)
	}

	// Read system prompt from file
	content, err := os.ReadFile(f.systemPromptFile)
	if err != nil {
		slog.Error("Failed to read system prompt file", "file", f.systemPromptFile, "error", err)
		return fmt.Errorf("failed to read system prompt file: %w", err)
	}

	f.systemPrompt = strings.TrimSpace(string(content))
	slog.Info("System prompt loaded successfully", "file", f.systemPromptFile, "length", len(f.systemPrompt))
	return nil
}

// Generate generates conversation responses based on user input and history.
func (f *ConversationFlow) Generate(ctx context.Context, p models.Prompt) (string, error) {
	slog.Debug("ConversationFlow Generate invoked", "to", p.To, "userPrompt", p.UserPrompt)

	// For simple message generation without state operations, dependencies are not required
	// Dependencies are only needed for stateful operations like maintaining conversation history
	switch p.State {
	case "", models.StateConversationActive:
		// Return a generic response - actual conversation logic happens in ProcessResponse
		return "I'm ready to chat! Send me a message to start our conversation.", nil
	default:
		slog.Error("ConversationFlow unsupported state", "state", p.State, "to", p.To)
		return "", fmt.Errorf("unsupported conversation flow state '%s'", p.State)
	}
}

// ProcessResponse handles participant responses and maintains conversation state.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) ProcessResponse(ctx context.Context, participantID, response string) (string, error) {
	// Validate dependencies for stateful operations
	if f.stateManager == nil || f.genaiClient == nil {
		slog.Error("ConversationFlow dependencies not initialized for state operations")
		return "", fmt.Errorf("flow dependencies not properly initialized for state operations")
	}

	// Load system prompt if not already loaded
	if f.systemPrompt == "" {
		if err := f.LoadSystemPrompt(); err != nil {
			// If system prompt file doesn't exist or fails to load, use a default
			f.systemPrompt = "You are a helpful AI assistant. Engage in natural conversation with the user."
			slog.Warn("Using default system prompt due to load failure", "error", err)
		}
	}

	// Get current state
	currentState, err := f.stateManager.GetCurrentState(ctx, participantID, models.FlowTypeConversation)
	if err != nil {
		slog.Error("ConversationFlow ProcessResponse: failed to get current state", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get current state: %w", err)
	}

	// If no state exists, initialize the conversation
	if currentState == "" {
		err = f.transitionToState(ctx, participantID, models.StateConversationActive)
		if err != nil {
			return "", err
		}
		// Initialize empty conversation history
		emptyHistory := ConversationHistory{Messages: []ConversationMessage{}}
		historyJSON, _ := json.Marshal(emptyHistory)
		f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, string(historyJSON))
		slog.Debug("ConversationFlow initialized new conversation", "participantID", participantID)
	}

	slog.Debug("ConversationFlow ProcessResponse", "participantID", participantID, "response", response, "currentState", currentState)

	// Process the conversation response
	switch currentState {
	case models.StateConversationActive:
		return f.processConversationMessage(ctx, participantID, response)
	default:
		slog.Warn("ConversationFlow ProcessResponse: unhandled state", "state", currentState, "participantID", participantID)
		return "", fmt.Errorf("unhandled conversation state: %s", currentState)
	}
}

// processConversationMessage handles a user message and generates an AI response.
// Returns the AI response that should be sent back to the user.
func (f *ConversationFlow) processConversationMessage(ctx context.Context, participantID, userMessage string) (string, error) {
	// Get conversation history
	history, err := f.getConversationHistory(ctx, participantID)
	if err != nil {
		slog.Error("ConversationFlow failed to get conversation history", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Add user message to history
	userMsg := ConversationMessage{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, userMsg)

	// Build conversation context for GenAI
	conversationContext := f.buildConversationContext(history)

	// Generate response using GenAI
	response, err := f.genaiClient.GeneratePromptWithContext(ctx, f.systemPrompt, conversationContext)
	if err != nil {
		slog.Error("ConversationFlow GenAI generation failed", "error", err, "participantID", participantID)
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	// Add assistant response to history
	assistantMsg := ConversationMessage{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	}
	history.Messages = append(history.Messages, assistantMsg)

	// Save updated history
	err = f.saveConversationHistory(ctx, participantID, history)
	if err != nil {
		slog.Error("ConversationFlow failed to save conversation history", "error", err, "participantID", participantID)
		// Don't fail the request if we can't save history, but log the error
	}

	// Return the AI response for sending
	slog.Info("ConversationFlow generated response", "participantID", participantID, "responseLength", len(response))
	return response, nil
}

// transitionToState safely transitions to a new state with logging
func (f *ConversationFlow) transitionToState(ctx context.Context, participantID string, newState models.StateType) error {
	err := f.stateManager.SetCurrentState(ctx, participantID, models.FlowTypeConversation, newState)
	if err != nil {
		slog.Error("ConversationFlow failed to transition state", "error", err, "participantID", participantID, "newState", newState)
		return fmt.Errorf("failed to transition to state %s: %w", newState, err)
	}
	slog.Info("ConversationFlow state transition", "participantID", participantID, "newState", newState)
	return nil
}

// getConversationHistory retrieves and parses conversation history from state storage.
func (f *ConversationFlow) getConversationHistory(ctx context.Context, participantID string) (*ConversationHistory, error) {
	historyJSON, err := f.stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory)
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
		slog.Error("ConversationFlow failed to parse conversation history", "error", err, "participantID", participantID)
		// Return empty history if parsing fails
		return &ConversationHistory{Messages: []ConversationMessage{}}, nil
	}

	return &history, nil
}

// saveConversationHistory saves conversation history to state storage.
func (f *ConversationFlow) saveConversationHistory(ctx context.Context, participantID string, history *ConversationHistory) error {
	// Optionally limit history length to prevent unbounded growth
	const maxHistoryLength = 50 // Keep last 50 messages
	if len(history.Messages) > maxHistoryLength {
		// Keep the most recent messages
		history.Messages = history.Messages[len(history.Messages)-maxHistoryLength:]
		slog.Debug("ConversationFlow trimmed history to max length", "participantID", participantID, "maxLength", maxHistoryLength)
	}

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	err = f.stateManager.SetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyConversationHistory, string(historyJSON))
	if err != nil {
		return fmt.Errorf("failed to save conversation history: %w", err)
	}

	return nil
}

// buildConversationContext creates a formatted context string from conversation history.
func (f *ConversationFlow) buildConversationContext(history *ConversationHistory) string {
	if len(history.Messages) == 0 {
		return ""
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString("Previous conversation:\n\n")

	// Include recent conversation history (last 10 exchanges)
	start := 0
	if len(history.Messages) > 20 { // 10 exchanges = 20 messages
		start = len(history.Messages) - 20
	}

	for i := start; i < len(history.Messages)-1; i++ { // -1 to exclude the current user message
		msg := history.Messages[i]
		if msg.Role == "user" {
			contextBuilder.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		} else {
			contextBuilder.WriteString(fmt.Sprintf("Assistant: %s\n", msg.Content))
		}
	}

	// Add the current user message
	if len(history.Messages) > 0 {
		lastMsg := history.Messages[len(history.Messages)-1]
		if lastMsg.Role == "user" {
			contextBuilder.WriteString(fmt.Sprintf("\nCurrent user message: %s", lastMsg.Content))
		}
	}

	return contextBuilder.String()
}

// GetSystemPromptPath returns the default system prompt file path.
func GetSystemPromptPath() string {
	// Default to a prompts directory in the project root
	return filepath.Join("prompts", "conversation_system.txt")
}

// CreateDefaultSystemPromptFile creates a default system prompt file if it doesn't exist.
func CreateDefaultSystemPromptFile(filePath string) error {
	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		slog.Debug("System prompt file already exists", "path", filePath)
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Default system prompt content
	defaultContent := `You are a helpful, knowledgeable, and empathetic AI assistant. You engage in natural conversation with users, providing thoughtful and informative responses.

Key guidelines:
- Be conversational and friendly
- Provide helpful and accurate information
- Ask clarifying questions when needed
- Remember the context of our conversation
- Be concise but thorough in your responses
- Show empathy and understanding

Your goal is to have meaningful conversations and assist users with their questions and needs.`

	// Write default content to file
	err := os.WriteFile(filePath, []byte(defaultContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write default system prompt file: %w", err)
	}

	slog.Info("Created default system prompt file", "path", filePath)
	return nil
}
