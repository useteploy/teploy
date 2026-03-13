package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/useteploy/teploy/internal/ssh"
)

const deploymentsDir = "/deployments"

// AppState represents the deploy state for an app on the server.
// Stored at /deployments/<app>/state as key=value pairs.
type AppState struct {
	CurrentPort  int
	CurrentHash  string
	PreviousPort int
	PreviousHash string
}

// LogEntry represents a single entry in /deployments/teploy.log.
type LogEntry struct {
	Timestamp  time.Time `json:"ts"`
	App        string    `json:"app"`
	Type       string    `json:"type"` // deploy, rollback, restart, health_failure
	Hash       string    `json:"hash,omitempty"`
	Success    bool      `json:"success"`
	DurationMs int64     `json:"duration_ms"`
	Message    string    `json:"message,omitempty"`
}

// Read reads the app state from the server. Returns nil if no state exists.
func Read(ctx context.Context, exec ssh.Executor, app string) (*AppState, error) {
	path := fmt.Sprintf("%s/%s/state", deploymentsDir, app)
	output, err := exec.Run(ctx, fmt.Sprintf("cat %s 2>/dev/null", path))
	if err != nil || strings.TrimSpace(output) == "" {
		return nil, nil
	}

	s := &AppState{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "current_port":
			s.CurrentPort, _ = strconv.Atoi(parts[1])
		case "current_hash":
			s.CurrentHash = parts[1]
		case "previous_port":
			s.PreviousPort, _ = strconv.Atoi(parts[1])
		case "previous_hash":
			s.PreviousHash = parts[1]
		}
	}
	return s, nil
}

// Write writes the app state to the server atomically via upload.
func Write(ctx context.Context, exec ssh.Executor, app string, s *AppState) error {
	content := fmt.Sprintf(
		"current_port=%d\ncurrent_hash=%s\nprevious_port=%d\nprevious_hash=%s\n",
		s.CurrentPort, s.CurrentHash, s.PreviousPort, s.PreviousHash,
	)
	path := fmt.Sprintf("%s/%s/state", deploymentsDir, app)
	return exec.Upload(ctx, strings.NewReader(content), path, "0644")
}

// LockInfo represents the metadata stored in a .lock directory.
type LockInfo struct {
	Type    string `json:"type"`              // "auto" or "manual"
	User    string `json:"user,omitempty"`
	Message string `json:"message,omitempty"`
	TS      string `json:"ts"`
}

// ReadLock reads the lock info for an app. Returns nil if no lock exists.
func ReadLock(ctx context.Context, exec ssh.Executor, app string) (*LockInfo, error) {
	lockPath := fmt.Sprintf("%s/%s/.lock/info", deploymentsDir, app)
	output, err := exec.Run(ctx, fmt.Sprintf("cat %s 2>/dev/null", lockPath))
	if err != nil || strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var info LockInfo
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &info); err != nil {
		return nil, nil
	}
	return &info, nil
}

// AcquireLock acquires the deploy lock for an app using atomic mkdir.
// Returns an error if a lock already exists (another deploy in progress or manual freeze).
func AcquireLock(ctx context.Context, exec ssh.Executor, app string) error {
	lockPath := fmt.Sprintf("%s/%s/.lock", deploymentsDir, app)
	if _, err := exec.Run(ctx, fmt.Sprintf("mkdir %s 2>/dev/null", lockPath)); err != nil {
		// Lock exists — read info for a descriptive error.
		info, _ := ReadLock(ctx, exec, app)
		if info != nil && info.Type == "manual" {
			msg := fmt.Sprintf("Deploy locked by %s", info.User)
			if info.Message != "" {
				msg += fmt.Sprintf(": '%s'", info.Message)
			}
			msg += fmt.Sprintf(". Locked at %s. Use 'teploy unlock' to release.", info.TS)
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("deploy is already in progress for %s", app)
	}

	info, _ := json.Marshal(LockInfo{
		Type: "auto",
		TS:   time.Now().UTC().Format(time.RFC3339),
	})
	if err := exec.Upload(ctx, bytes.NewReader(info), lockPath+"/info", "0644"); err != nil {
		// Lock directory was created but info file failed — release and return error.
		ReleaseLock(ctx, exec, app)
		return fmt.Errorf("writing lock info: %w", err)
	}
	return nil
}

// AcquireManualLock places a manual deploy freeze on an app.
func AcquireManualLock(ctx context.Context, exec ssh.Executor, app, user, message string) error {
	lockPath := fmt.Sprintf("%s/%s/.lock", deploymentsDir, app)
	if _, err := exec.Run(ctx, fmt.Sprintf("mkdir %s 2>/dev/null", lockPath)); err != nil {
		return fmt.Errorf("app %s is already locked", app)
	}

	info, _ := json.Marshal(LockInfo{
		Type:    "manual",
		User:    user,
		Message: message,
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	return exec.Upload(ctx, bytes.NewReader(info), lockPath+"/info", "0644")
}

// ReleaseLock releases the deploy lock for an app.
func ReleaseLock(ctx context.Context, exec ssh.Executor, app string) {
	lockPath := fmt.Sprintf("%s/%s/.lock", deploymentsDir, app)
	exec.Run(ctx, fmt.Sprintf("rm -rf %s", lockPath))
}

// AppendLog appends a deploy log entry to /deployments/teploy.log.
func AppendLog(ctx context.Context, exec ssh.Executor, entry LogEntry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling log entry: %w", err)
	}
	line = append(line, '\n')

	tmpPath := "/tmp/teploy_log_entry"
	if err := exec.Upload(ctx, bytes.NewReader(line), tmpPath, "0644"); err != nil {
		return fmt.Errorf("uploading log entry: %w", err)
	}
	if _, err := exec.Run(ctx, fmt.Sprintf("cat %s >> %s/teploy.log && rm -f %s", tmpPath, deploymentsDir, tmpPath)); err != nil {
		return fmt.Errorf("appending log entry: %w", err)
	}
	return nil
}

// EnsureAppDir creates /deployments/<app>/ if it doesn't exist.
func EnsureAppDir(ctx context.Context, exec ssh.Executor, app string) error {
	_, err := exec.Run(ctx, fmt.Sprintf("mkdir -p %s/%s", deploymentsDir, app))
	return err
}
