package deploy

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/useteploy/teploy/internal/caddy"
	"github.com/useteploy/teploy/internal/docker"
	"github.com/useteploy/teploy/internal/ssh"
	"github.com/useteploy/teploy/internal/state"
)

// Config holds all parameters for a deploy.
type Config struct {
	App         string
	Domain      string
	Image       string
	Version     string // short git hash or tag
	EnvFile     string
	Env         map[string]string
	Volumes     map[string]string
	Cmd         string            // command override for single-process deploys
	Processes   map[string]string // process_name -> command (overrides Cmd)
	Memory        string
	CPU           string
	ContainerPort int // internal container port (default 80)
	StopTimeout   int // graceful shutdown seconds (default 10)
	Health      HealthConfig
	PreDeploy   string // hook: runs in web container before traffic switch (failure aborts)
	PostDeploy  string // hook: runs in web container after traffic switch (failure warns)
	AssetPath     string // container path for asset bridging (e.g. "/app/public/assets")
	AssetKeepDays int    // cleanup bridged assets older than N days (default 7)
}

// Deployer orchestrates zero-downtime deploys.
type Deployer struct {
	exec   ssh.Executor
	docker *docker.Client
	caddy  *caddy.Client
	out    io.Writer
}

// NewDeployer creates a new deploy orchestrator.
func NewDeployer(exec ssh.Executor, out io.Writer) *Deployer {
	return &Deployer{
		exec:   exec,
		docker: docker.NewClient(exec),
		caddy:  caddy.NewClient(exec),
		out:    out,
	}
}

// Deploy performs a zero-downtime deploy.
//
// Flow: lock → start web → health check → start workers → route traffic →
// write state → stop old containers → log → unlock.
func (d *Deployer) Deploy(ctx context.Context, cfg Config) error {
	if cfg.App == "" || cfg.Image == "" || cfg.Version == "" || cfg.Domain == "" {
		return fmt.Errorf("app, image, version, and domain are required")
	}

	stopTimeout := cfg.StopTimeout
	if stopTimeout == 0 {
		stopTimeout = 10
	}

	// Determine processes. Default: single web process with image CMD.
	processes := cfg.Processes
	if len(processes) == 0 {
		processes = map[string]string{"web": cfg.Cmd}
	}

	start := time.Now()
	webContainerName := docker.ContainerName(cfg.App, "web", cfg.Version)

	// 1. Ensure app directory exists.
	fmt.Fprintf(d.out, "Deploying %s (version %s)...\n", cfg.App, cfg.Version)
	if err := state.EnsureAppDir(ctx, d.exec, cfg.App); err != nil {
		return fmt.Errorf("creating app directory: %w", err)
	}

	// 2. Acquire deploy lock.
	if err := state.AcquireLock(ctx, d.exec, cfg.App); err != nil {
		return err
	}
	defer state.ReleaseLock(ctx, d.exec, cfg.App)

	// 3. Read current state.
	current, _ := state.Read(ctx, d.exec, cfg.App)

	// 4. Allocate port (for web process).
	fmt.Fprintln(d.out, "Allocating port...")
	port, err := d.docker.FindAvailablePort(ctx)
	if err != nil {
		return fmt.Errorf("allocating port: %w", err)
	}
	fmt.Fprintf(d.out, "  Port %d allocated\n", port)

	// 5. Asset bridging: extract assets from image before starting the container.
	if cfg.AssetPath != "" {
		hostAssetDir := fmt.Sprintf("/deployments/%s/assets", cfg.App)
		fmt.Fprintln(d.out, "Bridging assets...")
		if _, err := d.exec.Run(ctx, "mkdir -p "+hostAssetDir); err != nil {
			return fmt.Errorf("creating asset bridge directory: %w", err)
		}

		// Extract assets from image using a one-shot container.
		extractCmd := fmt.Sprintf(
			"docker run --rm -v %s:/bridge %s sh -c 'cp -r %s/. /bridge/ 2>/dev/null || true'",
			hostAssetDir, cfg.Image, cfg.AssetPath,
		)
		if _, err := d.exec.Run(ctx, extractCmd); err != nil {
			fmt.Fprintf(d.out, "  Warning: asset extraction failed: %v\n", err)
		} else {
			fmt.Fprintln(d.out, "  Assets extracted to host")
		}

		// Mount the shared asset directory into the container.
		if cfg.Volumes == nil {
			cfg.Volumes = map[string]string{}
		}
		cfg.Volumes[hostAssetDir] = cfg.AssetPath
	}

	// 6. Handle same-version redeploy: rename existing containers to avoid name conflicts.
	if current != nil && current.CurrentHash == cfg.Version {
		for _, process := range sortedProcessNames(processes) {
			name := docker.ContainerName(cfg.App, process, cfg.Version)
			d.exec.Run(ctx, fmt.Sprintf("docker rename %s %s 2>/dev/null", name, name+"_replaced"))
		}
	}

	// Track started containers for cleanup on failure.
	var started []string

	// 6. Start web container.
	fmt.Fprintf(d.out, "Starting container %s...\n", webContainerName)
	containerID, err := d.docker.Run(ctx, docker.RunConfig{
		App:           cfg.App,
		Process:       "web",
		Version:       cfg.Version,
		Image:         cfg.Image,
		Port:          port,
		ContainerPort: cfg.ContainerPort,
		EnvFile:       cfg.EnvFile,
		Env:           cfg.Env,
		Volumes:       cfg.Volumes,
		Cmd:           processes["web"],
		Memory:        cfg.Memory,
		CPU:           cfg.CPU,
	})
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}
	started = append(started, webContainerName)
	fmt.Fprintf(d.out, "  Container %s started\n", containerID[:min(12, len(containerID))])

	// From here on, failures must clean up all started containers.
	fail := func(reason error) error {
		logs, _ := d.exec.Run(ctx, fmt.Sprintf("docker logs --tail 50 %s 2>&1", webContainerName))
		if logs != "" {
			fmt.Fprintf(d.out, "\n--- Container logs ---\n%s\n--- End logs ---\n", logs)
		}
		for _, name := range started {
			d.docker.Stop(ctx, name, 5)
			d.docker.Remove(ctx, name)
		}
		d.logDeploy(ctx, cfg, false, start)
		return reason
	}

	// 7. Verify web container is running (catch immediate crashes).
	statusOut, err := d.exec.Run(ctx, fmt.Sprintf(
		"docker inspect -f '{{.State.Status}}' %s", webContainerName,
	))
	if err != nil || strings.TrimSpace(statusOut) != "running" {
		return fail(fmt.Errorf("container failed to start (status: %s)", strings.TrimSpace(statusOut)))
	}

	// 8. Pre-deploy hook (runs in web container before health check and traffic switch).
	if cfg.PreDeploy != "" {
		fmt.Fprintf(d.out, "Running pre-deploy hook...\n")
		if output, err := d.docker.Exec(ctx, webContainerName, cfg.PreDeploy); err != nil {
			if output != "" {
				fmt.Fprintf(d.out, "  %s\n", output)
			}
			return fail(fmt.Errorf("pre-deploy hook failed: %w", err))
		}
		fmt.Fprintln(d.out, "  Pre-deploy hook passed")
	}

	// 9. Health check (web process only).
	fmt.Fprintln(d.out, "Running health check...")
	healthCfg := cfg.Health.withDefaults()
	if err := d.healthCheck(ctx, port, healthCfg); err != nil {
		fmt.Fprintf(d.out, "  Health check failed: %v\n", err)
		return fail(fmt.Errorf("health check failed: %w", err))
	}
	fmt.Fprintln(d.out, "  Health check passed")

	// 10. Start non-web process containers (workers, etc.).
	for _, process := range sortedProcessNames(processes) {
		if process == "web" {
			continue
		}
		name := docker.ContainerName(cfg.App, process, cfg.Version)
		fmt.Fprintf(d.out, "Starting %s...\n", name)
		_, err := d.docker.Run(ctx, docker.RunConfig{
			App:     cfg.App,
			Process: process,
			Version: cfg.Version,
			Image:   cfg.Image,
			Port:    0, // non-web processes don't get a port
			EnvFile: cfg.EnvFile,
			Env:     cfg.Env,
			Volumes: cfg.Volumes,
			Cmd:     processes[process],
			Memory:  cfg.Memory,
			CPU:     cfg.CPU,
		})
		if err != nil {
			return fail(fmt.Errorf("starting %s: %w", name, err))
		}
		started = append(started, name)
	}

	// 11. Update Caddy route to point at new web container.
	fmt.Fprintln(d.out, "Updating routes...")
	if err := d.caddy.SetRoute(ctx, cfg.App, cfg.Domain, port); err != nil {
		return fail(fmt.Errorf("updating route: %w", err))
	}
	fmt.Fprintln(d.out, "  Traffic routed to new container")

	// 12. Post-deploy hook (runs in web container after traffic switch — failure warns, no rollback).
	if cfg.PostDeploy != "" {
		fmt.Fprintf(d.out, "Running post-deploy hook...\n")
		if output, err := d.docker.Exec(ctx, webContainerName, cfg.PostDeploy); err != nil {
			fmt.Fprintf(d.out, "  Warning: post-deploy hook failed: %v\n", err)
			if output != "" {
				fmt.Fprintf(d.out, "  %s\n", output)
			}
		} else {
			fmt.Fprintln(d.out, "  Post-deploy hook passed")
		}
	}

	// 13. Write new state.
	newState := &state.AppState{
		CurrentPort: port,
		CurrentHash: cfg.Version,
	}
	if current != nil {
		newState.PreviousPort = current.CurrentPort
		newState.PreviousHash = current.CurrentHash
	}
	stateErr := state.Write(ctx, d.exec, cfg.App, newState)
	if stateErr != nil {
		fmt.Fprintf(d.out, "Warning: writing state failed: %v\n", stateErr)
	}

	// 14. Stop old containers (all processes) — proceed even if state write failed,
	// since traffic is already routed to the new container.
	if current != nil && current.CurrentHash != "" {
		for _, process := range sortedProcessNames(processes) {
			oldName := docker.ContainerName(cfg.App, process, current.CurrentHash)
			if current.CurrentHash == cfg.Version {
				oldName += "_replaced"
			}
			fmt.Fprintf(d.out, "Stopping old container %s...\n", oldName)
			d.docker.Stop(ctx, oldName, stopTimeout)
		}
	}

	if stateErr != nil {
		return fmt.Errorf("writing state: %w", stateErr)
	}

	// 15. Clean up old bridged assets.
	if cfg.AssetPath != "" {
		keepDays := cfg.AssetKeepDays
		if keepDays <= 0 {
			keepDays = 7
		}
		cleanCmd := fmt.Sprintf(
			"find /deployments/%s/assets -type f -mtime +%d -delete 2>/dev/null || true",
			cfg.App, keepDays,
		)
		d.exec.Run(ctx, cleanCmd)
	}

	// 16. Log success.
	d.logDeploy(ctx, cfg, true, start)

	duration := time.Since(start)
	fmt.Fprintf(d.out, "\nDeployed %s version %s in %s\n", cfg.App, cfg.Version, duration.Round(time.Millisecond))
	return nil
}

func (d *Deployer) logDeploy(ctx context.Context, cfg Config, success bool, start time.Time) {
	state.AppendLog(ctx, d.exec, state.LogEntry{
		Timestamp:  time.Now().UTC(),
		App:        cfg.App,
		Type:       "deploy",
		Hash:       cfg.Version,
		Success:    success,
		DurationMs: time.Since(start).Milliseconds(),
	})
}

// sortedProcessNames returns process names with "web" first, then alphabetical.
func sortedProcessNames(processes map[string]string) []string {
	var others []string
	for name := range processes {
		if name != "web" {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	return append([]string{"web"}, others...)
}
