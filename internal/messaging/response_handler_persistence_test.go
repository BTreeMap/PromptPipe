package messaging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/flow"
	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestResponseHandler_RegisterPersistentHook(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	params := map[string]string{
		"participant_id": "test-participant",
		"phone_number":   "+1234567890",
	}

	err := handler.RegisterPersistentHook("+1234567890", models.HookTypeIntervention, params)
	if err != nil {
		t.Fatalf("Failed to register persistent hook: %v", err)
	}

	// Verify hook was stored in database
	hook, err := st.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("Failed to get registered hook: %v", err)
	}

	if hook == nil {
		t.Fatal("Hook was not stored in database")
	}

	if hook.PhoneNumber != "+1234567890" {
		t.Errorf("Expected phone number '+1234567890', got '%s'", hook.PhoneNumber)
	}

	if hook.HookType != models.HookTypeIntervention {
		t.Errorf("Expected hook type '%s', got '%s'", models.HookTypeIntervention, hook.HookType)
	}

	if hook.Parameters["participant_id"] != "test-participant" {
		t.Errorf("Expected participant_id 'test-participant', got '%s'", hook.Parameters["participant_id"])
	}
}

func TestResponseHandler_UnregisterPersistentHook(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	// First register a hook
	params := map[string]string{
		"participant_id": "test-participant",
		"phone_number":   "+1234567890",
	}

	err := handler.RegisterPersistentHook("+1234567890", models.HookTypeIntervention, params)
	if err != nil {
		t.Fatalf("Failed to register persistent hook: %v", err)
	}

	// Verify hook exists
	hook, err := st.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("Failed to get registered hook: %v", err)
	}
	if hook == nil {
		t.Fatal("Hook was not stored in database")
	}

	// Now unregister it
	err = handler.UnregisterPersistentHook("+1234567890")
	if err != nil {
		t.Fatalf("Failed to unregister persistent hook: %v", err)
	}

	// Verify hook was removed from database
	hook, err = st.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("Failed to check hook removal: %v", err)
	}

	if hook != nil {
		t.Error("Hook should have been removed from database")
	}
}

func TestResponseHandler_RecoverPersistentHooks_Success(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	// Manually store a hook in the database (simulating previous registration)
	hookData := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeIntervention,
		Parameters: map[string]string{
			"participant_id": "test-participant",
			"phone_number":   "+1234567890",
		},
		CreatedAt: time.Now(),
	}

	err := st.SaveRegisteredHook(hookData)
	if err != nil {
		t.Fatalf("Failed to save test hook: %v", err)
	}

	// Recover hooks
	ctx := context.Background()
	err = handler.RecoverPersistentHooks(ctx)
	if err != nil {
		t.Fatalf("Failed to recover persistent hooks: %v", err)
	}

	// Test that the hook is functional by processing a response
	response := models.Response{
		From: "+1234567890",
		Body: "test message",
		Time: time.Now().Unix(),
	}

	err = handler.ProcessResponse(ctx, response)
	if err != nil {
		t.Errorf("Failed to process response: %v", err)
	}

	// We can't easily test if it was "handled" since ProcessResponse doesn't return that info
	// But we can verify the hook was recreated by checking internal state or side effects
}

func TestResponseHandler_RecoverPersistentHooks_FailureWarning(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	// Store a hook with invalid parameters (to simulate hook recreation failure)
	hookData := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeIntervention,
		Parameters:  map[string]string{
			// Missing required parameters to trigger creation failure
		},
		CreatedAt: time.Now(),
	}

	err := st.SaveRegisteredHook(hookData)
	if err != nil {
		t.Fatalf("Failed to save test hook: %v", err)
	}

	// Recover hooks - should not fail even if individual hook recreation fails
	ctx := context.Background()
	err = handler.RecoverPersistentHooks(ctx)
	if err != nil {
		t.Fatalf("RecoverPersistentHooks should not fail even if individual hooks fail: %v", err)
	}

	// The test passes if no error is thrown - warnings are logged but not propagated
}

func TestResponseHandler_RecoverPersistentHooks_UnknownHookType(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	// Store a hook with unknown hook type
	hookData := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookType("unknown_type"),
		Parameters: map[string]string{
			"test": "value",
		},
		CreatedAt: time.Now(),
	}

	err := st.SaveRegisteredHook(hookData)
	if err != nil {
		t.Fatalf("Failed to save test hook: %v", err)
	}

	// Recover hooks - should handle unknown hook types gracefully
	ctx := context.Background()
	err = handler.RecoverPersistentHooks(ctx)
	if err != nil {
		t.Fatalf("RecoverPersistentHooks should handle unknown hook types gracefully: %v", err)
	}
}

func TestResponseHandler_RegisterPersistentHook_UnsupportedType(t *testing.T) {
	st := store.NewInMemoryStore()
	msgService := NewMockService()
	handler := NewResponseHandler(msgService, st)

	// Set up dependencies
	stateManager := flow.NewStoreBasedStateManager(st)
	timer := &MockTimer{}
	handler.SetDependencies(stateManager, timer)

	params := map[string]string{
		"participant_id": "test-participant",
		"flow_type":      "test-flow",
		"branches":       "branch1,branch2",
	}

	// Try to register a branch hook (not supported for persistence)
	err := handler.RegisterPersistentHook("+1234567890", models.HookTypeBranch, params)
	if err == nil {
		t.Fatal("Expected error for unsupported hook type, got nil")
	}

	expectedError := "branch hooks are not supported for persistence"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}

	// Verify hook was not stored
	hook, err := st.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("Failed to check hook storage: %v", err)
	}

	if hook != nil {
		t.Error("Hook should not have been stored for unsupported type")
	}
}
