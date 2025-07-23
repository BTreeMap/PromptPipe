package util

import (
	"strings"
	"testing"
)

func TestGenerateRandomID(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		hexLength  int
		wantPrefix string
		wantLength int // expected total length: prefix + hexLength
	}{
		{
			name:       "participant ID format",
			prefix:     "p_",
			hexLength:  32,
			wantPrefix: "p_",
			wantLength: 34, // 2 + 32
		},
		{
			name:       "response ID format",
			prefix:     "r_",
			hexLength:  32,
			wantPrefix: "r_",
			wantLength: 34, // 2 + 32
		},
		{
			name:       "custom prefix",
			prefix:     "test_",
			hexLength:  16,
			wantPrefix: "test_",
			wantLength: 21, // 5 + 16
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRandomID(tt.prefix, tt.hexLength)

			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("GenerateRandomID() = %v, want prefix %v", got, tt.wantPrefix)
			}

			if len(got) != tt.wantLength {
				t.Errorf("GenerateRandomID() length = %v, want %v", len(got), tt.wantLength)
			}

			// Check that the hex part is valid
			hexPart := got[len(tt.wantPrefix):]
			if !isValidHex(hexPart) {
				t.Errorf("GenerateRandomID() hex part = %v is not valid hex", hexPart)
			}
		})
	}
}

func TestGenerateRandomHex(t *testing.T) {
	tests := []struct {
		name   string
		length int
		want   int
	}{
		{"zero length", 0, 0},
		{"negative length", -1, 0},
		{"small length", 8, 8},
		{"medium length", 16, 16},
		{"large length", 64, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRandomHex(tt.length)

			if len(got) != tt.want {
				t.Errorf("GenerateRandomHex() length = %v, want %v", len(got), tt.want)
			}

			if tt.want > 0 && !isValidHex(got) {
				t.Errorf("GenerateRandomHex() = %v is not valid hex", got)
			}
		})
	}
}

func TestGenerateRandomAlphaNumeric(t *testing.T) {
	tests := []struct {
		name   string
		length int
		want   int
	}{
		{"zero length", 0, 0},
		{"negative length", -1, 0},
		{"small length", 8, 8},
		{"medium length", 16, 16},
		{"large length", 64, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRandomAlphaNumeric(tt.length)

			if len(got) != tt.want {
				t.Errorf("GenerateRandomAlphaNumeric() length = %v, want %v", len(got), tt.want)
			}

			if tt.want > 0 && !isValidAlphaNumeric(got) {
				t.Errorf("GenerateRandomAlphaNumeric() = %v is not valid alphanumeric", got)
			}
		})
	}
}

func TestGenerateParticipantID(t *testing.T) {
	got := GenerateParticipantID()

	if !strings.HasPrefix(got, "p_") {
		t.Errorf("GenerateParticipantID() = %v, want prefix p_", got)
	}

	if len(got) != 34 { // "p_" + 32 hex chars
		t.Errorf("GenerateParticipantID() length = %v, want 34", len(got))
	}

	hexPart := got[2:] // Remove "p_" prefix
	if !isValidHex(hexPart) {
		t.Errorf("GenerateParticipantID() hex part = %v is not valid hex", hexPart)
	}
}

func TestGenerateResponseID(t *testing.T) {
	got := GenerateResponseID()

	if !strings.HasPrefix(got, "r_") {
		t.Errorf("GenerateResponseID() = %v, want prefix r_", got)
	}

	if len(got) != 34 { // "r_" + 32 hex chars
		t.Errorf("GenerateResponseID() length = %v, want 34", len(got))
	}

	hexPart := got[2:] // Remove "r_" prefix
	if !isValidHex(hexPart) {
		t.Errorf("GenerateResponseID() hex part = %v is not valid hex", hexPart)
	}
}

func TestRandomIDUniqueness(t *testing.T) {
	const iterations = 1000
	seen := make(map[string]bool)

	for i := 0; i < iterations; i++ {
		id := GenerateRandomID("test_", 16)
		if seen[id] {
			t.Errorf("GenerateRandomID() generated duplicate: %v", id)
		}
		seen[id] = true
	}
}

func TestRandomHexUniqueness(t *testing.T) {
	const iterations = 1000
	seen := make(map[string]bool)

	for i := 0; i < iterations; i++ {
		hex := GenerateRandomHex(16)
		if seen[hex] {
			t.Errorf("GenerateRandomHex() generated duplicate: %v", hex)
		}
		seen[hex] = true
	}
}

// Helper function to validate hex strings
func isValidHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Helper function to validate alphanumeric strings
func isValidAlphaNumeric(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}
