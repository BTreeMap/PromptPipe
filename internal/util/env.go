// Package util provides environment variable parsing helpers shared across components.
package util

import (
	"log/slog"
	"os"
	"strconv"
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

// GetEnvWithDefault returns the environment variable or a default if unset/empty.
func GetEnvWithDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// ParseIntEnv parses an integer environment variable with a default fallback.
func ParseIntEnv(key string, defaultValue int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("ParseIntEnv: invalid integer value, using default", "key", key, "value", val, "default", defaultValue)
		return defaultValue
	}
	return intVal
}

// ParseFloatEnv parses a float environment variable with a default fallback.
// For temperature-like values (0.0-1.0) caller should validate range if needed.
func ParseFloatEnv(key string, defaultValue float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	floatVal, err := strconv.ParseFloat(val, 64)
	if err != nil {
		slog.Warn("ParseFloatEnv: invalid float value, using default", "key", key, "value", val, "default", defaultValue)
		return defaultValue
	}
	return floatVal
}
