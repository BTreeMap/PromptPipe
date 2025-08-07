package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Mock timer for testing
type MockTimer struct {
	scheduledCalls []ScheduledCall
}

type ScheduledCall struct {
	Schedule *models.Schedule
	Delay    *time.Duration
	When     *time.Time
	Fn       func()
}

func (m *MockTimer) ScheduleAfter(delay time.Duration, fn func()) (string, error) {
	m.scheduledCalls = append(m.scheduledCalls, ScheduledCall{
		Delay: &delay,
		Fn:    fn,
	})
	return "mock-timer-id", nil
}

func (m *MockTimer) ScheduleAt(when time.Time, fn func()) (string, error) {
	m.scheduledCalls = append(m.scheduledCalls, ScheduledCall{
		When: &when,
		Fn:   fn,
	})
	return "mock-timer-id", nil
}

func (m *MockTimer) ScheduleWithSchedule(schedule *models.Schedule, fn func()) (string, error) {
	m.scheduledCalls = append(m.scheduledCalls, ScheduledCall{
		Schedule: schedule,
		Fn:       fn,
	})
	return "mock-timer-id", nil
}

func (m *MockTimer) Cancel(id string) error {
	return nil
}

func (m *MockTimer) Stop() {}

func (m *MockTimer) ListActive() []models.TimerInfo {
	return nil
}

func (m *MockTimer) GetTimer(id string) (*models.TimerInfo, error) {
	return nil, nil
}

// Mock messaging service for testing
type MockMessagingService struct{}

func (m *MockMessagingService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	// Simple validation - assume phone number format
	return recipient, nil
}

func (m *MockMessagingService) SendMessage(ctx context.Context, to, message string) error {
	// Mock sending - just log the operation
	slog.Debug("MockMessagingService.SendMessage: sending mock message", "to", to, "messageLength", len(message))
	return nil
}

func TestSchedulerTool_GetToolDefinition(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	tool := NewSchedulerTool(timer, msgService)

	definition := tool.GetToolDefinition()

	if definition.Type != "function" {
		t.Errorf("Expected tool type 'function', got %s", definition.Type)
	}

	if definition.Function.Name != "scheduler" {
		t.Errorf("Expected function name 'scheduler', got %s", definition.Function.Name)
	}

	// Check that required parameters are present
	params := definition.Function.Parameters
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be map[string]interface{}")
	}

	requiredFields := []string{"action"}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("Expected required to be []string")
	}

	for _, field := range requiredFields {
		found := false
		for _, req := range required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required field %s not found in required list", field)
		}

		if _, exists := properties[field]; !exists {
			t.Errorf("Required field %s not found in properties", field)
		}
	}
}

func TestSchedulerTool_ExecuteScheduler_CreateFixed(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := NewSchedulerToolWithStateManager(timer, msgService, nil, stateManager)

	// Add phone number to context as the scheduler tool expects it
	ctx := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	participantID := "test-participant"

	params := models.SchedulerToolParams{
		Action:             models.SchedulerActionCreate,
		Type:               models.SchedulerTypeFixed,
		FixedTime:          "09:30",
		Timezone:           "America/Toronto",
		PromptSystemPrompt: "You are a helpful habit coach",
		PromptUserPrompt:   "Time for your daily habit reminder!",
		HabitDescription:   "5-minute morning meditation",
	}

	result, err := tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if result.Message == "" {
		t.Error("Expected non-empty success message")
	}

	// Check that a schedule was created
	if len(timer.scheduledCalls) != 1 {
		t.Errorf("Expected 1 scheduled call, got %d", len(timer.scheduledCalls))
	} else {
		// Should schedule at 9:30 AM (minute=30, hour=9)
		schedule := timer.scheduledCalls[0].Schedule
		if schedule == nil {
			t.Error("Expected non-nil schedule")
		} else if schedule.Hour == nil || *schedule.Hour != 9 {
			t.Errorf("Expected hour=9, got %v", schedule.Hour)
		} else if schedule.Minute == nil || *schedule.Minute != 30 {
			t.Errorf("Expected minute=30, got %v", schedule.Minute)
		}
	}
}

func TestSchedulerTool_ExecuteScheduler_CreateRandom(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := NewSchedulerToolWithStateManager(timer, msgService, nil, stateManager)

	// Add phone number to context as the scheduler tool expects it
	ctx := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	participantID := "test-participant"

	params := models.SchedulerToolParams{
		Action:             models.SchedulerActionCreate,
		Type:               models.SchedulerTypeRandom,
		RandomStartTime:    "08:00",
		RandomEndTime:      "10:00",
		Timezone:           "UTC",
		PromptSystemPrompt: "You are a helpful habit coach",
		PromptUserPrompt:   "Time for your daily habit reminder!",
		HabitDescription:   "1-minute stretching break",
	}

	result, err := tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if result.Message == "" {
		t.Error("Expected non-empty success message")
	}

	// Check that a cron job was scheduled
	if len(timer.scheduledCalls) != 1 {
		t.Errorf("Expected 1 scheduled call, got %d", len(timer.scheduledCalls))
	}
}

func TestSchedulerTool_ExecuteScheduler_InvalidParams(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	tool := NewSchedulerTool(timer, msgService)

	// Add phone number to context so we can test parameter validation specifically
	ctx := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	participantID := "test-participant"

	// Test with missing required fields
	params := models.SchedulerToolParams{
		Action: models.SchedulerActionCreate,
		Type:   models.SchedulerTypeFixed,
		// Missing FixedTime, PromptSystemPrompt, PromptUserPrompt
	}

	result, err := tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected success=false for invalid params")
	}

	if result.Error == "" {
		t.Error("Expected non-empty error message for invalid params")
	}
}

func TestSchedulerToolParams_Validate(t *testing.T) {
	tests := []struct {
		name    string
		params  models.SchedulerToolParams
		wantErr bool
	}{
		{
			name: "Valid create fixed params",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				Type:               models.SchedulerTypeFixed,
				FixedTime:          "09:30",
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: false,
		},
		{
			name: "Valid create random params",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				Type:               models.SchedulerTypeRandom,
				RandomStartTime:    "08:00",
				RandomEndTime:      "10:00",
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: false,
		},
		{
			name: "Valid list params",
			params: models.SchedulerToolParams{
				Action: models.SchedulerActionList,
			},
			wantErr: false,
		},
		{
			name: "Valid delete params",
			params: models.SchedulerToolParams{
				Action:     models.SchedulerActionDelete,
				ScheduleID: "some-schedule-id",
			},
			wantErr: false,
		},
		{
			name: "Missing action",
			params: models.SchedulerToolParams{
				Type:               models.SchedulerTypeFixed,
				FixedTime:          "09:30",
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: true,
		},
		{
			name: "Invalid action",
			params: models.SchedulerToolParams{
				Action: "invalid",
			},
			wantErr: true,
		},
		{
			name: "Create missing type",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: true,
		},
		{
			name: "Create fixed missing fixed_time",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				Type:               models.SchedulerTypeFixed,
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: true,
		},
		{
			name: "Create random missing start time",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				Type:               models.SchedulerTypeRandom,
				RandomEndTime:      "10:00",
				PromptSystemPrompt: "System prompt",
				PromptUserPrompt:   "User prompt",
			},
			wantErr: true,
		},
		{
			name: "Create missing system prompt",
			params: models.SchedulerToolParams{
				Action:           models.SchedulerActionCreate,
				Type:             models.SchedulerTypeFixed,
				FixedTime:        "09:30",
				PromptUserPrompt: "User prompt",
			},
			wantErr: true,
		},
		{
			name: "Create missing user prompt",
			params: models.SchedulerToolParams{
				Action:             models.SchedulerActionCreate,
				Type:               models.SchedulerTypeFixed,
				FixedTime:          "09:30",
				PromptSystemPrompt: "System prompt",
			},
			wantErr: true,
		},
		{
			name: "Delete missing schedule ID",
			params: models.SchedulerToolParams{
				Action: models.SchedulerActionDelete,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SchedulerToolParams.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToolCall_ParseSchedulerParams(t *testing.T) {
	// Test valid scheduler parameters
	params := models.SchedulerToolParams{
		Action:             models.SchedulerActionCreate,
		Type:               models.SchedulerTypeFixed,
		FixedTime:          "09:30",
		Timezone:           "America/Toronto",
		PromptSystemPrompt: "You are a helpful habit coach",
		PromptUserPrompt:   "Time for your daily habit!",
		HabitDescription:   "5-minute meditation",
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal params: %v", err)
	}

	fc := models.FunctionCall{
		Name:      "scheduler",
		Arguments: json.RawMessage(paramsJSON),
	}

	parsed, err := fc.ParseSchedulerParams()
	if err != nil {
		t.Fatalf("ParseSchedulerParams failed: %v", err)
	}

	if parsed.Type != params.Type {
		t.Errorf("Expected type %s, got %s", params.Type, parsed.Type)
	}
	if parsed.Action != params.Action {
		t.Errorf("Expected action %s, got %s", params.Action, parsed.Action)
	}
	if parsed.FixedTime != params.FixedTime {
		t.Errorf("Expected fixed_time %s, got %s", params.FixedTime, parsed.FixedTime)
	}
	if parsed.PromptSystemPrompt != params.PromptSystemPrompt {
		t.Errorf("Expected system prompt %s, got %s", params.PromptSystemPrompt, parsed.PromptSystemPrompt)
	}

	// Test with wrong function name
	fc.Name = "wrong_function"
	_, err = fc.ParseSchedulerParams()
	if err == nil {
		t.Error("Expected error for wrong function name")
	}

	// Test with invalid JSON
	fc.Name = "scheduler"
	fc.Arguments = json.RawMessage(`{"invalid": json}`)
	_, err = fc.ParseSchedulerParams()
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestSchedulerTool_ExecuteScheduler_List(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := NewSchedulerToolWithStateManager(timer, msgService, nil, stateManager)

	ctx := context.Background()
	participantID := "test-participant"

	// Test empty list
	params := models.SchedulerToolParams{
		Action: models.SchedulerActionList,
	}

	result, err := tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler list failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if !strings.Contains(result.Message, "don't have any active schedules") {
		t.Errorf("Expected empty schedules message, got: %s", result.Message)
	}

	// Create a schedule first
	createParams := models.SchedulerToolParams{
		Action:             models.SchedulerActionCreate,
		Type:               models.SchedulerTypeFixed,
		FixedTime:          "09:30",
		Timezone:           "America/Toronto",
		PromptSystemPrompt: "You are a helpful habit coach",
		PromptUserPrompt:   "Time for your daily habit reminder!",
		HabitDescription:   "morning meditation",
	}

	// Add phone number to context for create
	ctxWithPhone := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	_, err = tool.ExecuteScheduler(ctxWithPhone, participantID, createParams)
	if err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Now test list with content
	result, err = tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler list failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if !strings.Contains(result.Message, "Your active schedules") {
		t.Errorf("Expected schedules list message, got: %s", result.Message)
	}

	if !strings.Contains(result.Message, "morning meditation") {
		t.Errorf("Expected habit description in message, got: %s", result.Message)
	}
}

func TestSchedulerTool_ExecuteScheduler_Delete(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := NewSchedulerToolWithStateManager(timer, msgService, nil, stateManager)

	ctx := context.Background()
	participantID := "test-participant"

	// Test delete non-existent schedule
	params := models.SchedulerToolParams{
		Action:     models.SchedulerActionDelete,
		ScheduleID: "non-existent-id",
	}

	result, err := tool.ExecuteScheduler(ctx, participantID, params)
	if err != nil {
		t.Fatalf("ExecuteScheduler delete failed: %v", err)
	}

	if result.Success {
		t.Errorf("Expected success=false for non-existent schedule, got %v", result.Success)
	}

	if !strings.Contains(result.Message, "not found") {
		t.Errorf("Expected 'not found' message, got: %s", result.Message)
	}

	// Create a schedule first
	createParams := models.SchedulerToolParams{
		Action:             models.SchedulerActionCreate,
		Type:               models.SchedulerTypeFixed,
		FixedTime:          "09:30",
		Timezone:           "America/Toronto",
		PromptSystemPrompt: "You are a helpful habit coach",
		PromptUserPrompt:   "Time for your daily habit reminder!",
		HabitDescription:   "morning meditation",
	}

	// Add phone number to context for create
	ctxWithPhone := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	createResult, err := tool.ExecuteScheduler(ctxWithPhone, participantID, createParams)
	if err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Extract schedule ID from message (this is a bit hacky for testing)
	if !strings.Contains(createResult.Message, "Schedule ID:") {
		t.Fatalf("Create result should contain schedule ID")
	}

	// Get the stored schedules to find the ID
	schedules, err := tool.getStoredSchedules(ctx, participantID)
	if err != nil || len(schedules) == 0 {
		t.Fatalf("Failed to get created schedule")
	}

	scheduleID := schedules[0].ID

	// Now test successful delete
	deleteParams := models.SchedulerToolParams{
		Action:     models.SchedulerActionDelete,
		ScheduleID: scheduleID,
	}

	result, err = tool.ExecuteScheduler(ctx, participantID, deleteParams)
	if err != nil {
		t.Fatalf("ExecuteScheduler delete failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success=true, got %v. Error: %s", result.Success, result.Error)
	}

	if !strings.Contains(result.Message, "Successfully deleted") {
		t.Errorf("Expected success message, got: %s", result.Message)
	}

	// Verify schedule was actually deleted
	schedules, err = tool.getStoredSchedules(ctx, participantID)
	if err != nil {
		t.Fatalf("Failed to get schedules after delete: %v", err)
	}

	if len(schedules) != 0 {
		t.Errorf("Expected 0 schedules after delete, got %d", len(schedules))
	}
}
