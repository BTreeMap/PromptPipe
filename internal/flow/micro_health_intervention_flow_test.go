package flow

import (
	"context"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestMicroHealthInterventionFlow_CompleteHappyPath(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	timer := NewSimpleTimer()

	// Create generator with dependencies
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)

	participantID := "test_participant_001"

	// Test flow: Orientation -> Commitment -> Feeling -> Random Assignment -> Intervention -> Done -> Reinforcement

	// 1. Start with orientation
	err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateOrientation)
	if err != nil {
		t.Fatalf("Failed to set initial state: %v", err)
	}

	// Generate orientation message
	prompt := models.Prompt{State: models.StateOrientation, To: participantID}
	msg, err := generator.Generate(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate orientation message: %v", err)
	}
	if msg != MsgOrientation {
		t.Errorf("Expected orientation message, got: %s", msg)
	}

	// 2. Process orientation response (moves to commitment)
	err = generator.ProcessResponse(ctx, participantID, "ready")
	if err != nil {
		t.Fatalf("Failed to process orientation response: %v", err)
	}

	// Verify state transition
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateCommitmentPrompt {
		t.Errorf("Expected state %s, got %s", models.StateCommitmentPrompt, currentState)
	}

	// 3. Generate commitment message
	prompt.State = models.StateCommitmentPrompt
	msg, err = generator.Generate(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate commitment message: %v", err)
	}
	if msg != MsgCommitment {
		t.Errorf("Expected commitment message, got: %s", msg)
	}

	// 4. Process positive commitment response
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process commitment response: %v", err)
	}

	// Verify transition to feeling prompt
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateFeelingPrompt {
		t.Errorf("Expected state %s, got %s", models.StateFeelingPrompt, currentState)
	}

	// 5. Generate feeling message
	prompt.State = models.StateFeelingPrompt
	msg, err = generator.Generate(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate feeling message: %v", err)
	}
	if msg != MsgFeeling {
		t.Errorf("Expected feeling message, got: %s", msg)
	}

	// 6. Process feeling response
	err = generator.ProcessResponse(ctx, participantID, "3")
	if err != nil {
		t.Fatalf("Failed to process feeling response: %v", err)
	}

	// Verify transition to intervention (should be either immediate or reflective)
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateSendInterventionImmediate && currentState != models.StateSendInterventionReflective {
		t.Errorf("Expected intervention state, got %s", currentState)
	}

	// 7. Generate intervention message
	prompt.State = currentState
	msg, err = generator.Generate(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate intervention message: %v", err)
	}
	expectedMsg := MsgImmediateIntervention
	if currentState == models.StateSendInterventionReflective {
		expectedMsg = MsgReflectiveIntervention
	}
	if msg != expectedMsg {
		t.Errorf("Expected intervention message %s, got: %s", expectedMsg, msg)
	}

	// 8. Process completion response (done)
	err = generator.ProcessResponse(ctx, participantID, "done")
	if err != nil {
		t.Fatalf("Failed to process completion response: %v", err)
	}

	// Verify transition to reinforcement
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateReinforcementFollowup {
		t.Errorf("Expected state %s, got %s", models.StateReinforcementFollowup, currentState)
	}

	// 9. Generate reinforcement message
	prompt.State = models.StateReinforcementFollowup
	msg, err = generator.Generate(ctx, prompt)
	if err != nil {
		t.Fatalf("Failed to generate reinforcement message: %v", err)
	}
	if msg != MsgReinforcement {
		t.Errorf("Expected reinforcement message, got: %s", msg)
	}
}

func TestMicroHealthInterventionFlow_NoPath(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	timer := NewSimpleTimer()

	// Create generator with dependencies
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)

	participantID := "test_participant_002"

	// Test flow: Commitment -> Feeling -> Intervention -> No -> Did you get a chance -> No -> Barrier Reason

	// 1. Start with commitment prompt
	err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateCommitmentPrompt)
	if err != nil {
		t.Fatalf("Failed to set initial state: %v", err)
	}

	// 2. Process positive commitment response
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process commitment response: %v", err)
	}

	// 3. Process feeling response
	err = generator.ProcessResponse(ctx, participantID, "2")
	if err != nil {
		t.Fatalf("Failed to process feeling response: %v", err)
	}

	// 4. Process completion response (no)
	err = generator.ProcessResponse(ctx, participantID, "no")
	if err != nil {
		t.Fatalf("Failed to process completion response: %v", err)
	}

	// Verify transition to "did you get a chance"
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateDidYouGetAChance {
		t.Errorf("Expected state %s, got %s", models.StateDidYouGetAChance, currentState)
	}

	// 5. Process "did you get a chance" response (no)
	err = generator.ProcessResponse(ctx, participantID, "2")
	if err != nil {
		t.Fatalf("Failed to process did you get a chance response: %v", err)
	}

	// Verify transition to barrier reason
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateBarrierReasonNoChance {
		t.Errorf("Expected state %s, got %s", models.StateBarrierReasonNoChance, currentState)
	}

	// 6. Process barrier reason response
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process barrier reason response: %v", err)
	}

	// Verify transition to end of day
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateEndOfDay {
		t.Errorf("Expected state %s, got %s", models.StateEndOfDay, currentState)
	}
}

func TestMicroHealthInterventionFlow_ContextMoodPath(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	timer := NewSimpleTimer()

	// Create generator with dependencies
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)

	participantID := "test_participant_003"

	// Test flow through context and mood questions

	// Start at "did you get a chance" state and say yes
	err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateDidYouGetAChance)
	if err != nil {
		t.Fatalf("Failed to set initial state: %v", err)
	}

	// Process "did you get a chance" response (yes)
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process did you get a chance response: %v", err)
	}

	// Verify transition to context question
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateContextQuestion {
		t.Errorf("Expected state %s, got %s", models.StateContextQuestion, currentState)
	}

	// Process context response
	err = generator.ProcessResponse(ctx, participantID, "2")
	if err != nil {
		t.Fatalf("Failed to process context response: %v", err)
	}

	// Verify transition to mood question
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateMoodQuestion {
		t.Errorf("Expected state %s, got %s", models.StateMoodQuestion, currentState)
	}

	// Process mood response
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process mood response: %v", err)
	}

	// Verify transition to barrier check
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateBarrierCheckAfterContextMood {
		t.Errorf("Expected state %s, got %s", models.StateBarrierCheckAfterContextMood, currentState)
	}

	// Process barrier detail response (free text)
	err = generator.ProcessResponse(ctx, participantID, "It was easy because I was at home")
	if err != nil {
		t.Fatalf("Failed to process barrier detail response: %v", err)
	}

	// Verify transition to end of day
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateEndOfDay {
		t.Errorf("Expected state %s, got %s", models.StateEndOfDay, currentState)
	}
}

func TestMicroHealthInterventionFlow_InvalidResponses(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	timer := NewSimpleTimer()

	// Create generator with dependencies
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)

	participantID := "test_participant_004"

	// Test invalid responses don't cause state transitions

	// Start with commitment prompt
	err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateCommitmentPrompt)
	if err != nil {
		t.Fatalf("Failed to set initial state: %v", err)
	}

	// Process invalid commitment response
	err = generator.ProcessResponse(ctx, participantID, "invalid")
	if err != nil {
		t.Fatalf("Failed to process invalid response: %v", err)
	}

	// Verify state didn't change
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateCommitmentPrompt {
		t.Errorf("State should not have changed from %s, got %s", models.StateCommitmentPrompt, currentState)
	}

	// Now send valid response
	err = generator.ProcessResponse(ctx, participantID, "1")
	if err != nil {
		t.Fatalf("Failed to process valid response: %v", err)
	}

	// Verify state changed
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateFeelingPrompt {
		t.Errorf("Expected state %s, got %s", models.StateFeelingPrompt, currentState)
	}
}

func TestMicroHealthInterventionFlow_ReadyOverride(t *testing.T) {
	// Setup dependencies
	ctx := context.Background()
	st := store.NewInMemoryStore()
	stateManager := NewStoreBasedStateManager(st)
	timer := NewSimpleTimer()

	// Create generator with dependencies
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)

	participantID := "test_participant_005"

	// Test "Ready" override functionality

	// Start in end of day state
	err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
	if err != nil {
		t.Fatalf("Failed to set initial state: %v", err)
	}

	// Process "Ready" response
	err = generator.ProcessResponse(ctx, participantID, "ready")
	if err != nil {
		t.Fatalf("Failed to process ready response: %v", err)
	}

	// Verify transition to commitment prompt
	currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateCommitmentPrompt {
		t.Errorf("Expected state %s, got %s", models.StateCommitmentPrompt, currentState)
	}

	// Test "Ready" during feeling prompt (should trigger immediate random assignment)
	err = stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateFeelingPrompt)
	if err != nil {
		t.Fatalf("Failed to set feeling prompt state: %v", err)
	}

	err = generator.ProcessResponse(ctx, participantID, "Ready")
	if err != nil {
		t.Fatalf("Failed to process ready during feeling: %v", err)
	}

	// Should go to intervention state
	currentState, err = stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
	if err != nil {
		t.Fatalf("Failed to get current state: %v", err)
	}
	if currentState != models.StateSendInterventionImmediate && currentState != models.StateSendInterventionReflective {
		t.Errorf("Expected intervention state, got %s", currentState)
	}
}
