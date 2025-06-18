package whatsapp

import (
	"context"
	"strings"
	"testing"

	"github.com/BTreeMap/PromptPipe/internal/store"
)

func TestDSNDetection(t *testing.T) {
	tests := []struct {
		name           string
		dsn            string
		expectedDriver string
	}{
		{
			name:           "PostgreSQL DSN with postgres:// scheme",
			dsn:            "postgres://user:password@localhost/dbname",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with host= parameter",
			dsn:            "host=localhost user=postgres dbname=test",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with multiple key=value pairs",
			dsn:            "user=postgres password=secret dbname=test sslmode=disable",
			expectedDriver: "postgres",
		},
		{
			name:           "SQLite DSN with file path",
			dsn:            "/var/lib/promptpipe/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with relative path",
			dsn:            "./data/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with .db extension",
			dsn:            "test.db",
			expectedDriver: "sqlite3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the DSN detection logic using the shared function
			detectedDriver := store.DetectDSNType(tt.dsn)

			if detectedDriver != tt.expectedDriver {
				t.Errorf("DSN detection failed for %q: expected driver %q, got %q", tt.dsn, tt.expectedDriver, detectedDriver)
			}
		})
	}
}

func TestWithDBDSNOption(t *testing.T) {
	opts := &Opts{}

	testDSN := "/var/lib/promptpipe/test.db"
	WithDBDSN(testDSN)(opts)

	if opts.DBDSN != testDSN {
		t.Errorf("Expected DBDSN to be %q, got %q", testDSN, opts.DBDSN)
	}
}

func TestWithQRCodeOutputOption(t *testing.T) {
	opts := &Opts{}

	testPath := "/tmp/qr.txt"
	WithQRCodeOutput(testPath)(opts)

	if opts.QRPath != testPath {
		t.Errorf("Expected QRPath to be %q, got %q", testPath, opts.QRPath)
	}
}

func TestWithNumericCodeOption(t *testing.T) {
	opts := &Opts{}

	WithNumericCode()(opts)

	if !opts.NumericCode {
		t.Errorf("Expected NumericCode to be true, got false")
	}
}

func TestNewClientOptionsApplied(t *testing.T) {
	// Test that options are properly applied when creating a new client
	// We don't actually create the client to avoid database connections

	testDSN := "/tmp/test.db"

	opts := &Opts{}
	WithDBDSN(testDSN)(opts)

	if opts.DBDSN != testDSN {
		t.Errorf("Expected DBDSN to be %q, got %q", testDSN, opts.DBDSN)
	}
}

func TestCanonicalizePhoneNumber(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expected     string
		expectChange bool
	}{
		{
			name:         "Plain numeric phone number",
			input:        "1234567890",
			expected:     "1234567890",
			expectChange: false,
		},
		{
			name:         "Phone number with dashes",
			input:        "123-456-7890",
			expected:     "1234567890",
			expectChange: true,
		},
		{
			name:         "Phone number with spaces",
			input:        "123 456 7890",
			expected:     "1234567890",
			expectChange: true,
		},
		{
			name:         "Phone number with parentheses",
			input:        "(123) 456-7890",
			expected:     "1234567890",
			expectChange: true,
		},
		{
			name:         "International format with plus",
			input:        "+1-234-567-8900",
			expected:     "12345678900",
			expectChange: true,
		},
		{
			name:         "Phone number with dots",
			input:        "123.456.7890",
			expected:     "1234567890",
			expectChange: true,
		},
		{
			name:         "Phone number with mixed separators",
			input:        "+1 (234) 567-8900",
			expected:     "12345678900",
			expectChange: true,
		},
		{
			name:         "Empty string",
			input:        "",
			expected:     "",
			expectChange: false,
		},
		{
			name:         "Only non-numeric characters",
			input:        "abc-def",
			expected:     "",
			expectChange: true,
		},
		{
			name:         "Phone number with extension",
			input:        "123-456-7890 ext 123",
			expected:     "1234567890123",
			expectChange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := canonicalizePhoneNumber(tt.input)
			
			if result != tt.expected {
				t.Errorf("canonicalizePhoneNumber(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
			
			if changed != tt.expectChange {
				t.Errorf("canonicalizePhoneNumber(%q) changed flag = %v, expected %v", tt.input, changed, tt.expectChange)
			}
		})
	}
}

func TestSendMessagePhoneNumberValidation(t *testing.T) {
	// Create a mock client to test validation without actual WhatsApp connection
	mockClient := &MockClient{}
	
	// Test cases for phone number validation in SendMessage
	tests := []struct {
		name        string
		phoneNumber string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "Valid phone number",
			phoneNumber: "1234567890",
			expectError: false,
		},
		{
			name:        "Valid international number",
			phoneNumber: "+1-234-567-8900",
			expectError: false,
		},
		{
			name:        "Empty phone number",
			phoneNumber: "",
			expectError: true,
			errorSubstr: "recipient cannot be empty",
		},
		{
			name:        "Phone number with no digits",
			phoneNumber: "abc-def-ghij",
			expectError: true,
			errorSubstr: "no digits found",
		},
		{
			name:        "Phone number too short (5 digits)",
			phoneNumber: "12345",
			expectError: true,
			errorSubstr: "too short",
		},
		{
			name:        "Phone number too short with formatting",
			phoneNumber: "123-45",
			expectError: true,
			errorSubstr: "too short",
		},
		{
			name:        "Minimum valid length (6 digits)",
			phoneNumber: "123456",
			expectError: false,
		},
		{
			name:        "Minimum valid length with formatting",
			phoneNumber: "123-456",
			expectError: false,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mockClient.SendMessage(ctx, tt.phoneNumber, "test message")
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for phone number %q, but got nil", tt.phoneNumber)
					return
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Expected error to contain %q, but got: %s", tt.errorSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for phone number %q, but got: %s", tt.phoneNumber, err.Error())
				}
			}
		})
	}
}
