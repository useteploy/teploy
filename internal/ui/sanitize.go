package ui

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	appNameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	serverNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	envKeyRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	groupNameRe  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _-]*$`)
)

// validateAppName checks that a string is a valid app name.
// Allowed: lowercase alphanumeric, hyphens, underscores. Must start with alphanumeric.
func validateAppName(name string) error {
	if name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("app name too long (max 128 characters)")
	}
	if !appNameRe.MatchString(name) {
		return fmt.Errorf("invalid app name %q: must match [a-z0-9][a-z0-9_-]*", name)
	}
	return nil
}

// validateServerName checks that a string is a valid server name.
// Allowed: alphanumeric, hyphens, underscores, dots. Must start with alphanumeric.
func validateServerName(name string) error {
	if name == "" {
		return fmt.Errorf("server name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("server name too long (max 128 characters)")
	}
	if !serverNameRe.MatchString(name) {
		return fmt.Errorf("invalid server name %q: must match [a-zA-Z0-9][a-zA-Z0-9_.-]*", name)
	}
	return nil
}

// validateEnvKey checks that a string is a valid environment variable key.
func validateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("env key cannot be empty")
	}
	if len(key) > 256 {
		return fmt.Errorf("env key too long (max 256 characters)")
	}
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", key)
	}
	return nil
}

// validateGroupName checks that a string is a valid group name.
func validateGroupName(name string) error {
	if name == "" {
		return fmt.Errorf("group name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("group name too long (max 128 characters)")
	}
	if !groupNameRe.MatchString(name) {
		return fmt.Errorf("invalid group name %q: must match [a-zA-Z0-9][a-zA-Z0-9 _-]*", name)
	}
	return nil
}

// validateProjectName checks that a string is a valid project name.
// Uses the same rules as group names.
func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("project name too long (max 128 characters)")
	}
	if !groupNameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must match [a-zA-Z0-9][a-zA-Z0-9 _-]*", name)
	}
	return nil
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
// This prevents shell injection when constructing SSH commands.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
