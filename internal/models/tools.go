// Package models defines tool structures for LLM function calling.
package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// ToolType defines the type of tool available to the LLM.
type ToolType string

const (
	// ToolTypeScheduler allows the LLM to schedule daily prompts for users.
	ToolTypeScheduler ToolType = "scheduler"
	// ToolTypeOneMinuteIntervention allows the LLM to initiate one-minute health interventions.
	ToolTypeOneMinuteIntervention ToolType = "one_minute_intervention"
)

// SchedulerType defines how the scheduler should send daily prompts.
type SchedulerType string

const (
	// SchedulerTypeFixed sends prompts at a fixed time each day.
	SchedulerTypeFixed SchedulerType = "fixed"
	// SchedulerTypeRandom sends prompts at a random time within a time window each day.
	SchedulerTypeRandom SchedulerType = "random"
)

// SchedulerToolParams defines the parameters for the scheduler tool call.
type SchedulerToolParams struct {
	Type               SchedulerType `json:"type"`                        // "fixed" or "random"
	FixedTime          string        `json:"fixed_time,omitempty"`        // Time in HH:MM format (e.g., "09:30")
	Timezone           string        `json:"timezone,omitempty"`          // Timezone (e.g., "America/Toronto")
	RandomStartTime    string        `json:"random_start_time,omitempty"` // Start of random window in HH:MM format
	RandomEndTime      string        `json:"random_end_time,omitempty"`   // End of random window in HH:MM format
	PromptSystemPrompt string        `json:"prompt_system_prompt"`        // System prompt for the daily messages
	PromptUserPrompt   string        `json:"prompt_user_prompt"`          // User prompt for the daily messages
	HabitDescription   string        `json:"habit_description,omitempty"` // Description of the habit for personalization
}

// Validate ensures the scheduler tool parameters are valid.
func (stp *SchedulerToolParams) Validate() error {
	if stp.Type != SchedulerTypeFixed && stp.Type != SchedulerTypeRandom {
		return fmt.Errorf("invalid scheduler type: %s", stp.Type)
	}

	if stp.Type == SchedulerTypeFixed {
		if stp.FixedTime == "" {
			return fmt.Errorf("fixed_time is required for fixed scheduler type")
		}
		if err := validateTimeFormat(stp.FixedTime); err != nil {
			return fmt.Errorf("invalid fixed_time format: %w", err)
		}
	}

	if stp.Type == SchedulerTypeRandom {
		if stp.RandomStartTime == "" {
			return fmt.Errorf("random_start_time is required for random scheduler type")
		}
		if stp.RandomEndTime == "" {
			return fmt.Errorf("random_end_time is required for random scheduler type")
		}
		if err := validateTimeFormat(stp.RandomStartTime); err != nil {
			return fmt.Errorf("invalid random_start_time format: %w", err)
		}
		if err := validateTimeFormat(stp.RandomEndTime); err != nil {
			return fmt.Errorf("invalid random_end_time format: %w", err)
		}

		// Validate that start time is before end time
		startTime, _ := time.Parse("15:04", stp.RandomStartTime)
		endTime, _ := time.Parse("15:04", stp.RandomEndTime)
		if !endTime.After(startTime) {
			return fmt.Errorf("random_end_time must be after random_start_time")
		}
	}

	if stp.PromptSystemPrompt == "" {
		return fmt.Errorf("prompt_system_prompt is required")
	}
	if stp.PromptUserPrompt == "" {
		return fmt.Errorf("prompt_user_prompt is required")
	}

	return nil
}

// validateTimeFormat validates that a time string is in HH:MM format (24-hour).
func validateTimeFormat(timeStr string) error {
	timeRegex := regexp.MustCompile(`^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$`)
	if !timeRegex.MatchString(timeStr) {
		return fmt.Errorf("time must be in HH:MM format (24-hour)")
	}

	// Additional validation using time.Parse to ensure it's a valid time
	_, err := time.Parse("15:04", timeStr)
	if err != nil {
		return fmt.Errorf("invalid time: %w", err)
	}

	return nil
}

// ToolCall represents an LLM tool function call.
type ToolCall struct {
	ID       string       `json:"id"`       // Tool call ID from OpenAI
	Type     string       `json:"type"`     // Always "function" for OpenAI
	Function FunctionCall `json:"function"` // Function details
}

// FunctionCall represents the function details within a tool call.
type FunctionCall struct {
	Name      string          `json:"name"`      // Function name (e.g., "scheduler")
	Arguments json.RawMessage `json:"arguments"` // JSON arguments as raw message
}

// ParseSchedulerParams parses the arguments as SchedulerToolParams.
func (fc *FunctionCall) ParseSchedulerParams() (*SchedulerToolParams, error) {
	if fc.Name != string(ToolTypeScheduler) {
		return nil, fmt.Errorf("function name %s is not a scheduler function", fc.Name)
	}

	var params SchedulerToolParams
	if err := json.Unmarshal(fc.Arguments, &params); err != nil {
		return nil, fmt.Errorf("failed to parse scheduler parameters: %w", err)
	}

	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scheduler parameters: %w", err)
	}

	return &params, nil
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`    // ID of the tool call this responds to
	Success    bool   `json:"success"`         // Whether the tool execution succeeded
	Message    string `json:"message"`         // Human-readable result message
	Error      string `json:"error,omitempty"` // Error message if success is false
}

// OneMinuteInterventionToolParams defines the parameters for the intervention tool call.
type OneMinuteInterventionToolParams struct {
	InterventionFocus    string `json:"intervention_focus,omitempty"`    // Focus or type of intervention (flexible)
	PersonalizationNotes string `json:"personalization_notes,omitempty"` // Notes for personalizing the intervention
}

// Validate ensures the intervention tool parameters are valid.
func (omt *OneMinuteInterventionToolParams) Validate() error {
	// All parameters are optional and flexible
	return nil
}
