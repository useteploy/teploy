package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/teploy/teploy/internal/ssh"
)

// Mode represents the detected build method.
type Mode int

const (
	ModeNone       Mode = iota // pre-built image, no build needed
	ModeDockerfile             // Dockerfile found
	ModeNixpacks               // no Dockerfile, use Nixpacks
)

func (m Mode) String() string {
	switch m {
	case ModeDockerfile:
		return "dockerfile"
	case ModeNixpacks:
		return "nixpacks"
	default:
		return "none"
	}
}

// Detect examines the directory and returns the appropriate build mode.
// Priority: Dockerfile → Nixpacks fallback.
func Detect(dir string) Mode {
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
		return ModeDockerfile
	}
	return ModeNixpacks
}

// ImageTag returns the image tag used for server-built images.
func ImageTag(app, version string) string {
	return app + "-build-" + version
}

// BuildConfig holds parameters for a server-side build.
type BuildConfig struct {
	App      string
	Version  string
	Mode     Mode
	BuildDir string // remote directory containing the source
	Platform string // e.g. "linux/arm64" (optional)
}

// Builder runs Docker or Nixpacks builds on the server via SSH.
type Builder struct {
	exec   ssh.Executor
	stdout io.Writer
}

// NewBuilder creates a Builder backed by the given SSH executor.
func NewBuilder(exec ssh.Executor, stdout io.Writer) *Builder {
	return &Builder{exec: exec, stdout: stdout}
}

// Build runs the appropriate build command on the server and returns the image tag.
func (b *Builder) Build(ctx context.Context, cfg BuildConfig) (string, error) {
	tag := ImageTag(cfg.App, cfg.Version)

	switch cfg.Mode {
	case ModeDockerfile:
		return tag, b.buildDockerfile(ctx, tag, cfg.BuildDir, cfg.Platform)
	case ModeNixpacks:
		return tag, b.buildNixpacks(ctx, tag, cfg.App, cfg.BuildDir)
	default:
		return "", fmt.Errorf("unknown build mode: %s", cfg.Mode)
	}
}

func (b *Builder) buildDockerfile(ctx context.Context, tag, buildDir, platform string) error {
	cmd := "docker build -t " + tag
	if platform != "" {
		cmd += " --platform " + platform
	}
	cmd += " " + buildDir
	return b.exec.RunStream(ctx, cmd, b.stdout, b.stdout)
}

func (b *Builder) buildNixpacks(ctx context.Context, tag, app, buildDir string) error {
	// Ensure Nixpacks is installed (lazy installation).
	if err := b.ensureNixpacks(ctx); err != nil {
		return err
	}

	cachePath := fmt.Sprintf("/deployments/%s/cache", app)
	cmd := fmt.Sprintf("nixpacks build %s --name %s --cache-path %s", buildDir, tag, cachePath)
	return b.exec.RunStream(ctx, cmd, b.stdout, b.stdout)
}

func (b *Builder) ensureNixpacks(ctx context.Context) error {
	if _, err := b.exec.Run(ctx, "which nixpacks"); err == nil {
		return nil
	}

	fmt.Fprintln(b.stdout, "Installing Nixpacks...")
	return b.exec.RunStream(ctx, "curl -sSL https://nixpacks.com/install.sh | bash", b.stdout, b.stdout)
}

// LocalBuildConfig holds parameters for building locally and streaming to server.
type LocalBuildConfig struct {
	App      string
	Version  string
	Mode     Mode
	Dir      string // local source directory
	Host     string
	User     string
	KeyPath  string
	Platform string       // e.g. "linux/arm64" (optional, overrides auto-detection)
	Exec     ssh.Executor // optional: if set, enables layer-optimized transfer
}

// LocalBuild builds the image on the local machine, then streams it to the
// server via `docker save | ssh docker load`. Returns the image tag.
func LocalBuild(ctx context.Context, cfg LocalBuildConfig, stdout io.Writer) (string, error) {
	tag := ImageTag(cfg.App, cfg.Version)

	// Build locally.
	switch cfg.Mode {
	case ModeDockerfile:
		if err := localBuildDockerfile(ctx, tag, cfg.Dir, cfg.Platform, stdout); err != nil {
			return "", err
		}
	case ModeNixpacks:
		if err := localBuildNixpacks(ctx, tag, cfg.Dir, stdout); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown build mode: %s", cfg.Mode)
	}

	// Stream image to server. Try layer-optimized (gzip) transfer first.
	fmt.Fprintln(stdout, "Streaming image to server...")
	if cfg.Exec != nil {
		if err := LayerOptimizedTransfer(ctx, tag, cfg.App, cfg.Host, cfg.User, cfg.KeyPath, cfg.Exec, stdout); err != nil {
			fmt.Fprintf(stdout, "  Layer-optimized transfer unavailable (%v), using plain transfer\n", err)
			if err := streamImage(ctx, tag, cfg.Host, cfg.User, cfg.KeyPath, stdout); err != nil {
				return "", fmt.Errorf("streaming image: %w", err)
			}
		}
	} else {
		if err := streamImage(ctx, tag, cfg.Host, cfg.User, cfg.KeyPath, stdout); err != nil {
			return "", fmt.Errorf("streaming image: %w", err)
		}
	}
	fmt.Fprintln(stdout, "  Image loaded on server")

	return tag, nil
}

func localBuildDockerfile(ctx context.Context, tag, dir, platform string, stdout io.Writer) error {
	args := []string{"build", "-t", tag}

	if platform != "" {
		// Explicit platform from config.
		args = append(args, "--platform", platform)
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		// Cross-compile for linux/amd64 when building on macOS ARM.
		args = append(args, "--platform", "linux/amd64")
	}

	args = append(args, dir)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local docker build failed: %w", err)
	}
	return nil
}

func localBuildNixpacks(ctx context.Context, tag, dir string, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, "nixpacks", "build", dir, "--name", tag)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local nixpacks build failed: %w", err)
	}
	return nil
}

func streamImage(ctx context.Context, tag, host, user, keyPath string, stdout io.Writer) error {
	sshArgs := []string{"-o", "StrictHostKeyChecking=no"}
	if keyPath != "" {
		sshArgs = append(sshArgs, "-i", keyPath)
	}
	sshTarget := fmt.Sprintf("%s@%s", user, host)
	sshArgs = append(sshArgs, sshTarget, "docker", "load")

	// docker save <tag> | ssh <host> docker load
	save := exec.CommandContext(ctx, "docker", "save", tag)
	load := exec.CommandContext(ctx, "ssh", sshArgs...)

	pipe, err := save.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	load.Stdin = pipe
	load.Stdout = stdout
	load.Stderr = stdout

	if err := save.Start(); err != nil {
		return fmt.Errorf("starting docker save: %w", err)
	}
	if err := load.Start(); err != nil {
		save.Process.Kill()
		return fmt.Errorf("starting ssh load: %w", err)
	}

	saveErr := save.Wait()
	loadErr := load.Wait()
	if saveErr != nil {
		return fmt.Errorf("docker save: %w", saveErr)
	}
	if loadErr != nil {
		return fmt.Errorf("ssh docker load: %w", loadErr)
	}
	return nil
}

// PruneImages removes build images older than 72 hours for the given app.
func (b *Builder) PruneImages(ctx context.Context, app string) error {
	// Remove images matching the app build tag that are older than 72h.
	cmd := fmt.Sprintf(
		"docker image ls --filter reference='%s-build-*' --format '{{.ID}} {{.CreatedAt}}' | "+
			"awk -v cutoff=\"$(date -d '72 hours ago' +%%s 2>/dev/null || date -v-72H +%%s)\" "+
			"'{ cmd=\"date -d \\\"\"$2\" \"$3\"\\\" +%%s 2>/dev/null || date -j -f \\\"%%Y-%%m-%%d %%H:%%M:%%S\\\" \\\"\"$2\" \"$3\"\\\" +%%s\"; "+
			"cmd | getline ts; close(cmd); if (ts < cutoff) print $1 }' | "+
			"xargs -r docker rmi 2>/dev/null || true",
		app,
	)
	_, err := b.exec.Run(ctx, cmd)
	return err
}
