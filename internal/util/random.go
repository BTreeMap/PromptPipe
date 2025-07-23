// Package util provides utility functions for the PromptPipe application.
package util

import (
	"math/rand/v2"
	"strings"
)

// GenerateRandomID generates a random ID with the specified prefix and hex length.
// The returned ID will be in the format: "{prefix}{hex_string}".
// Uses math/rand/v2 for optimal performance with modern best practices.
func GenerateRandomID(prefix string, hexLength int) string {
	return prefix + GenerateRandomHex(hexLength)
}

// GenerateRandomHex generates a random hexadecimal string of the specified length.
// Uses math/rand/v2 with optimal entropy utilization for non-cryptographic purposes.
func GenerateRandomHex(length int) string {
	if length <= 0 {
		return ""
	}

	const hexChars = "0123456789abcdef"
	var builder strings.Builder
	builder.Grow(length) // Pre-allocate capacity for efficiency

	for i := 0; i < length; i++ {
		builder.WriteByte(hexChars[rand.IntN(16)])
	}

	return builder.String()
}

// GenerateRandomAlphaNumeric generates a random alphanumeric string of the specified length.
// Uses math/rand/v2 for optimal performance and modern best practices.
func GenerateRandomAlphaNumeric(length int) string {
	if length <= 0 {
		return ""
	}

	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var builder strings.Builder
	builder.Grow(length) // Pre-allocate capacity for efficiency

	for i := 0; i < length; i++ {
		builder.WriteByte(chars[rand.IntN(len(chars))])
	}

	return builder.String()
}

// GenerateParticipantID generates a unique participant ID with "p_" prefix.
func GenerateParticipantID() string {
	return GenerateRandomID("p_", 32)
}

// GenerateResponseID generates a unique response ID with "r_" prefix.
func GenerateResponseID() string {
	return GenerateRandomID("r_", 32)
}
