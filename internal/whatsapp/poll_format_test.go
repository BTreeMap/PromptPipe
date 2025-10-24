package whatsapp

import "testing"

func TestFormatPollResponse(t *testing.T) {
	tests := []struct {
		name     string
		question string
		answer   string
		expected string
	}{
		{
			name:     "Standard poll response",
			question: "Did you do it?",
			answer:   "Done",
			expected: "Q: Did you do it? A: Done",
		},
		{
			name:     "Alternative answer",
			question: "Did you do it?",
			answer:   "Next time",
			expected: "Q: Did you do it? A: Next time",
		},
		{
			name:     "Generic response",
			question: "Did you do it?",
			answer:   "[responded]",
			expected: "Q: Did you do it? A: [responded]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPollResponse(tt.question, tt.answer)
			if result != tt.expected {
				t.Errorf("FormatPollResponse(%q, %q) = %q, expected %q",
					tt.question, tt.answer, result, tt.expected)
			}
		})
	}
}

func TestGetSuccessPollResponse(t *testing.T) {
	// This tests that GetSuccessPollResponse returns the expected format
	expected := "Q: Did you do it? A: Done"
	result := GetSuccessPollResponse()

	if result != expected {
		t.Errorf("GetSuccessPollResponse() = %q, expected %q", result, expected)
	}

	// Also verify it matches the actual constants
	expectedFromConstants := FormatPollResponse(PollQuestion, PollOptions[0])
	if result != expectedFromConstants {
		t.Errorf("GetSuccessPollResponse() = %q, but FormatPollResponse(PollQuestion, PollOptions[0]) = %q",
			result, expectedFromConstants)
	}
}

func TestPollResponseConsistency(t *testing.T) {
	// Verify that the success response format is consistent
	manualFormat := "Q: Did you do it? A: Done"
	fromFunction := GetSuccessPollResponse()

	if manualFormat != fromFunction {
		t.Errorf("Poll response format inconsistency:\n  Manual: %q\n  Function: %q",
			manualFormat, fromFunction)
	}
}
