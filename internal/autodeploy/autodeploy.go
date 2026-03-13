package autodeploy

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/useteploy/teploy/internal/ssh"
)

const (
	deploymentsDir = "/deployments"
	scriptName     = "autodeploy.sh"
)

// Config holds auto-deploy configuration.
type Config struct {
	App    string
	Branch string // branch to watch (default "main")
	Secret string // webhook secret for validation
}

// Manager handles webhook auto-deploy setup on the server.
type Manager struct {
	exec ssh.Executor
	out  io.Writer
}

// sudoPrefix returns "sudo " if not running as root, empty string otherwise.
func (m *Manager) sudoPrefix(ctx context.Context) string {
	if id, err := m.exec.Run(ctx, "id -u"); err == nil && strings.TrimSpace(id) == "0" {
		return ""
	}
	return "sudo "
}

// NewManager creates an auto-deploy manager.
func NewManager(exec ssh.Executor, out io.Writer) *Manager {
	return &Manager{exec: exec, out: out}
}

// Setup installs the auto-deploy webhook handler on the server.
// Creates a lightweight shell script that:
//  1. Validates the webhook secret
//  2. Checks if the push is to the watched branch
//  3. Pulls the latest code and rebuilds
//
// Caddy is configured to route POST /teploy-webhook/<app> to the script
// via a simple exec handler using a systemd service.
func (m *Manager) Setup(ctx context.Context, cfg Config) error {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}

	appDir := fmt.Sprintf("%s/%s", deploymentsDir, cfg.App)
	scriptPath := fmt.Sprintf("%s/%s", appDir, scriptName)

	// Ensure app directory exists.
	if _, err := m.exec.Run(ctx, "mkdir -p "+appDir); err != nil {
		return fmt.Errorf("creating app directory: %w", err)
	}

	// Write the auto-deploy script.
	fmt.Fprintln(m.out, "Installing auto-deploy script...")
	script := generateScript(cfg)
	if err := m.exec.Upload(ctx, strings.NewReader(script), scriptPath, "0755"); err != nil {
		return fmt.Errorf("uploading deploy script: %w", err)
	}

	// Create systemd service for the webhook listener.
	fmt.Fprintln(m.out, "Setting up webhook listener...")
	serviceName := fmt.Sprintf("teploy-webhook-%s", cfg.App)
	listenerScript := generateListener(cfg.App, cfg.Secret, scriptPath)
	listenerPath := fmt.Sprintf("%s/webhook-listener.sh", appDir)

	if err := m.exec.Upload(ctx, strings.NewReader(listenerScript), listenerPath, "0755"); err != nil {
		return fmt.Errorf("uploading listener script: %w", err)
	}

	// Install and start systemd service.
	serviceContent := generateService(serviceName, listenerPath)
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)
	if err := m.exec.Upload(ctx, strings.NewReader(serviceContent), servicePath, "0644"); err != nil {
		return fmt.Errorf("uploading systemd service: %w", err)
	}

	sudo := m.sudoPrefix(ctx)
	cmds := []string{
		sudo + "systemctl daemon-reload",
		fmt.Sprintf("%ssystemctl enable %s", sudo, serviceName),
		fmt.Sprintf("%ssystemctl restart %s", sudo, serviceName),
	}
	for _, cmd := range cmds {
		if _, err := m.exec.Run(ctx, cmd); err != nil {
			return fmt.Errorf("setting up service: %w", err)
		}
	}

	fmt.Fprintf(m.out, "  Webhook listener running on port 9876\n")
	return nil
}

// SetupCaddyRoute adds a Caddy route to proxy webhook requests to the listener.
func (m *Manager) SetupCaddyRoute(ctx context.Context, app, domain string) error {
	fmt.Fprintln(m.out, "Adding Caddy webhook route...")

	// We add a route that matches POST to /teploy-webhook/{app} and proxies to the local listener.
	// This uses a direct curl to the Caddy admin API to add a webhook-specific route.
	webhookRoute := fmt.Sprintf(`{
		"@id": "teploy-webhook-%s",
		"match": [{"host": ["%s"], "path": ["/teploy-webhook/%s"]}],
		"handle": [{"handler": "reverse_proxy", "upstreams": [{"dial": "localhost:9876"}]}]
	}`, app, domain, app)

	uploadPath := "/tmp/teploy_webhook_route.json"
	if err := m.exec.Upload(ctx, strings.NewReader(webhookRoute), uploadPath, "0644"); err != nil {
		return fmt.Errorf("uploading webhook route: %w", err)
	}

	// Try to add the route to existing Caddy config.
	cmd := fmt.Sprintf(
		"curl -sf -X POST http://localhost:2019/config/apps/http/servers/srv0/routes -H 'Content-Type: application/json' -d @%s",
		uploadPath,
	)
	if _, err := m.exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("adding Caddy webhook route: %w", err)
	}

	m.exec.Run(ctx, "rm -f "+uploadPath)
	return nil
}

// Status checks if auto-deploy is set up for the app.
func (m *Manager) Status(ctx context.Context, app string) (bool, string, error) {
	serviceName := fmt.Sprintf("teploy-webhook-%s", app)
	out, err := m.exec.Run(ctx, fmt.Sprintf("systemctl is-active %s 2>/dev/null", serviceName))
	if err != nil {
		return false, "", nil
	}
	status := strings.TrimSpace(out)
	return status == "active", status, nil
}

// Remove disables and removes the auto-deploy webhook for the app.
func (m *Manager) Remove(ctx context.Context, app string) error {
	serviceName := fmt.Sprintf("teploy-webhook-%s", app)

	sudo := m.sudoPrefix(ctx)
	cmds := []string{
		fmt.Sprintf("%ssystemctl stop %s 2>/dev/null", sudo, serviceName),
		fmt.Sprintf("%ssystemctl disable %s 2>/dev/null", sudo, serviceName),
		fmt.Sprintf("%srm -f /etc/systemd/system/%s.service", sudo, serviceName),
		sudo + "systemctl daemon-reload",
	}
	for _, cmd := range cmds {
		m.exec.Run(ctx, cmd)
	}

	fmt.Fprintf(m.out, "Auto-deploy removed for %s\n", app)
	return nil
}

func generateScript(cfg Config) string {
	return fmt.Sprintf(`#!/bin/bash
# Auto-deploy script for %s
# Triggered by webhook on push to %s
set -e

APP="%s"
BRANCH="%s"
DEPLOY_DIR="/deployments/$APP/build"
LOG="/deployments/$APP/autodeploy.log"

echo "$(date -u '+%%Y-%%m-%%dT%%H:%%M:%%SZ') Auto-deploy triggered for $APP (branch: $BRANCH)" >> "$LOG"

cd "$DEPLOY_DIR" 2>/dev/null || { echo "No build directory" >> "$LOG"; exit 1; }

# Pull latest changes.
git fetch origin "$BRANCH" >> "$LOG" 2>&1
git reset --hard "origin/$BRANCH" >> "$LOG" 2>&1

# Detect build method and build.
if [ -f Dockerfile ]; then
    VERSION=$(git rev-parse --short HEAD)
    docker build -t "${APP}-build-${VERSION}" . >> "$LOG" 2>&1
else
    echo "No Dockerfile found" >> "$LOG"
    exit 1
fi

echo "$(date -u '+%%Y-%%m-%%dT%%H:%%M:%%SZ') Build complete: ${APP}-build-${VERSION}" >> "$LOG"
`, cfg.App, cfg.Branch, cfg.App, cfg.Branch)
}

func generateListener(app, secret, scriptPath string) string {
	secretCheck := ""
	if secret != "" {
		// Escape single quotes in the secret to prevent shell injection.
		escaped := strings.ReplaceAll(secret, "'", "'\\''")
		secretCheck = fmt.Sprintf(`
    # Validate webhook secret.
    SIGNATURE=$(echo "$BODY" | openssl dgst -sha256 -hmac '%s' | awk '{print $2}')
    EXPECTED="sha256=$SIGNATURE"
    if [ "$HTTP_X_HUB_SIGNATURE_256" != "$EXPECTED" ]; then
        echo "HTTP/1.1 403 Forbidden\r\n\r\nInvalid signature"
        continue
    fi`, escaped)
	}

	return fmt.Sprintf(`#!/bin/bash
# Webhook listener for %s
# Listens on port 9876, validates requests, triggers deploy script

while true; do
    echo -e "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nok" | \
    nc -l -p 9876 -q 1 | {
        read -r METHOD PATH VERSION
        BODY=""
        while IFS= read -r LINE; do
            LINE=$(echo "$LINE" | tr -d '\r')
            [ -z "$LINE" ] && break
        done
        read -r BODY
%s
        # Trigger deploy in background.
        nohup %s >> /deployments/%s/autodeploy.log 2>&1 &
    }
done
`, app, secretCheck, scriptPath, app)
}

func generateService(name, listenerPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Teploy webhook listener (%s)
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, name, listenerPath)
}
