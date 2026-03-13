package ssh

import (
	"context"
	"io"
)

// Executor defines the interface for running commands on a remote server.
// All server interaction in teploy goes through this interface.
// Tests use MockExecutor; production uses RemoteExecutor.
type Executor interface {
	// Run executes a command and returns the combined output.
	Run(ctx context.Context, cmd string) (string, error)

	// RunStream executes a command and streams stdout/stderr to the provided writers in real time.
	RunStream(ctx context.Context, cmd string, stdout, stderr io.Writer) error

	// Upload sends content to a remote file with the specified permissions.
	Upload(ctx context.Context, content io.Reader, remotePath string, mode string) error

	// Close closes the underlying SSH connection.
	Close() error

	// Host returns the target host address.
	Host() string

	// User returns the SSH user for this connection.
	User() string
}
