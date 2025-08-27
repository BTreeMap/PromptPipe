// Package util provides environment variable parsing helpers shared across components.
package util

import (
	"log/slog"
	"os"
	"strings"
)

// ParseBoolEnv parses a boolean environment variable with a default value.
// Accepts: true/1/yes/on and false/0/no/off (case-insensitive). Invalid values return default.
func ParseBoolEnv(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		slog.Warn("ParseBoolEnv: invalid boolean value, using default", "key", key, "value", val, "default", defaultValue)
		return defaultValue
	}
}
