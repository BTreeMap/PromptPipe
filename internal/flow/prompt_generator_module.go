// Package flow provides a first-class Prompt Generator module as a peer to Intake/Feedback.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/BTreeMap/PromptPipe/internal/genai"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/openai/openai-go"
)

// PromptGeneratorModule encapsulates prompt generation as a stateful module (PROMPT_GENERATOR state).
type PromptGeneratorModule struct {
	stateManager        StateManager
	genaiClient         genai.ClientInterface
	msgService          MessagingService
	stateTransitionTool *StateTransitionTool
	schedulerTool       *SchedulerTool
	profileSaveTool     *ProfileSaveTool
	promptGeneratorTool *PromptGeneratorTool
}

// NewPromptGeneratorModule creates a new module instance.
func NewPromptGeneratorModule(stateManager StateManager, genaiClient genai.ClientInterface, msgService MessagingService, stateTransitionTool *StateTransitionTool, schedulerTool *SchedulerTool, profileSaveTool *ProfileSaveTool, promptGeneratorTool *PromptGeneratorTool) *PromptGeneratorModule {
	slog.Debug("PromptGeneratorModule.NewPromptGeneratorModule: creating module", "hasStateManager", stateManager != nil, "hasGenAI", genaiClient != nil, "hasMessaging", msgService != nil)
	return &PromptGeneratorModule{
		stateManager:        stateManager,
		genaiClient:         genaiClient,
		msgService:          msgService,
		stateTransitionTool: stateTransitionTool,
		schedulerTool:       schedulerTool,
		profileSaveTool:     profileSaveTool,
		promptGeneratorTool: promptGeneratorTool,
	}
}

// LoadSystemPrompt loads the system prompt.
func (pgm *PromptGeneratorModule) LoadSystemPrompt() error {
	// Delegate to the prompt generator tool's prompt loader, so we keep a single prompt file.
	if pgm.promptGeneratorTool == nil {
		return fmt.Errorf("prompt generator tool not available")
	}
	return pgm.promptGeneratorTool.LoadSystemPrompt()
}

// ExecutePromptGeneratorWithHistoryAndConversation runs the module with conversation context.
func (pgm *PromptGeneratorModule) ExecutePromptGeneratorWithHistoryAndConversation(ctx context.Context, participantID string, args map[string]interface{}, chatHistory []openai.ChatCompletionMessageParamUnion, conversationHistory *ConversationHistory) (string, error) {
	slog.Debug("PromptGeneratorModule.ExecutePromptGeneratorWithHistoryAndConversation: start", "participantID", participantID, "historyLen", len(chatHistory))

	if pgm.genaiClient == nil || pgm.promptGeneratorTool == nil {
		return "", fmt.Errorf("dependencies not initialized")
	}

	// Tools available to the LLM when crafting the prompt and deciding next steps.
	tools := []openai.ChatCompletionToolParam{}

	if pgm.stateTransitionTool != nil {
		tools = append(tools, pgm.stateTransitionTool.GetToolDefinition())
	}
	if pgm.schedulerTool != nil {
		tools = append(tools, pgm.schedulerTool.GetToolDefinition())
	}
	if pgm.profileSaveTool != nil {
		tools = append(tools, pgm.profileSaveTool.GetToolDefinition())
	}
	// Expose the internal prompt generator as a function so the LLM can generate on-demand.
	tools = append(tools, pgm.promptGeneratorTool.GetToolDefinition())

	// Build a minimal message set: the module's system prompt (shared with tool) + prior chat + the user's latest input.
	// We reuse the tool's loaded system prompt to avoid prompt drift.
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(pgm.promptGeneratorTool.systemPrompt),
	}
	if len(chatHistory) > 0 {
		messages = append(messages, chatHistory...)
	}
	if userResp, ok := args["user_response"].(string); ok && userResp != "" {
		messages = append(messages, openai.UserMessage(userResp))
	}

	// First round with tools enabled.
	thinkingResp, err := pgm.genaiClient.GenerateThinkingWithTools(ctx, messages, tools)
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt (tools): %w", err)
	}
	if len(thinkingResp.ToolCalls) == 0 && thinkingResp.Content != "" {
		return thinkingResp.Content, nil
	}

	// Handle tool loop via the same execution helpers used by Intake/Feedback by leveraging their tool execution paths.
	// We’ll manually execute only the functions we own here to keep it simple.
	currentContent := thinkingResp.Content
	for _, tc := range thinkingResp.ToolCalls {
		switch tc.Function.Name {
		case "generate_habit_prompt":
			var toolArgs map[string]interface{}
			if err := json.Unmarshal(tc.Function.Arguments, &toolArgs); err != nil {
				slog.Error("PromptGeneratorModule: failed to parse generate_habit_prompt args", "error", err)
				continue
			}
			result, err := pgm.promptGeneratorTool.ExecutePromptGeneratorWithHistory(ctx, participantID, toolArgs, chatHistory)
			if err != nil {
				slog.Error("PromptGeneratorModule: generate_habit_prompt failed", "error", err, "participantID", participantID)
				continue
			}
			if conversationHistory != nil {
				conversationHistory.Messages = append(conversationHistory.Messages, ConversationMessage{Role: "assistant", Content: result})
			}
			currentContent = result

		case "scheduler":
			if pgm.schedulerTool != nil {
				var params models.SchedulerToolParams
				if err := json.Unmarshal(tc.Function.Arguments, &params); err != nil {
					slog.Error("PromptGeneratorModule: failed to parse scheduler args", "error", err)
				} else {
					if _, err := pgm.schedulerTool.ExecuteScheduler(ctx, participantID, params); err != nil {
						slog.Warn("PromptGeneratorModule: scheduler execution failed", "error", err)
					}
				}
			}

		case "transition_state":
			if pgm.stateTransitionTool != nil {
				var toolArgs map[string]interface{}
				if err := json.Unmarshal(tc.Function.Arguments, &toolArgs); err != nil {
					slog.Error("PromptGeneratorModule: failed to parse state transition args", "error", err)
				} else {
					if _, err := pgm.stateTransitionTool.ExecuteStateTransition(ctx, participantID, toolArgs); err != nil {
						slog.Warn("PromptGeneratorModule: state transition failed", "error", err)
					}
				}
			}

		case "save_user_profile":
			if pgm.profileSaveTool != nil {
				var toolArgs map[string]interface{}
				if err := json.Unmarshal(tc.Function.Arguments, &toolArgs); err != nil {
					slog.Error("PromptGeneratorModule: failed to parse profile save args", "error", err)
				} else {
					if _, err := pgm.profileSaveTool.ExecuteProfileSave(ctx, participantID, toolArgs); err != nil {
						slog.Warn("PromptGeneratorModule: profile save failed", "error", err)
					}
				}
			}

		default:
			slog.Debug("PromptGeneratorModule: unknown tool call ignored", "name", tc.Function.Name)
		}
	}

	if currentContent == "" {
		currentContent = "I’ve prepared a habit prompt for you."
	}
	return currentContent, nil
}
