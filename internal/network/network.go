package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/useteploy/teploy/internal/ssh"
)

// Provider defines the VPN mesh provider interface.
// All interaction with servers goes through ssh.Executor.
type Provider interface {
	// Install installs the VPN client on the server. No-op if already installed.
	Install(ctx context.Context, exec ssh.Executor) error

	// Join connects the server to the VPN mesh. No-op if already connected.
	Join(ctx context.Context, exec ssh.Executor) error

	// Status returns the VPN status output from the server.
	Status(ctx context.Context, exec ssh.Executor) (string, error)

	// GetIP returns the server's stable private VPN IP address.
	GetIP(ctx context.Context, exec ssh.Executor) (string, error)
}

// Config holds network configuration parsed from teploy.yml.
type Config struct {
	Provider string
	AuthKey  string
	Server   string
	SetupKey string
}

// NewProvider creates a Provider from the given config.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "tailscale":
		return &TailscaleProvider{AuthKey: cfg.AuthKey}, nil
	case "headscale":
		return &HeadscaleProvider{Server: cfg.Server, AuthKey: cfg.AuthKey}, nil
	case "netbird":
		return &NetbirdProvider{SetupKey: cfg.SetupKey}, nil
	default:
		return nil, fmt.Errorf("unknown network provider: %q (supported: tailscale, headscale, netbird)", cfg.Provider)
	}
}

// --- Tailscale ---

// TailscaleProvider manages Tailscale VPN on servers.
type TailscaleProvider struct {
	AuthKey string
}

func (t *TailscaleProvider) Install(ctx context.Context, exec ssh.Executor) error {
	if _, err := exec.Run(ctx, "which tailscale"); err == nil {
		return nil // already installed
	}
	_, err := exec.Run(ctx, "curl -fsSL https://tailscale.com/install.sh | sh")
	if err != nil {
		return fmt.Errorf("installing tailscale: %w", err)
	}
	return nil
}

func (t *TailscaleProvider) Join(ctx context.Context, exec ssh.Executor) error {
	out, err := exec.Run(ctx, "tailscale status --json 2>/dev/null")
	if err == nil && strings.Contains(out, `"BackendState":"Running"`) {
		return nil // already connected
	}
	cmd := fmt.Sprintf("tailscale up --authkey=%q --accept-routes", t.AuthKey)
	if _, err := exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("joining tailscale mesh: %w", err)
	}
	return nil
}

func (t *TailscaleProvider) Status(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "tailscale status")
	if err != nil {
		return "", fmt.Errorf("getting tailscale status: %w", err)
	}
	return out, nil
}

func (t *TailscaleProvider) GetIP(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "tailscale ip -4")
	if err != nil {
		return "", fmt.Errorf("getting tailscale IP: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// --- Headscale ---

// HeadscaleProvider manages Headscale (self-hosted Tailscale) VPN on servers.
// Headscale uses the standard tailscale client pointed at a custom coordination server.
type HeadscaleProvider struct {
	Server  string
	AuthKey string
}

func (h *HeadscaleProvider) Install(ctx context.Context, exec ssh.Executor) error {
	// Headscale uses the tailscale client
	if _, err := exec.Run(ctx, "which tailscale"); err == nil {
		return nil
	}
	_, err := exec.Run(ctx, "curl -fsSL https://tailscale.com/install.sh | sh")
	if err != nil {
		return fmt.Errorf("installing tailscale client for headscale: %w", err)
	}
	return nil
}

func (h *HeadscaleProvider) Join(ctx context.Context, exec ssh.Executor) error {
	out, err := exec.Run(ctx, "tailscale status --json 2>/dev/null")
	if err == nil && strings.Contains(out, `"BackendState":"Running"`) {
		return nil
	}
	cmd := fmt.Sprintf("tailscale up --login-server=%q --authkey=%q --accept-routes", h.Server, h.AuthKey)
	if _, err := exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("joining headscale mesh: %w", err)
	}
	return nil
}

func (h *HeadscaleProvider) Status(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "tailscale status")
	if err != nil {
		return "", fmt.Errorf("getting headscale status: %w", err)
	}
	return out, nil
}

func (h *HeadscaleProvider) GetIP(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "tailscale ip -4")
	if err != nil {
		return "", fmt.Errorf("getting headscale IP: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// --- Netbird ---

// NetbirdProvider manages Netbird VPN on servers.
type NetbirdProvider struct {
	SetupKey string
}

func (n *NetbirdProvider) Install(ctx context.Context, exec ssh.Executor) error {
	if _, err := exec.Run(ctx, "which netbird"); err == nil {
		return nil
	}
	_, err := exec.Run(ctx, "curl -fsSL https://pkgs.netbird.io/install.sh | sh")
	if err != nil {
		return fmt.Errorf("installing netbird: %w", err)
	}
	return nil
}

func (n *NetbirdProvider) Join(ctx context.Context, exec ssh.Executor) error {
	out, err := exec.Run(ctx, "netbird status 2>/dev/null")
	if err == nil && strings.Contains(out, "Connected") {
		return nil
	}
	cmd := fmt.Sprintf("netbird up --setup-key %q", n.SetupKey)
	if _, err := exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("joining netbird mesh: %w", err)
	}
	return nil
}

func (n *NetbirdProvider) Status(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "netbird status")
	if err != nil {
		return "", fmt.Errorf("getting netbird status: %w", err)
	}
	return out, nil
}

func (n *NetbirdProvider) GetIP(ctx context.Context, exec ssh.Executor) (string, error) {
	out, err := exec.Run(ctx, "netbird status")
	if err != nil {
		return "", fmt.Errorf("getting netbird status for IP: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "NetBird IP:") || strings.HasPrefix(trimmed, "IP:") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				ip := parts[len(parts)-1]
				// Strip CIDR suffix if present (e.g., "100.64.0.1/32" -> "100.64.0.1")
				if idx := strings.Index(ip, "/"); idx != -1 {
					ip = ip[:idx]
				}
				return ip, nil
			}
		}
	}
	return "", fmt.Errorf("could not determine netbird IP from status output")
}
