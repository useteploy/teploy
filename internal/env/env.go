package env

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/teploy/teploy/internal/ssh"
)

const deploymentsDir = "/deployments"

// Manager reads and writes env files on the server.
type Manager struct {
	exec ssh.Executor
}

// NewManager creates a new env manager backed by the given SSH executor.
func NewManager(exec ssh.Executor) *Manager {
	return &Manager{exec: exec}
}

// envPath returns the path to an app's env file on the server.
func envPath(app string) string {
	return fmt.Sprintf("%s/%s/.env", deploymentsDir, app)
}

// Get returns the value of a single env var, or an error if not set.
func (m *Manager) Get(ctx context.Context, app, key string) (string, error) {
	vars, err := m.readEnv(ctx, app)
	if err != nil {
		return "", err
	}
	val, ok := vars[key]
	if !ok {
		return "", fmt.Errorf("key %s is not set", key)
	}
	return val, nil
}

// Set writes one or more key=value pairs to the env file.
// Existing keys are updated, new keys are appended.
// Returns an error if any key is PORT (reserved by teploy).
func (m *Manager) Set(ctx context.Context, app string, pairs map[string]string) error {
	for k := range pairs {
		if strings.ToUpper(k) == "PORT" {
			return fmt.Errorf("PORT is reserved — teploy sets it automatically on every container start")
		}
	}

	vars, err := m.readEnv(ctx, app)
	if err != nil {
		return err
	}
	for k, v := range pairs {
		vars[k] = v
	}
	return m.writeEnv(ctx, app, vars)
}

// Unset removes a key from the env file.
func (m *Manager) Unset(ctx context.Context, app, key string) error {
	vars, err := m.readEnv(ctx, app)
	if err != nil {
		return err
	}
	if _, ok := vars[key]; !ok {
		return fmt.Errorf("key %s is not set", key)
	}
	delete(vars, key)
	return m.writeEnv(ctx, app, vars)
}

// Entry represents a single key-value pair for display.
type Entry struct {
	Key   string
	Value string
}

// List returns all env vars sorted by key.
func (m *Manager) List(ctx context.Context, app string) ([]Entry, error) {
	vars, err := m.readEnv(ctx, app)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]Entry, len(keys))
	for i, k := range keys {
		entries[i] = Entry{Key: k, Value: vars[k]}
	}
	return entries, nil
}

// readEnv reads and parses the env file from the server.
// Returns an empty map if the file doesn't exist.
func (m *Manager) readEnv(ctx context.Context, app string) (map[string]string, error) {
	output, err := m.exec.Run(ctx, "cat "+envPath(app)+" 2>/dev/null")
	if err != nil || strings.TrimSpace(output) == "" {
		return make(map[string]string), nil
	}
	return parseEnv(output), nil
}

// writeEnv serializes the vars map and writes it to the server with secure permissions.
func (m *Manager) writeEnv(ctx context.Context, app string, vars map[string]string) error {
	// Ensure directory exists.
	if _, err := m.exec.Run(ctx, fmt.Sprintf("mkdir -p %s/%s", deploymentsDir, app)); err != nil {
		return fmt.Errorf("creating app directory: %w", err)
	}

	content := serializeEnv(vars)
	path := envPath(app)
	if err := m.exec.Upload(ctx, strings.NewReader(content), path, "0600"); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}

	// Ensure ownership is root:root.
	m.exec.Run(ctx, fmt.Sprintf("chown root:root %s", path))
	return nil
}

// parseEnv parses dotenv-format content into a map.
// Handles quoted values, blank lines, and comments.
func parseEnv(content string) map[string]string {
	vars := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+1:]
		val = unquote(val)
		vars[key] = val
	}
	return vars
}

// serializeEnv writes a vars map as sorted key=value lines.
func serializeEnv(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		v := vars[k]
		if needsQuoting(v) {
			fmt.Fprintf(&sb, "%s=%q\n", k, v)
		} else {
			fmt.Fprintf(&sb, "%s=%s\n", k, v)
		}
	}
	return sb.String()
}

// unquote strips surrounding single or double quotes from a value.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// needsQuoting returns true if the value contains spaces, quotes, or special chars.
func needsQuoting(s string) bool {
	return strings.ContainsAny(s, " \t\"'\\$`!#&|;(){}[]")
}
