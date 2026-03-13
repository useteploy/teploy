package build

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// SyncConfig holds parameters for rsyncing source to the server.
type SyncConfig struct {
	LocalDir  string   // local source directory
	RemoteDir string   // remote destination directory
	Host      string   // SSH host
	User      string   // SSH user
	KeyPath   string   // SSH key path (optional)
	Excludes  []string // patterns to exclude
}

// Sync transfers the local directory to the remote server via rsync over SSH.
// Output is streamed to stdout in real time.
func Sync(ctx context.Context, cfg SyncConfig, stdout, stderr io.Writer) error {
	sshCmd := "ssh -o StrictHostKeyChecking=no"
	if cfg.KeyPath != "" {
		sshCmd += " -i " + cfg.KeyPath
	}

	// Ensure local dir has trailing slash so rsync copies contents, not the dir itself.
	localDir := strings.TrimRight(cfg.LocalDir, "/") + "/"

	args := []string{
		"-az", "--delete",
		"-e", sshCmd,
	}

	for _, pattern := range cfg.Excludes {
		args = append(args, "--exclude", pattern)
	}

	remote := fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, cfg.RemoteDir)
	args = append(args, localDir, remote)

	cmd := exec.CommandContext(ctx, "rsync", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}
	return nil
}
