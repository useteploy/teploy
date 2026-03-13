package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Compile-time check: RemoteExecutor implements Executor.
var _ Executor = (*RemoteExecutor)(nil)

// RemoteExecutor implements Executor using a real SSH connection.
type RemoteExecutor struct {
	client *ssh.Client
	host   string
	user   string
}

// ConnectConfig holds the parameters for establishing an SSH connection.
type ConnectConfig struct {
	Host    string // IP or hostname (with optional :port)
	User    string // SSH user (default: root)
	KeyPath string // Path to SSH private key (optional, tries defaults)
}

// Connect establishes an SSH connection and returns a RemoteExecutor.
func Connect(ctx context.Context, cfg ConnectConfig) (*RemoteExecutor, error) {
	if cfg.User == "" {
		cfg.User = "root"
	}

	host := cfg.Host
	if !strings.Contains(host, ":") {
		host = host + ":22"
	}

	signers, err := resolveSigners(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("resolving SSH keys: %w", err)
	}
	if len(signers) == 0 {
		return nil, fmt.Errorf("no SSH keys found; provide --key, set TEPLOY_SSH_KEY, or place a key at ~/.ssh/id_ed25519")
	}

	hostKeyCallback, err := defaultHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("loading known hosts: %w", err)
	}

	clientConfig := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		},
		HostKeyCallback: hostKeyCallback,
	}

	client, err := dialWithContext(ctx, "tcp", host, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.Host, err)
	}

	return &RemoteExecutor{client: client, host: cfg.Host, user: cfg.User}, nil
}

func (e *RemoteExecutor) Run(ctx context.Context, cmd string) (string, error) {
	var stdout, stderr bytes.Buffer
	if err := e.RunStream(ctx, cmd, &stdout, &stderr); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (e *RemoteExecutor) RunStream(ctx context.Context, cmd string, stdout, stderr io.Writer) error {
	session, err := e.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		_ = session.Close()
		return ctx.Err()
	}
}

func (e *RemoteExecutor) Upload(ctx context.Context, content io.Reader, remotePath string, mode string) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading upload content: %w", err)
	}

	// Use path (not filepath) — remote is always Linux.
	dir := path.Dir(remotePath)

	cmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod %s %s",
		shellQuote(dir),
		shellQuote(remotePath),
		shellQuote(mode),
		shellQuote(remotePath),
	)

	session, err := e.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session for upload: %w", err)
	}
	defer session.Close()

	session.Stdin = bytes.NewReader(data)

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("uploading %s: %w", remotePath, err)
	}
	return nil
}

func (e *RemoteExecutor) Close() error {
	return e.client.Close()
}

func (e *RemoteExecutor) Host() string {
	return e.host
}

func (e *RemoteExecutor) User() string {
	return e.user
}

// defaultHostKeyCallback returns a known_hosts-based callback, falling back
// to accept-all only if ~/.ssh/known_hosts does not exist.
func defaultHostKeyCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsPath); err != nil {
		// No known_hosts file — accept any host key.
		// This matches default ssh behavior for first connections.
		return ssh.InsecureIgnoreHostKey(), nil
	}

	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("parsing known_hosts: %w", err)
	}
	return callback, nil
}

// resolveSigners finds and loads SSH private keys.
func resolveSigners(keyPath string) ([]ssh.Signer, error) {
	var paths []string
	if keyPath != "" {
		paths = []string{keyPath}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		paths = []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_rsa"),
		}
	}

	var signers []ssh.Signer
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			var passphraseErr *ssh.PassphraseMissingError
			if errors.As(err, &passphraseErr) {
				signer, err = parseEncryptedKey(data, p)
				if err != nil {
					continue
				}
			} else {
				continue
			}
		}
		signers = append(signers, signer)
	}
	return signers, nil
}

func parseEncryptedKey(data []byte, keyPath string) (ssh.Signer, error) {
	fmt.Fprintf(os.Stderr, "Enter passphrase for %s: ", keyPath)
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("reading passphrase: %w", err)
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
}

func dialWithContext(ctx context.Context, network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
