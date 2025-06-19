package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestCanonicalizeResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic functionality
		{
			name:     "simple text",
			input:    "Hello",
			expected: "hello",
		},
		{
			name:     "text with leading and trailing spaces",
			input:    "  Ready  ",
			expected: "ready",
		},
		{
			name:     "text with mixed case",
			input:    "ReAdY",
			expected: "ready",
		},
		{
			name:     "text with tabs and newlines",
			input:    "\t\nDONE\n\t",
			expected: "done",
		},
		{
			name:     "number choice",
			input:    " 1 ",
			expected: "1",
		},
		{
			name:     "emoji with text",
			input:    " 🚀 Let's do it! ",
			expected: "🚀 let's do it",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \t\n   ",
			expected: "",
		},

		// Punctuation handling tests
		{
			name:     "done with exclamation",
			input:    "done!",
			expected: "done",
		},
		{
			name:     "done with period",
			input:    "done.",
			expected: "done",
		},
		{
			name:     "done with multiple exclamations",
			input:    "done!!!",
			expected: "done",
		},
		{
			name:     "ready with exclamation and space",
			input:    " ready! ",
			expected: "ready",
		},
		{
			name:     "yes with period",
			input:    "yes.",
			expected: "yes",
		},
		{
			name:     "no with question mark",
			input:    "no?",
			expected: "no",
		},
		{
			name:     "number with period",
			input:    "1.",
			expected: "1",
		},
		{
			name:     "number with dash",
			input:    "1-",
			expected: "1",
		},
		{
			name:     "mixed punctuation",
			input:    "done!.",
			expected: "done",
		},
		{
			name:     "punctuation with spaces",
			input:    " ready ! ",
			expected: "ready",
		},
		{
			name:     "only punctuation",
			input:    "!",
			expected: "",
		},
		{
			name:     "text with comma",
			input:    "done,",
			expected: "done",
		},
		{
			name:     "text with semicolon",
			input:    "ready;",
			expected: "ready",
		},
		{
			name:     "text with colon",
			input:    "yes:",
			expected: "yes",
		},
		{
			name:     "text with underscore",
			input:    "done_",
			expected: "done",
		},
		{
			name:     "complex response with punctuation",
			input:    "  DONE!!!  ",
			expected: "done",
		},
		{
			name:     "real user response examples",
			input:    "Done!",
			expected: "done",
		},
		{
			name:     "real user response with enthusiasm",
			input:    "READY!!!",
			expected: "ready",
		},
		{
			name:     "formal response",
			input:    "Yes.",
			expected: "yes",
		},
		{
			name:     "hesitant response",
			input:    "no...",
			expected: "no",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canonicalizeResponse(tt.input)
			if result != tt.expected {
				t.Errorf("canonicalizeResponse(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMicroHealthInterventionCanonicalization(t *testing.T) {
	// Create mock dependencies
	mockStateManager := NewMockStateManager()
	mockTimer := NewSimpleTimer()

	// Create generator
	generator := NewMicroHealthInterventionGenerator(mockStateManager, mockTimer)

	participantID := "test_participant_canonicalization"
	ctx := context.Background()

	// Test various input formats for commitment response
	commitmentTests := []struct {
		name     string
		input    string
		expected models.StateType
	}{
		{"clean input", "1", models.StateFeelingPrompt},
		{"with spaces", "  1  ", models.StateFeelingPrompt},
		{"with tabs", "\t1\t", models.StateFeelingPrompt},
		{"emoji choice", "🚀 Let's do it!", models.StateFeelingPrompt},
		{"mixed case emoji", "  🚀 LET'S DO IT!  ", models.StateFeelingPrompt},
		{"negative response", " 2 ", models.StateEndOfDay},
		{"negative emoji", "⏳ Not yet", models.StateEndOfDay},
		{"mixed case negative", "  ⏳ NOT YET  ", models.StateEndOfDay},
		// Punctuation handling tests
		{"number with period", "1.", models.StateFeelingPrompt},
		{"number with exclamation", "1!", models.StateFeelingPrompt},
		{"number with comma", "1,", models.StateFeelingPrompt},
		{"negative with punctuation", "2!", models.StateEndOfDay},
		{"mixed punctuation", "1!.", models.StateFeelingPrompt},
	}

	for _, tt := range commitmentTests {
		t.Run("commitment_"+tt.name, func(t *testing.T) {
			// Reset state for each test
			mockStateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateCommitmentPrompt)

			err := generator.ProcessResponse(ctx, participantID, tt.input)
			if err != nil {
				t.Fatalf("ProcessResponse failed: %v", err)
			}

			// Check final state
			state, err := mockStateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("GetCurrentState failed: %v", err)
			}

			if state != tt.expected {
				t.Errorf("Expected state %s, got %s for input %q", tt.expected, state, tt.input)
			}
		})
	}

	// Test feeling response canonicalization
	feelingTests := []struct {
		name   string
		input  string
		stored string
	}{
		{"clean number", "3", "3"},
		{"with spaces", "  3  ", "3"},
		{"with tabs", "\t4\t", "4"},
		{"ready override", " READY ", "on_demand"},
		{"ready mixed case", "ReAdY", "on_demand"},
		// Punctuation handling tests
		{"number with period", "3.", "3"},
		{"number with exclamation", "4!", "4"},
		{"number with comma", "2,", "2"},
		{"ready with punctuation", "ready!", "on_demand"},
		{"ready with period", "Ready.", "on_demand"},
		{"mixed punctuation number", "1!.", "1"},
	}

	for _, tt := range feelingTests {
		t.Run("feeling_"+tt.name, func(t *testing.T) {
			// Reset state to feeling prompt
			mockStateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateFeelingPrompt)

			err := generator.ProcessResponse(ctx, participantID, tt.input)
			if err != nil {
				t.Fatalf("ProcessResponse failed: %v", err)
			}

			// Check stored feeling response
			storedFeeling, err := mockStateManager.GetStateData(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.DataKeyFeelingResponse)
			if err != nil {
				t.Fatalf("GetStateData failed: %v", err)
			}

			if storedFeeling != tt.stored {
				t.Errorf("Expected stored feeling %s, got %s for input %q", tt.stored, storedFeeling, tt.input)
			}
		})
	}

	// Test intervention response canonicalization
	interventionTests := []struct {
		name     string
		input    string
		expected models.StateType
	}{
		{"clean done", "done", models.StateReinforcementFollowup},
		{"mixed case done", "DONE", models.StateReinforcementFollowup},
		{"done with spaces", "  done  ", models.StateReinforcementFollowup},
		{"clean no", "no", models.StateDidYouGetAChance},
		{"mixed case no", "NO", models.StateDidYouGetAChance},
		{"no with spaces", "  no  ", models.StateDidYouGetAChance},
	}

	for _, tt := range interventionTests {
		t.Run("intervention_"+tt.name, func(t *testing.T) {
			// Reset state to intervention state
			mockStateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateSendInterventionImmediate)

			err := generator.ProcessResponse(ctx, participantID, tt.input)
			if err != nil {
				t.Fatalf("ProcessResponse failed: %v", err)
			}

			// Check final state
			state, err := mockStateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("GetCurrentState failed: %v", err)
			}

			if state != tt.expected {
				t.Errorf("Expected state %s, got %s for input %q", tt.expected, state, tt.input)
			}
		})
	}

	// Test "Ready" override works from END_OF_DAY state
	readyTests := []string{
		"ready",
		"READY",
		"Ready",
		"  ready  ",
		"  READY  ",
		"\tReAdY\t",
	}

	for i, input := range readyTests {
		t.Run("ready_override_"+string(rune('a'+i)), func(t *testing.T) {
			// Set state to END_OF_DAY
			mockStateManager.SetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention, models.StateEndOfDay)

			err := generator.ProcessResponse(ctx, participantID, input)
			if err != nil {
				t.Fatalf("ProcessResponse failed: %v", err)
			}

			// Check that state transitioned to commitment prompt
			state, err := mockStateManager.GetCurrentState(ctx, participantID, models.FlowTypeMicroHealthIntervention)
			if err != nil {
				t.Fatalf("GetCurrentState failed: %v", err)
			}

			if state != models.StateCommitmentPrompt {
				t.Errorf("Expected state %s, got %s for ready input %q", models.StateCommitmentPrompt, state, input)
			}
		})
	}
}

// MockStateManager implements StateManager interface for testing
type MockStateManager struct {
	states map[string]string // participantID+flowType -> state
	data   map[string]string // participantID+flowType+key -> value
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		states: make(map[string]string),
		data:   make(map[string]string),
	}
}

func (m *MockStateManager) GetCurrentState(ctx context.Context, participantID string, flowType models.FlowType) (models.StateType, error) {
	key := participantID + ":" + string(flowType)
	return models.StateType(m.states[key]), nil
}

func (m *MockStateManager) SetCurrentState(ctx context.Context, participantID string, flowType models.FlowType, state models.StateType) error {
	key := participantID + ":" + string(flowType)
	m.states[key] = string(state)
	return nil
}

func (m *MockStateManager) GetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey) (string, error) {
	dataKey := participantID + ":" + string(flowType) + ":" + string(key)
	return m.data[dataKey], nil
}

func (m *MockStateManager) SetStateData(ctx context.Context, participantID string, flowType models.FlowType, key models.DataKey, value string) error {
	dataKey := participantID + ":" + string(flowType) + ":" + string(key)
	m.data[dataKey] = value
	return nil
}

func (m *MockStateManager) TransitionState(ctx context.Context, participantID string, flowType models.FlowType, fromState, toState models.StateType) error {
	return m.SetCurrentState(ctx, participantID, flowType, toState)
}

func (m *MockStateManager) ResetState(ctx context.Context, participantID string, flowType models.FlowType) error {
	// Remove state and data for this participant and flow type
	stateKey := participantID + ":" + string(flowType)
	delete(m.states, stateKey)

	// Remove all state data for this participant and flow type
	prefix := participantID + ":" + string(flowType) + ":"
	for dataKey := range m.data {
		if strings.HasPrefix(dataKey, prefix) {
			delete(m.data, dataKey)
		}
	}
	return nil
}
