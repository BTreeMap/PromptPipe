package whatsapp

import (
	"testing"
)

func TestParseIntensityPollResponse(t *testing.T) {
	tests := []struct {
		name             string
		response         string
		currentIntensity string
		expectedNew      string
	}{
		{
			name:             "Decrease from normal to low",
			response:         "Q: How's the intensity? A: Decrease",
			currentIntensity: "normal",
			expectedNew:      "low",
		},
		{
			name:             "Decrease from high to normal",
			response:         "Q: How's the intensity? A: Decrease",
			currentIntensity: "high",
			expectedNew:      "normal",
		},
		{
			name:             "Decrease from low stays low",
			response:         "Q: How's the intensity? A: Decrease",
			currentIntensity: "low",
			expectedNew:      "low",
		},
		{
			name:             "Increase from low to normal",
			response:         "Q: How's the intensity? A: Increase",
			currentIntensity: "low",
			expectedNew:      "normal",
		},
		{
			name:             "Increase from normal to high",
			response:         "Q: How's the intensity? A: Increase",
			currentIntensity: "normal",
			expectedNew:      "high",
		},
		{
			name:             "Increase from high stays high",
			response:         "Q: How's the intensity? A: Increase",
			currentIntensity: "high",
			expectedNew:      "high",
		},
		{
			name:             "Keep current at low",
			response:         "Q: How's the intensity? A: Keep current",
			currentIntensity: "low",
			expectedNew:      "low",
		},
		{
			name:             "Keep current at normal",
			response:         "Q: How's the intensity? A: Keep current",
			currentIntensity: "normal",
			expectedNew:      "normal",
		},
		{
			name:             "Keep current at high",
			response:         "Q: How's the intensity? A: Keep current",
			currentIntensity: "high",
			expectedNew:      "high",
		},
		{
			name:             "Not an intensity response",
			response:         "Q: Did you do it? A: Done",
			currentIntensity: "normal",
			expectedNew:      "",
		},
		{
			name:             "Random user message",
			response:         "I'm feeling great today!",
			currentIntensity: "normal",
			expectedNew:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseIntensityPollResponse(tt.response, tt.currentIntensity)
			if result != tt.expectedNew {
				t.Errorf("ParseIntensityPollResponse(%q, %q) = %q, expected %q",
					tt.response, tt.currentIntensity, result, tt.expectedNew)
			}
		})
	}
}

func TestIntensityPollOptions(t *testing.T) {
	// Verify that each intensity level has the correct options
	lowOptions, ok := IntensityPollOptions["low"]
	if !ok {
		t.Fatal("IntensityPollOptions missing 'low' key")
	}
	if len(lowOptions) != 2 {
		t.Errorf("Expected 2 options for 'low', got %d", len(lowOptions))
	}
	if lowOptions[0] != "Keep current" || lowOptions[1] != "Increase" {
		t.Errorf("Unexpected options for 'low': %v", lowOptions)
	}

	normalOptions, ok := IntensityPollOptions["normal"]
	if !ok {
		t.Fatal("IntensityPollOptions missing 'normal' key")
	}
	if len(normalOptions) != 3 {
		t.Errorf("Expected 3 options for 'normal', got %d", len(normalOptions))
	}
	if normalOptions[0] != "Decrease" || normalOptions[1] != "Keep current" || normalOptions[2] != "Increase" {
		t.Errorf("Unexpected options for 'normal': %v", normalOptions)
	}

	highOptions, ok := IntensityPollOptions["high"]
	if !ok {
		t.Fatal("IntensityPollOptions missing 'high' key")
	}
	if len(highOptions) != 2 {
		t.Errorf("Expected 2 options for 'high', got %d", len(highOptions))
	}
	if highOptions[0] != "Decrease" || highOptions[1] != "Keep current" {
		t.Errorf("Unexpected options for 'high': %v", highOptions)
	}
}

func TestIntensityPollQuestion(t *testing.T) {
	if IntensityPollQuestion != "How's the intensity?" {
		t.Errorf("Unexpected IntensityPollQuestion: %q", IntensityPollQuestion)
	}
}
