package ui

import (
	"testing"
)

func TestValidateAppName(t *testing.T) {
	valid := []string{"myapp", "my-app", "my_app", "app123", "a", "0app"}
	for _, name := range valid {
		if err := validateAppName(name); err != nil {
			t.Errorf("validateAppName(%q) should be valid, got: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"MyApp",             // uppercase
		"-app",              // starts with hyphen
		"_app",              // starts with underscore
		"my app",            // space
		"my.app",            // dot
		"app; rm -rf /",     // injection
		"app\necho pwned",   // newline injection
		"app$(whoami)",      // command substitution
		"app`whoami`",       // backtick injection
		"app|cat /etc/passwd", // pipe injection
	}
	for _, name := range invalid {
		if err := validateAppName(name); err == nil {
			t.Errorf("validateAppName(%q) should be invalid", name)
		}
	}
}

func TestValidateAppNameMaxLength(t *testing.T) {
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if err := validateAppName(string(long)); err == nil {
		t.Error("validateAppName should reject names longer than 128 characters")
	}
}

func TestValidateServerName(t *testing.T) {
	valid := []string{"prod", "staging-1", "web_server", "My.Server", "server01"}
	for _, name := range valid {
		if err := validateServerName(name); err != nil {
			t.Errorf("validateServerName(%q) should be valid, got: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"-server",
		".server",
		"_server",
		"server; rm -rf /",
		"server\necho pwned",
		"server$(whoami)",
		"server`whoami`",
		"my server", // space
	}
	for _, name := range invalid {
		if err := validateServerName(name); err == nil {
			t.Errorf("validateServerName(%q) should be invalid", name)
		}
	}
}

func TestValidateEnvKey(t *testing.T) {
	valid := []string{"DATABASE_URL", "PORT", "MY_VAR_123", "_PRIVATE", "a"}
	for _, key := range valid {
		if err := validateEnvKey(key); err != nil {
			t.Errorf("validateEnvKey(%q) should be valid, got: %v", key, err)
		}
	}

	invalid := []string{
		"",
		"123_VAR",         // starts with digit
		"MY-VAR",          // hyphen
		"MY VAR",          // space
		"KEY=VALUE",       // equals sign
		"KEY;echo pwned",  // injection
	}
	for _, key := range invalid {
		if err := validateEnvKey(key); err == nil {
			t.Errorf("validateEnvKey(%q) should be invalid", key)
		}
	}
}

func TestValidateGroupName(t *testing.T) {
	valid := []string{"Production", "staging-1", "My Group", "web_servers", "Group 123"}
	for _, name := range valid {
		if err := validateGroupName(name); err != nil {
			t.Errorf("validateGroupName(%q) should be valid, got: %v", name, err)
		}
	}

	invalid := []string{
		"",
		" group",                // starts with space
		"-group",                // starts with hyphen
		"group; rm -rf /",       // injection (semicolon)
		"group$(whoami)",        // command substitution
		"group\necho pwned",     // newline
	}
	for _, name := range invalid {
		if err := validateGroupName(name); err == nil {
			t.Errorf("validateGroupName(%q) should be invalid", name)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\"'\"'s'"},
		{"$(whoami)", "'$(whoami)'"},
		{"`rm -rf /`", "'`rm -rf /`'"},
		{"a;b", "'a;b'"},
		{"a|b", "'a|b'"},
		{"", "''"},
	}

	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
