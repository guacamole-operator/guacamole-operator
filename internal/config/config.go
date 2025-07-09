package config

import (
	"os"
	"strconv"
)

// EnvOrDefault returns the value of an environment variable
// or the default value.
func EnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// EnvIntOrDefault returns the value of an environment variable
// or the default value.
func EnvIntOrDefault(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// EnvBoolOrDefault returns the value of an environment variable
// or the default value.
func EnvBoolOrDefault(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
