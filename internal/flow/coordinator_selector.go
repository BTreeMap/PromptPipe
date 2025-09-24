// Package flow provides a small selector utility to choose between coordinator implementations.
package flow

import "log/slog"

// CoordinatorChoice determines which coordinator to use.
type CoordinatorChoice string

const (
	CoordinatorChoiceLLM    CoordinatorChoice = "llm"
	CoordinatorChoiceStatic CoordinatorChoice = "static"
)

// NewCoordinator selects and constructs a coordinator implementation without changing
// existing call sites. Default is LLM if choice unrecognized or nil dependencies.
func NewCoordinator(choice CoordinatorChoice, stateManager StateManager, genaiClient any, msgService MessagingService, systemPromptFile string, schedulerTool *SchedulerTool, promptGeneratorTool *PromptGeneratorTool, stateTransitionTool *StateTransitionTool, profileSaveTool *ProfileSaveTool) Coordinator {
	switch choice {
	case CoordinatorChoiceStatic:
		slog.Info("Coordinator selector: using static coordinator")
		return NewStaticCoordinatorModule(stateManager, msgService, schedulerTool, promptGeneratorTool, stateTransitionTool, profileSaveTool)
	case CoordinatorChoiceLLM:
		fallthrough
	default:
		slog.Info("Coordinator selector: using LLM coordinator")
		return NewCoordinatorModule(stateManager, nil, msgService, systemPromptFile, schedulerTool, promptGeneratorTool, stateTransitionTool, profileSaveTool)
	}
}
