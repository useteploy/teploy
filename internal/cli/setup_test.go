package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/useteploy/teploy/internal/ssh"
)

func TestSetupServer(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "whoami", Output: "root"},
		ssh.MockCommand{Match: "docker --version", Output: "Docker version 24.0.0, build abc123"},
		ssh.MockCommand{Match: "ufw status", Err: fmt.Errorf("command not found")},
		ssh.MockCommand{Match: "systemctl is-active firewalld", Err: fmt.Errorf("inactive")},
		ssh.MockCommand{Match: "docker info", Output: ""},
		ssh.MockCommand{Match: "docker network", Output: "teploy"},
		ssh.MockCommand{Match: "mkdir", Output: ""},
		ssh.MockCommand{Match: "docker ps -a --filter name=", Output: ""},
		ssh.MockCommand{Match: "docker run", Output: "caddy_container_id"},
	)

	var buf bytes.Buffer
	if err := setupServer(context.Background(), mock, &buf, true); err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Docker already installed") {
		t.Error("should report Docker already installed")
	}
	if !strings.Contains(output, "No active firewall") {
		t.Error("should report no firewall")
	}
	if !strings.Contains(output, "Caddy started") {
		t.Error("should report Caddy started")
	}
	if !strings.Contains(output, "Server provisioned successfully") {
		t.Error("should report success")
	}

	// Verify Caddyfile was uploaded with correct content.
	content, ok := mock.Files["/deployments/caddy/Caddyfile"]
	if !ok {
		t.Fatal("Caddyfile not uploaded")
	}
	if !strings.Contains(string(content), "admin 0.0.0.0:2019") {
		t.Errorf("Caddyfile missing admin config, got: %s", string(content))
	}

	// Verify Caddy docker run command contains required flags.
	var caddyCmd string
	for _, call := range mock.Calls {
		if strings.Contains(call, "docker") && strings.Contains(call, "run") && strings.Contains(call, "caddy") {
			caddyCmd = call
		}
	}
	if caddyCmd == "" {
		t.Fatal("no docker run command found")
	}
	for _, want := range []string{
		"--restart always",
		"--name caddy",
		"--network teploy",
		"-p 80:80",
		"-p 443:443",
		"-p 127.0.0.1:2019:2019",
		"caddy_data:/data",
		"/deployments/caddy/Caddyfile:/etc/caddy/Caddyfile",
	} {
		if !strings.Contains(caddyCmd, want) {
			t.Errorf("Caddy command missing %q\ngot: %s", want, caddyCmd)
		}
	}
}

func TestSetupServer_InstallDocker(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "whoami", Output: "root"},
		ssh.MockCommand{Match: "docker --version", Err: fmt.Errorf("not found"), Once: true},
		ssh.MockCommand{Match: "which curl", Output: "/usr/bin/curl"},
		ssh.MockCommand{Match: "sh -c", Output: ""},                              // install stream
		ssh.MockCommand{Match: "docker --version", Output: "Docker version 24.0"}, // verify after install
		ssh.MockCommand{Match: "usermod", Output: ""},
		ssh.MockCommand{Match: "ufw status", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "systemctl", Err: fmt.Errorf("inactive")},
		ssh.MockCommand{Match: "docker info", Output: ""},
		ssh.MockCommand{Match: "docker network", Output: "teploy"},
		ssh.MockCommand{Match: "mkdir", Output: ""},
		ssh.MockCommand{Match: "chown", Output: ""},
		ssh.MockCommand{Match: "test -s /deployments/caddy/Caddyfile", Err: fmt.Errorf("no such file")},
		ssh.MockCommand{Match: "docker ps -a --filter name=", Output: ""},
		ssh.MockCommand{Match: "docker run", Output: "caddy_id"},
	)

	var buf bytes.Buffer
	if err := setupServer(context.Background(), mock, &buf, true); err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	if !strings.Contains(buf.String(), "Installing Docker") {
		t.Error("should report Docker installation")
	}
	if !strings.Contains(buf.String(), "Docker installed") {
		t.Error("should report Docker installed after verification")
	}
}

func TestSetupServer_CaddyAlreadyRunning(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "whoami", Output: "root"},
		ssh.MockCommand{Match: "docker --version", Output: "Docker version 24.0.0"},
		ssh.MockCommand{Match: "ufw status", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "systemctl", Err: fmt.Errorf("inactive")},
		ssh.MockCommand{Match: "docker info", Output: ""},
		ssh.MockCommand{Match: "docker network", Output: "teploy"},
		ssh.MockCommand{Match: "mkdir", Output: ""},
		ssh.MockCommand{Match: "chown", Output: ""},
		ssh.MockCommand{Match: "test -s /deployments/caddy/Caddyfile", Output: ""},
		ssh.MockCommand{Match: "docker ps -a --filter name=", Output: "caddy"},
		// Existing Caddy already launched with --resume: skip recreation.
		ssh.MockCommand{Match: "docker inspect -f", Output: "caddy run --config /etc/caddy/Caddyfile --adapter caddyfile --resume"},
	)

	var buf bytes.Buffer
	if err := setupServer(context.Background(), mock, &buf, true); err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	if !strings.Contains(buf.String(), "Caddy already running") {
		t.Error("should report Caddy already running")
	}

	for _, call := range mock.Calls {
		if strings.Contains(call, "docker") && strings.Contains(call, "run") && strings.Contains(call, "-d") {
			t.Error("should not start Caddy when already running")
		}
	}
}

func TestSetupServer_CaddyUpgradePreservesNetworksAndCaddyfile(t *testing.T) {
	// Simulates an existing server (e.g., post-Dokploy migration) where
	// Caddy was launched without --resume, the Caddyfile holds real
	// production routes, and Caddy is on multiple networks. Upgrade must
	// preserve Caddyfile, reattach non-teploy networks, and not silently
	// take the server down.
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "whoami", Output: "root"},
		ssh.MockCommand{Match: "docker --version", Output: "Docker version 24.0.0"},
		ssh.MockCommand{Match: "ufw status", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "systemctl", Err: fmt.Errorf("inactive")},
		ssh.MockCommand{Match: "docker info", Output: ""},
		ssh.MockCommand{Match: "docker network", Output: "teploy"},
		ssh.MockCommand{Match: "mkdir", Output: ""},
		ssh.MockCommand{Match: "chown", Output: ""},
		ssh.MockCommand{Match: "test -s /deployments/caddy/Caddyfile", Output: ""},
		ssh.MockCommand{Match: "docker ps -a --filter name=", Output: "caddy"},
		// Legacy Caddy cmd: no --resume, must upgrade.
		ssh.MockCommand{Match: "docker inspect -f '{{join .Config.Cmd", Output: "caddy run --config /etc/caddy/Caddyfile --adapter caddyfile"},
		// Extra networks the existing caddy is attached to.
		ssh.MockCommand{Match: "docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}", Output: "teploy dokploy-network bridge "},
		ssh.MockCommand{Match: "docker rm -f caddy", Output: ""},
		ssh.MockCommand{Match: "docker run", Output: "caddy_id"},
		ssh.MockCommand{Match: "docker network connect dokploy-network caddy", Output: ""},
		ssh.MockCommand{Match: "docker network connect bridge caddy", Output: ""},
	)

	var buf bytes.Buffer
	if err := setupServer(context.Background(), mock, &buf, true); err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	// The stub Caddyfile must NOT have been uploaded — the existing one is preserved.
	if _, uploaded := mock.Files["/deployments/caddy/Caddyfile"]; uploaded {
		t.Error("Caddyfile should have been preserved, not overwritten with stub")
	}
	if !strings.Contains(buf.String(), "Existing Caddyfile preserved") {
		t.Error("should report existing Caddyfile preserved")
	}

	// The upgraded container must launch with --resume and /config volume.
	var runCmd string
	for _, c := range mock.Calls {
		if strings.HasPrefix(c, "docker run") {
			runCmd = c
		}
	}
	for _, want := range []string{"--resume", "caddy_config:/config"} {
		if !strings.Contains(runCmd, want) {
			t.Errorf("recreated Caddy missing %q\ngot: %s", want, runCmd)
		}
	}

	// Must reattach dokploy-network and bridge, but not the base teploy network.
	foundDokploy, foundBridge, foundTeployReattach := false, false, false
	for _, c := range mock.Calls {
		if strings.Contains(c, "docker network connect dokploy-network caddy") {
			foundDokploy = true
		}
		if strings.Contains(c, "docker network connect bridge caddy") {
			foundBridge = true
		}
		if strings.Contains(c, "docker network connect teploy caddy") {
			foundTeployReattach = true
		}
	}
	if !foundDokploy {
		t.Error("should reattach dokploy-network to recreated Caddy")
	}
	if !foundBridge {
		t.Error("should reattach bridge network to recreated Caddy")
	}
	if foundTeployReattach {
		t.Error("should not reattach base teploy network — already attached via docker run")
	}
}

func TestSetupServer_UFWActive(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "whoami", Output: "root"},
		ssh.MockCommand{Match: "docker --version", Output: "Docker version 24.0.0"},
		ssh.MockCommand{Match: "ufw status", Output: "Status: active\n\nTo Action From\n22/tcp ALLOW Anywhere"},
		ssh.MockCommand{Match: "ufw allow 80", Output: "Rule added"},
		ssh.MockCommand{Match: "ufw allow 443", Output: "Rule added"},
		ssh.MockCommand{Match: "docker info", Output: ""},
		ssh.MockCommand{Match: "docker network", Output: "teploy"},
		ssh.MockCommand{Match: "mkdir", Output: ""},
		ssh.MockCommand{Match: "chown", Output: ""},
		ssh.MockCommand{Match: "test -s /deployments/caddy/Caddyfile", Err: fmt.Errorf("no such file")},
		ssh.MockCommand{Match: "docker ps -a --filter name=", Output: ""},
		ssh.MockCommand{Match: "docker run", Output: "caddy_id"},
	)

	var buf bytes.Buffer
	if err := setupServer(context.Background(), mock, &buf, true); err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	if !strings.Contains(buf.String(), "Opened ports 80 and 443") {
		t.Error("should report ports opened")
	}
}
