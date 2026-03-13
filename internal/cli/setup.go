package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/useteploy/teploy/internal/config"
	"github.com/useteploy/teploy/internal/ssh"
)

func newSetupCmd(flags *Flags) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "setup <host>",
		Short: "Provision a server for teploy",
		Long:  "Install Docker, configure firewall, start Caddy, and prepare a server for deployments.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(flags, args[0], name)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "server name for servers.yml (default: host address)")

	return cmd
}

func runSetup(flags *Flags, host string, name string) error {
	user := flags.User
	if user == "" {
		user = "root"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("Connecting to %s...\n", host)

	executor, err := ssh.Connect(ctx, ssh.ConnectConfig{
		Host:    host,
		User:    user,
		KeyPath: flags.Key,
	})
	if err != nil {
		return err
	}
	defer executor.Close()

	if err := setupServer(ctx, executor, os.Stdout); err != nil {
		return err
	}

	// Add to servers.yml
	if name == "" {
		name = host
	}
	serversPath, err := config.DefaultServersPath()
	if err != nil {
		return err
	}
	if err := config.AddServer(serversPath, name, host, user, ""); err != nil {
		return err
	}

	fmt.Printf("\nServer %q (%s) ready for deploys\n", name, host)
	return nil
}

// setupServer runs the provisioning steps on a connected server.
// Separated from runSetup for testability with MockExecutor.
func setupServer(ctx context.Context, exec ssh.Executor, w io.Writer) error {
	// Detect whether we need sudo (non-root users).
	sudo := ""
	if whoami, _ := exec.Run(ctx, "whoami"); strings.TrimSpace(whoami) != "root" {
		sudo = "sudo "
	}

	// 1. Check/install Docker
	fmt.Fprintln(w, "Checking Docker...")
	if _, err := exec.Run(ctx, "docker --version"); err != nil {
		fmt.Fprintln(w, "  Installing Docker...")

		// Try curl first, fall back to wget.
		installCmd := sudo + "sh -c 'curl -fsSL https://get.docker.com | sh'"
		if _, curlErr := exec.Run(ctx, "which curl"); curlErr != nil {
			installCmd = sudo + "sh -c 'wget -qO- https://get.docker.com | sh'"
		}

		if err := exec.RunStream(ctx, installCmd, w, w); err != nil {
			return fmt.Errorf("installing docker: %w", err)
		}

		// Verify Docker actually installed.
		if _, err := exec.Run(ctx, "docker --version"); err != nil {
			return fmt.Errorf("docker install appeared to succeed but docker is not available")
		}
		fmt.Fprintln(w, "  Docker installed")

		// Add current user to docker group so sudo isn't needed for docker commands.
		exec.Run(ctx, sudo+"usermod -aG docker $(whoami)")
	} else {
		fmt.Fprintln(w, "  Docker already installed")
	}

	// 2. Check firewall
	fmt.Fprintln(w, "Checking firewall...")
	ufwOutput, ufwErr := exec.Run(ctx, "ufw status 2>/dev/null")
	if ufwErr == nil && strings.Contains(ufwOutput, "Status: active") {
		_, err1 := exec.Run(ctx, sudo+"ufw allow 80/tcp")
		_, err2 := exec.Run(ctx, sudo+"ufw allow 443/tcp")
		if err1 != nil || err2 != nil {
			fmt.Fprintln(w, "  Warning: could not configure ufw. Ensure ports 80 and 443 are open.")
		} else {
			fmt.Fprintln(w, "  Opened ports 80 and 443 (ufw)")
		}
	} else if _, err := exec.Run(ctx, "systemctl is-active firewalld 2>/dev/null"); err == nil {
		fmt.Fprintln(w, "  Warning: firewalld detected. Ensure ports 80 and 443 are open.")
	} else {
		fmt.Fprintln(w, "  No active firewall detected")
	}

	// Determine docker prefix: use sudo if user can't access docker directly.
	dockerCmd := "docker"
	if _, err := exec.Run(ctx, "docker info >/dev/null 2>&1"); err != nil {
		dockerCmd = sudo + "docker"
	}

	// 3. Create Docker network
	fmt.Fprintln(w, "Creating Docker network...")
	netCmd := dockerCmd + " network inspect teploy >/dev/null 2>&1 || " + dockerCmd + " network create teploy"
	if _, err := exec.Run(ctx, netCmd); err != nil {
		return fmt.Errorf("creating docker network: %w", err)
	}

	// 4. Create directories and upload Caddyfile
	if _, err := exec.Run(ctx, sudo+"mkdir -p /deployments/caddy"); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	// Ensure deploy user owns the directory.
	exec.Run(ctx, sudo+"chown -R $(whoami):$(whoami) /deployments")

	// Caddy admin API listens on 0.0.0.0 inside container so Docker port
	// forwarding can reach it. Port 2019 is only published to 127.0.0.1
	// on the host — never publicly accessible.
	caddyfile := "{\n    admin 0.0.0.0:2019\n}\n"
	if err := exec.Upload(ctx, strings.NewReader(caddyfile), "/deployments/caddy/Caddyfile", "0644"); err != nil {
		return fmt.Errorf("uploading Caddyfile: %w", err)
	}

	// 5. Start Caddy (idempotent — skip if container already exists)
	fmt.Fprintln(w, "Starting Caddy...")
	caddyCheck, _ := exec.Run(ctx, dockerCmd+" ps -a --filter name=^caddy$ --format '{{.Names}}'")
	if strings.TrimSpace(caddyCheck) == "" {
		caddyRun := strings.Join([]string{
			dockerCmd, "run", "-d",
			"--restart", "always",
			"--name", "caddy",
			"--network", "teploy",
			"-p", "80:80",
			"-p", "443:443",
			"-p", "127.0.0.1:2019:2019",
			"-v", "caddy_data:/data",
			"-v", "/deployments/caddy/Caddyfile:/etc/caddy/Caddyfile",
			"caddy",
		}, " ")
		if _, err := exec.Run(ctx, caddyRun); err != nil {
			return fmt.Errorf("starting caddy: %w", err)
		}
		fmt.Fprintln(w, "  Caddy started")
	} else {
		fmt.Fprintln(w, "  Caddy already running")
	}

	fmt.Fprintln(w, "Server provisioned successfully")
	return nil
}
