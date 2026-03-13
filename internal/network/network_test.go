package network

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/teploy/teploy/internal/ssh"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		provider string
		wantType string
	}{
		{"tailscale", "*network.TailscaleProvider"},
		{"headscale", "*network.HeadscaleProvider"},
		{"netbird", "*network.NetbirdProvider"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			p, err := NewProvider(Config{Provider: tt.provider})
			if err != nil {
				t.Fatalf("NewProvider(%q): unexpected error: %v", tt.provider, err)
			}
			got := fmt.Sprintf("%T", p)
			if got != tt.wantType {
				t.Errorf("NewProvider(%q) = %s, want %s", tt.provider, got, tt.wantType)
			}
		})
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(Config{Provider: "wireguard"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown network provider") {
		t.Errorf("error should mention unknown provider, got: %v", err)
	}
	if !strings.Contains(err.Error(), "wireguard") {
		t.Errorf("error should include the attempted provider name, got: %v", err)
	}
}

func TestNewProvider_Empty(t *testing.T) {
	_, err := NewProvider(Config{Provider: ""})
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestTailscaleInstall_AlreadyInstalled(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "which tailscale", Output: "/usr/bin/tailscale"},
	)

	p := &TailscaleProvider{AuthKey: "tskey-auth-xxx"}
	if err := p.Install(context.Background(), mock); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Should only have called "which tailscale", not the install script.
	if len(mock.Calls) != 1 {
		t.Errorf("expected 1 call (which), got %d: %v", len(mock.Calls), mock.Calls)
	}
}

func TestTailscaleInstall_Fresh(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "which tailscale", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "curl -fsSL https://tailscale.com/install.sh", Output: "installed"},
	)

	p := &TailscaleProvider{AuthKey: "tskey-auth-xxx"}
	if err := p.Install(context.Background(), mock); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(mock.Calls) != 2 {
		t.Errorf("expected 2 calls, got %d: %v", len(mock.Calls), mock.Calls)
	}
}

func TestTailscaleJoin_AlreadyConnected(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "tailscale status --json", Output: `{"BackendState":"Running"}`},
	)

	p := &TailscaleProvider{AuthKey: "tskey-auth-xxx"}
	if err := p.Join(context.Background(), mock); err != nil {
		t.Fatalf("Join: %v", err)
	}

	// Should not call "tailscale up" since already running.
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "tailscale up") {
			t.Error("should not call 'tailscale up' when already connected")
		}
	}
}

func TestTailscaleJoin_Fresh(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "tailscale status --json", Err: fmt.Errorf("not running")},
		ssh.MockCommand{Match: "tailscale up", Output: ""},
	)

	p := &TailscaleProvider{AuthKey: "tskey-auth-xxx"}
	if err := p.Join(context.Background(), mock); err != nil {
		t.Fatalf("Join: %v", err)
	}

	var foundUp bool
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "tailscale up") {
			foundUp = true
			if !strings.Contains(call, "--authkey=") || !strings.Contains(call, "tskey-auth-xxx") {
				t.Errorf("tailscale up should include authkey, got: %s", call)
			}
			if !strings.Contains(call, "--accept-routes") {
				t.Errorf("tailscale up should include --accept-routes, got: %s", call)
			}
		}
	}
	if !foundUp {
		t.Error("expected 'tailscale up' call")
	}
}

func TestTailscaleGetIP(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "tailscale ip -4", Output: "100.64.0.1\n"},
	)

	p := &TailscaleProvider{AuthKey: "tskey-auth-xxx"}
	ip, err := p.GetIP(context.Background(), mock)
	if err != nil {
		t.Fatalf("GetIP: %v", err)
	}
	if ip != "100.64.0.1" {
		t.Errorf("GetIP = %q, want %q", ip, "100.64.0.1")
	}
}

func TestHeadscaleInstall_AlreadyInstalled(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "which tailscale", Output: "/usr/bin/tailscale"},
	)

	p := &HeadscaleProvider{Server: "https://headscale.example.com", AuthKey: "key123"}
	if err := p.Install(context.Background(), mock); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Errorf("expected 1 call, got %d: %v", len(mock.Calls), mock.Calls)
	}
}

func TestHeadscaleJoin_UsesLoginServer(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "tailscale status --json", Err: fmt.Errorf("not running")},
		ssh.MockCommand{Match: "tailscale up", Output: ""},
	)

	p := &HeadscaleProvider{Server: "https://headscale.example.com", AuthKey: "key123"}
	if err := p.Join(context.Background(), mock); err != nil {
		t.Fatalf("Join: %v", err)
	}

	var foundUp bool
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "tailscale up") {
			foundUp = true
			if !strings.Contains(call, "--login-server=") || !strings.Contains(call, "headscale.example.com") {
				t.Errorf("should use --login-server, got: %s", call)
			}
			if !strings.Contains(call, "--authkey=") || !strings.Contains(call, "key123") {
				t.Errorf("should include authkey, got: %s", call)
			}
		}
	}
	if !foundUp {
		t.Error("expected 'tailscale up' call")
	}
}

func TestNetbirdInstall_AlreadyInstalled(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "which netbird", Output: "/usr/bin/netbird"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	if err := p.Install(context.Background(), mock); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Errorf("expected 1 call, got %d: %v", len(mock.Calls), mock.Calls)
	}
}

func TestNetbirdInstall_Fresh(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "which netbird", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "curl -fsSL https://pkgs.netbird.io/install.sh", Output: "installed"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	if err := p.Install(context.Background(), mock); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(mock.Calls) != 2 {
		t.Errorf("expected 2 calls, got %d: %v", len(mock.Calls), mock.Calls)
	}
}

func TestNetbirdJoin_AlreadyConnected(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "netbird status", Output: "Status: Connected\nIP: 100.64.0.5/32"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	if err := p.Join(context.Background(), mock); err != nil {
		t.Fatalf("Join: %v", err)
	}

	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "netbird up") {
			t.Error("should not call 'netbird up' when already connected")
		}
	}
}

func TestNetbirdJoin_Fresh(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "netbird status", Err: fmt.Errorf("not running")},
		ssh.MockCommand{Match: "netbird up", Output: ""},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	if err := p.Join(context.Background(), mock); err != nil {
		t.Fatalf("Join: %v", err)
	}

	var foundUp bool
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "netbird up") {
			foundUp = true
			if !strings.Contains(call, "--setup-key") || !strings.Contains(call, "nb-setup-xxx") {
				t.Errorf("should include setup key, got: %s", call)
			}
		}
	}
	if !foundUp {
		t.Error("expected 'netbird up' call")
	}
}

func TestNetbirdGetIP(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "netbird status", Output: "  NetBird IP: 100.64.0.5/32\n  Status: Connected"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	ip, err := p.GetIP(context.Background(), mock)
	if err != nil {
		t.Fatalf("GetIP: %v", err)
	}
	if ip != "100.64.0.5" {
		t.Errorf("GetIP = %q, want %q", ip, "100.64.0.5")
	}
}

func TestNetbirdGetIP_ShortPrefix(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "netbird status", Output: "  IP: 100.64.0.9/32\n  Status: Connected"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	ip, err := p.GetIP(context.Background(), mock)
	if err != nil {
		t.Fatalf("GetIP: %v", err)
	}
	if ip != "100.64.0.9" {
		t.Errorf("GetIP = %q, want %q", ip, "100.64.0.9")
	}
}

func TestNetbirdGetIP_NoIP(t *testing.T) {
	mock := ssh.NewMockExecutor("server1",
		ssh.MockCommand{Match: "netbird status", Output: "Status: Disconnected\n"},
	)

	p := &NetbirdProvider{SetupKey: "nb-setup-xxx"}
	_, err := p.GetIP(context.Background(), mock)
	if err == nil {
		t.Fatal("expected error when IP not found in status output")
	}
	if !strings.Contains(err.Error(), "could not determine netbird IP") {
		t.Errorf("unexpected error: %v", err)
	}
}
