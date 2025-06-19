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

// TestMicroHealthInterventionFlow_PunctuationHandling tests that the system correctly handles
// user responses with various punctuation marks (done!, done., ready!, yes!, etc.)
func TestMicroHealthInterventionFlow_PunctuationHandling(t *testing.T) {
	stateManager := NewMockStateManager()
	timer := NewSimpleTimer()
	generator := NewMicroHealthInterventionGenerator(stateManager, timer)
	ctx := context.Background()
	participantID := "test-participant-punctuation"

	// Test cases for "done" responses with punctuation
	doneTestCases := []string{
		"done!",
		"done.",
		"Done!",
		"DONE!",
		"  done!  ",
		"done!!!",
		"done!.",
		"done,",
		"done;",
		"done:",
		"done-",
		"done_",
	}

	for _, doneResponse := range doneTestCases {
		t.Run("done_response_"+doneResponse, func(t *testing.T) {
			// Reset state for each test
			err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateSendInterventionImmediate)
			if err != nil {
				t.Fatalf("Failed to set initial state: %v", err)
			}

			// Process the "done" response with punctuation
			err = generator.ProcessResponse(ctx, participantID, doneResponse)
			if err != nil {
				t.Fatalf("Failed to process done response %q: %v", doneResponse, err)
			}

			// Verify it transitioned to reinforcement state
			currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("Failed to get current state: %v", err)
			}
			if currentState != models.StateReinforcementFollowup {
				t.Errorf("Expected state %s after done response %q, got %s", models.StateReinforcementFollowup, doneResponse, currentState)
			}

			// Verify the response was stored correctly (without punctuation)
			storedResponse, err := stateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyCompletionResponse)
			if err != nil {
				t.Fatalf("Failed to get stored response: %v", err)
			}
			if storedResponse != string(models.ResponseDone) {
				t.Errorf("Expected stored response %s, got %s", models.ResponseDone, storedResponse)
			}
		})
	}

	// Test cases for "ready" responses with punctuation
	readyTestCases := []string{
		"ready!",
		"ready.",
		"Ready!",
		"READY!",
		"  ready!  ",
		"ready!!!",
		"ready!.",
	}

	for _, readyResponse := range readyTestCases {
		t.Run("ready_response_"+readyResponse, func(t *testing.T) {
			// Reset state to END_OF_DAY for ready override test
			err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)
			if err != nil {
				t.Fatalf("Failed to set initial state: %v", err)
			}

			// Process the "ready" response with punctuation
			err = generator.ProcessResponse(ctx, participantID, readyResponse)
			if err != nil {
				t.Fatalf("Failed to process ready response %q: %v", readyResponse, err)
			}

			// Verify it transitioned to commitment prompt state
			currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("Failed to get current state: %v", err)
			}
			if currentState != models.StateCommitmentPrompt {
				t.Errorf("Expected state %s after ready response %q, got %s", models.StateCommitmentPrompt, readyResponse, currentState)
			}
		})
	}

	// Test cases for "yes" and "no" responses with punctuation
	yesNoTestCases := []struct {
		response      string
		expectedState models.StateType
	}{
		{"yes!", models.StateContextQuestion},
		{"yes.", models.StateContextQuestion},
		{"Yes!", models.StateContextQuestion},
		{"YES!", models.StateContextQuestion},
		{"  yes!  ", models.StateContextQuestion},
		{"no!", models.StateBarrierReasonNoChance},
		{"no.", models.StateBarrierReasonNoChance},
		{"No!", models.StateBarrierReasonNoChance},
		{"NO!", models.StateBarrierReasonNoChance},
		{"  no!  ", models.StateBarrierReasonNoChance},
	}

	for _, testCase := range yesNoTestCases {
		t.Run("yes_no_response_"+testCase.response, func(t *testing.T) {
			// Reset state to DID_YOU_GET_A_CHANCE for yes/no test
			err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateDidYouGetAChance)
			if err != nil {
				t.Fatalf("Failed to set initial state: %v", err)
			}

			// Process the yes/no response with punctuation
			err = generator.ProcessResponse(ctx, participantID, testCase.response)
			if err != nil {
				t.Fatalf("Failed to process yes/no response %q: %v", testCase.response, err)
			}

			// Verify it transitioned to the expected state
			currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("Failed to get current state: %v", err)
			}
			if currentState != testCase.expectedState {
				t.Errorf("Expected state %s after response %q, got %s", testCase.expectedState, testCase.response, currentState)
			}
		})
	}

	// Test cases for numeric responses with punctuation
	numericTestCases := []struct {
		response      string
		initialState  models.StateType
		expectedState models.StateType
	}{
		{"1!", models.StateCommitmentPrompt, models.StateFeelingPrompt},
		{"1.", models.StateCommitmentPrompt, models.StateFeelingPrompt},
		{"2!", models.StateCommitmentPrompt, models.StateEndOfDay},
		{"2.", models.StateCommitmentPrompt, models.StateEndOfDay},
		{"3!", models.StateFeelingPrompt, models.StateRandomAssignment},
		{"3.", models.StateFeelingPrompt, models.StateRandomAssignment},
		{"4!", models.StateFeelingPrompt, models.StateRandomAssignment},
		{"4.", models.StateFeelingPrompt, models.StateRandomAssignment},
	}

	for _, testCase := range numericTestCases {
		t.Run("numeric_response_"+testCase.response, func(t *testing.T) {
			// Reset state to the specified initial state
			err := stateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, testCase.initialState)
			if err != nil {
				t.Fatalf("Failed to set initial state: %v", err)
			}

			// Process the numeric response with punctuation
			err = generator.ProcessResponse(ctx, participantID, testCase.response)
			if err != nil {
				t.Fatalf("Failed to process numeric response %q: %v", testCase.response, err)
			}

			// For feeling prompts that go to random assignment, we need to check for either intervention state
			if testCase.expectedState == models.StateRandomAssignment {
				currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
				if err != nil {
					t.Fatalf("Failed to get current state: %v", err)
				}
				if currentState != models.StateSendInterventionImmediate && currentState != models.StateSendInterventionReflective {
					t.Errorf("Expected intervention state after response %q, got %s", testCase.response, currentState)
				}
			} else {
				// Verify it transitioned to the expected state
				currentState, err := stateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
				if err != nil {
					t.Fatalf("Failed to get current state: %v", err)
				}
				if currentState != testCase.expectedState {
					t.Errorf("Expected state %s after response %q, got %s", testCase.expectedState, testCase.response, currentState)
				}
			}
		})
	}
}
