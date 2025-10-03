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
	cancelledIDs   []string
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
	m.cancelledIDs = append(m.cancelledIDs, id)
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
type MockMessage struct {
	To      string
	Message string
}

type MockMessagingService struct {
	sentMessages []MockMessage
}

func (m *MockMessagingService) ValidateAndCanonicalizeRecipient(recipient string) (string, error) {
	// Simple validation - assume phone number format
	return recipient, nil
}

func (m *MockMessagingService) SendMessage(ctx context.Context, to, message string) error {
	// Mock sending - just log the operation
	slog.Debug("MockMessagingService.SendMessage: sending mock message", "to", to, "messageLength", len(message))
	m.sentMessages = append(m.sentMessages, MockMessage{To: to, Message: message})
	return nil
}

func (m *MockMessagingService) SendTypingIndicator(ctx context.Context, to string, typing bool) error {
	slog.Debug("MockMessagingService.SendTypingIndicator", "to", to, "typing", typing)
	return nil
}

func newSchedulerToolForTest(timer models.Timer, msgService MessagingService, stateManager StateManager) *SchedulerTool {
	return NewSchedulerTool(timer, msgService, nil, stateManager, nil, 10, true)
}

func TestSchedulerTool_GetToolDefinition(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	tool := newSchedulerToolForTest(timer, msgService, nil)

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
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

	// Add phone number to context as the scheduler tool expects it
	ctx := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	participantID := "test-participant"

	params := models.SchedulerToolParams{
		Action:    models.SchedulerActionCreate,
		Type:      models.SchedulerTypeFixed,
		FixedTime: "09:30",
		Timezone:  "America/Toronto",
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

	// New behavior: if the prep notification for today hasn't passed yet, we schedule TWO things:
	// 1. A same-day one-off delayed timer (ScheduleAfter with Delay set)
	// 2. The recurring daily schedule (ScheduleWithSchedule with Schedule set)
	// Depending on current clock when the test runs, #1 may or may not be present. We must always
	// have exactly one recurring schedule, and optionally one delayed call.
	recurringCount := 0
	var recurringSchedule *models.Schedule
	delayCount := 0
	for _, c := range timer.scheduledCalls {
		if c.Schedule != nil {
			recurringCount++
			recurringSchedule = c.Schedule
		}
		if c.Delay != nil {
			delayCount++
		}
	}
	if recurringCount != 1 {
		t.Errorf("Expected exactly 1 recurring schedule, got %d (total calls %d)", recurringCount, len(timer.scheduledCalls))
	}
	if recurringSchedule != nil { // Validate prep time hour/minute
		if recurringSchedule.Hour == nil || *recurringSchedule.Hour != 9 {
			t.Errorf("Expected recurring schedule hour=9, got %v", recurringSchedule.Hour)
		}
		if recurringSchedule.Minute == nil || *recurringSchedule.Minute != 20 {
			t.Errorf("Expected recurring schedule minute=20 (prep time), got %v", recurringSchedule.Minute)
		}
	}
	// Allow delayCount to be 0 or 1; just ensure no more than 1
	if delayCount > 1 {
		t.Errorf("Expected at most 1 same-day delay timer, got %d", delayCount)
	}
}

func TestSchedulerTool_ExecuteScheduler_CreateRandom(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

	// Add phone number to context as the scheduler tool expects it
	ctx := context.WithValue(context.Background(), phoneNumberContextKey, "+1234567890")
	participantID := "test-participant"

	params := models.SchedulerToolParams{
		Action:          models.SchedulerActionCreate,
		Type:            models.SchedulerTypeRandom,
		RandomStartTime: "08:00",
		RandomEndTime:   "10:00",
		Timezone:        "UTC",
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

	// Similar dual-scheduling behavior applies here. Validate exactly one recurring schedule.
	recurringCount := 0
	var recurringSchedule *models.Schedule
	delayCount := 0
	for _, c := range timer.scheduledCalls {
		if c.Schedule != nil {
			recurringCount++
			recurringSchedule = c.Schedule
		}
		if c.Delay != nil {
			delayCount++
		}
	}
	if recurringCount != 1 {
		t.Errorf("Expected exactly 1 recurring schedule, got %d (total calls %d)", recurringCount, len(timer.scheduledCalls))
	}
	if delayCount > 1 {
		t.Errorf("Expected at most 1 same-day delay timer, got %d", delayCount)
	}
	// For random window start 08:00, prep notification should be at 07:50
	if recurringSchedule != nil {
		if recurringSchedule.Hour == nil || *recurringSchedule.Hour != 7 {
			t.Errorf("Expected recurring schedule hour=7 (prep for 08:00), got %v", recurringSchedule.Hour)
		}
		if recurringSchedule.Minute == nil || *recurringSchedule.Minute != 50 {
			t.Errorf("Expected recurring schedule minute=50 (prep time), got %v", recurringSchedule.Minute)
		}
	}
}

func TestSchedulerTool_SchedulesDailyPromptReminder(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

	ctx := context.Background()
	participantID := "reminder-test"
	prompt := models.Prompt{To: "+1234567890", Type: models.PromptTypeStatic, Body: "Daily prompt"}

	tool.executeScheduledPrompt(ctx, participantID, prompt)

	if len(msgService.sentMessages) != 1 {
		t.Fatalf("expected exactly one message sent (the prompt), got %d", len(msgService.sentMessages))
	}

	if len(timer.scheduledCalls) == 0 {
		t.Fatal("expected reminder timer to be scheduled")
	}

	foundReminder := false
	for _, call := range timer.scheduledCalls {
		if call.Delay != nil && *call.Delay == tool.dailyPromptReminderDelay {
			foundReminder = true
		}
	}

	if !foundReminder {
		t.Fatalf("expected reminder scheduled with delay %s", tool.dailyPromptReminderDelay)
	}

	pendingJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if err != nil {
		t.Fatalf("unexpected error loading pending state: %v", err)
	}
	if strings.TrimSpace(pendingJSON) == "" {
		t.Fatal("expected pending reminder state to be stored")
	}
}

func TestSchedulerTool_DailyPromptReminderClearedOnReply(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

	ctx := context.Background()
	participantID := "reminder-reply"
	prompt := models.Prompt{To: "+15550000000", Type: models.PromptTypeStatic, Body: "Daily prompt"}

	tool.executeScheduledPrompt(ctx, participantID, prompt)

	pendingJSON, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if err != nil {
		t.Fatalf("unexpected error loading pending state: %v", err)
	}
	if strings.TrimSpace(pendingJSON) == "" {
		t.Fatal("expected pending reminder state to exist")
	}

	var pending dailyPromptPendingState
	if err := json.Unmarshal([]byte(pendingJSON), &pending); err != nil {
		t.Fatalf("failed to parse pending state: %v", err)
	}

	sentAt, err := time.Parse(time.RFC3339, pending.SentAt)
	if err != nil {
		t.Fatalf("failed to parse sent timestamp: %v", err)
	}
	replyAt := sentAt.Add(10 * time.Minute)

	tool.handleDailyPromptReply(ctx, participantID, replyAt)

	clearedPending, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if strings.TrimSpace(clearedPending) != "" {
		t.Error("expected pending reminder state to be cleared after reply")
	}

	clearedTimerID, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptReminderTimerID)
	if strings.TrimSpace(clearedTimerID) != "" {
		t.Error("expected reminder timer ID to be cleared after reply")
	}

	if len(timer.cancelledIDs) == 0 {
		t.Error("expected reminder timer to be cancelled after reply")
	}
}

func TestSchedulerTool_DailyPromptReminderSendsWhenNoReply(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := newSchedulerToolForTest(timer, msgService, stateManager)
	tool.dailyPromptReminderDelay = time.Second

	ctx := context.Background()
	participantID := "reminder-no-reply"
	prompt := models.Prompt{To: "+16660000000", Type: models.PromptTypeStatic, Body: "Daily prompt"}

	tool.executeScheduledPrompt(ctx, participantID, prompt)

	if len(timer.scheduledCalls) == 0 {
		t.Fatal("expected reminder timer to be scheduled")
	}

	if len(msgService.sentMessages) != 1 {
		t.Fatalf("expected exactly one message sent before reminder, got %d", len(msgService.sentMessages))
	}

	var reminderCall *ScheduledCall
	for i := range timer.scheduledCalls {
		call := &timer.scheduledCalls[i]
		if call.Delay != nil && *call.Delay == tool.dailyPromptReminderDelay {
			reminderCall = call
			break
		}
	}

	if reminderCall == nil {
		t.Fatalf("expected reminder scheduled with delay %s", tool.dailyPromptReminderDelay)
	}

	if reminderCall.Fn == nil {
		t.Fatal("expected reminder timer to have callback")
	}

	reminderCall.Fn()

	if len(msgService.sentMessages) != 2 {
		t.Fatalf("expected reminder message to be sent, got %d messages", len(msgService.sentMessages))
	}

	reminder := msgService.sentMessages[len(msgService.sentMessages)-1]
	if !strings.Contains(reminder.Message, "haven't heard back") {
		t.Errorf("unexpected reminder message: %s", reminder.Message)
	}

	pendingJSON, _ := stateManager.GetStateData(ctx, participantID, models.FlowTypeConversation, models.DataKeyDailyPromptPending)
	if strings.TrimSpace(pendingJSON) != "" {
		t.Error("expected pending reminder state to be cleared after sending reminder")
	}
}

func TestSchedulerTool_ExecuteScheduler_InvalidParams(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	tool := newSchedulerToolForTest(timer, msgService, nil)

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
				Action:    models.SchedulerActionCreate,
				Type:      models.SchedulerTypeFixed,
				FixedTime: "09:30",
			},
			wantErr: false,
		},
		{
			name: "Valid create random params",
			params: models.SchedulerToolParams{
				Action:          models.SchedulerActionCreate,
				Type:            models.SchedulerTypeRandom,
				RandomStartTime: "08:00",
				RandomEndTime:   "10:00",
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
				Type:      models.SchedulerTypeFixed,
				FixedTime: "09:30",
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
				Action: models.SchedulerActionCreate,
			},
			wantErr: true,
		},
		{
			name: "Create fixed missing fixed_time",
			params: models.SchedulerToolParams{
				Action: models.SchedulerActionCreate,
				Type:   models.SchedulerTypeFixed,
			},
			wantErr: true,
		},
		{
			name: "Create random missing start time",
			params: models.SchedulerToolParams{
				Action:        models.SchedulerActionCreate,
				Type:          models.SchedulerTypeRandom,
				RandomEndTime: "10:00",
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
		Action:    models.SchedulerActionCreate,
		Type:      models.SchedulerTypeFixed,
		FixedTime: "09:30",
		Timezone:  "America/Toronto",
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
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

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
		Action:    models.SchedulerActionCreate,
		Type:      models.SchedulerTypeFixed,
		FixedTime: "09:30",
		Timezone:  "America/Toronto",
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
}

func TestSchedulerTool_ExecuteScheduler_Delete(t *testing.T) {
	timer := &MockTimer{}
	msgService := &MockMessagingService{}
	stateManager := NewMockStateManager()
	tool := newSchedulerToolForTest(timer, msgService, stateManager)

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
		Action:    models.SchedulerActionCreate,
		Type:      models.SchedulerTypeFixed,
		FixedTime: "09:30",
		Timezone:  "America/Toronto",
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
