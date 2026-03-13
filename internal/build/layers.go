package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"

	"github.com/teploy/teploy/internal/ssh"
)

// getLocalLayers returns the layer diff IDs for a local Docker image.
func getLocalLayers(ctx context.Context, tag string) ([]string, error) {
	out, err := osexec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .RootFS.Layers}}", tag).Output()
	if err != nil {
		return nil, fmt.Errorf("inspecting local image: %w", err)
	}
	var layers []string
	if err := json.Unmarshal(out, &layers); err != nil {
		return nil, fmt.Errorf("parsing layers: %w", err)
	}
	return layers, nil
}

// getRemoteLayers returns layer diff IDs for an image on the server.
func getRemoteLayers(ctx context.Context, exec ssh.Executor, tag string) ([]string, error) {
	out, err := exec.Run(ctx, fmt.Sprintf("docker inspect --format '{{json .RootFS.Layers}}' %s", tag))
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	var layers []string
	if err := json.Unmarshal([]byte(out), &layers); err != nil {
		return nil, fmt.Errorf("parsing remote layers: %w", err)
	}
	return layers, nil
}

// findPreviousImage finds the most recent app build image on the server.
func findPreviousImage(ctx context.Context, exec ssh.Executor, app string) string {
	out, _ := exec.Run(ctx, fmt.Sprintf(
		"docker images --filter reference='%s-build-*' --format '{{.Repository}}:{{.Tag}}' | head -1",
		app,
	))
	return strings.TrimSpace(out)
}

// LayerOptimizedTransfer compresses the image with gzip before streaming.
// Also checks if the exact image already exists on the server (skip transfer).
// Reports layer statistics for user feedback.
// Returns an error if optimization isn't applicable (caller falls back to full transfer).
func LayerOptimizedTransfer(ctx context.Context, tag, app, host, user, keyPath string, exec ssh.Executor, stdout io.Writer) error {
	// 1. Check if exact image already exists on server.
	if _, err := exec.Run(ctx, fmt.Sprintf("docker inspect %s >/dev/null 2>&1", tag)); err == nil {
		fmt.Fprintln(stdout, "  Image already exists on server — skipping transfer")
		return nil
	}

	// 2. Find previous image and report layer stats.
	prevTag := findPreviousImage(ctx, exec, app)
	if prevTag != "" && prevTag != tag+":latest" {
		localLayers, localErr := getLocalLayers(ctx, tag)
		remoteLayers, remoteErr := getRemoteLayers(ctx, exec, prevTag)

		if localErr == nil && remoteErr == nil {
			shared := countSharedLayers(localLayers, remoteLayers)
			newCount := len(localLayers) - shared
			if shared > 0 {
				fmt.Fprintf(stdout, "  Layer stats: %d/%d layers shared with previous build, %d new\n",
					shared, len(localLayers), newCount)
			}
		}
	}

	// 3. Use gzip-compressed transfer: docker save | gzip | ssh "gunzip | docker load"
	fmt.Fprintln(stdout, "  Streaming compressed image to server...")

	sshArgs := []string{"-o", "StrictHostKeyChecking=no"}
	if keyPath != "" {
		sshArgs = append(sshArgs, "-i", keyPath)
	}
	sshTarget := fmt.Sprintf("%s@%s", user, host)
	sshArgs = append(sshArgs, sshTarget, "gunzip | docker load")

	save := osexec.CommandContext(ctx, "docker", "save", tag)
	gzip := osexec.CommandContext(ctx, "gzip", "-1") // fast compression
	load := osexec.CommandContext(ctx, "ssh", sshArgs...)

	// Pipeline: docker save | gzip | ssh "gunzip | docker load"
	savePipe, err := save.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating save pipe: %w", err)
	}
	gzip.Stdin = savePipe

	gzipPipe, err := gzip.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating gzip pipe: %w", err)
	}
	load.Stdin = gzipPipe
	load.Stdout = stdout
	load.Stderr = stdout

	// Check if gzip is available locally.
	if _, err := osexec.LookPath("gzip"); err != nil {
		return fmt.Errorf("gzip not found locally")
	}

	if err := save.Start(); err != nil {
		return fmt.Errorf("starting docker save: %w", err)
	}
	if err := gzip.Start(); err != nil {
		save.Process.Kill()
		return fmt.Errorf("starting gzip: %w", err)
	}
	if err := load.Start(); err != nil {
		save.Process.Kill()
		gzip.Process.Kill()
		return fmt.Errorf("starting ssh load: %w", err)
	}

	saveErr := save.Wait()
	gzipErr := gzip.Wait()
	loadErr := load.Wait()

	if saveErr != nil {
		return fmt.Errorf("docker save: %w", saveErr)
	}
	if gzipErr != nil {
		return fmt.Errorf("gzip: %w", gzipErr)
	}
	if loadErr != nil {
		return fmt.Errorf("ssh docker load: %w", loadErr)
	}

	return nil
}

// countSharedLayers counts the number of matching layers from the start.
func countSharedLayers(a, b []string) int {
	shared := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			shared++
		} else {
			break
		}
	}
	return shared
}

// humanSize formats bytes into human-readable format.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// TempFileSize returns the size of a docker save output for the given tag.
func TempFileSize(ctx context.Context, tag string) (int64, error) {
	f, err := os.CreateTemp("", "teploy-size-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	cmd := osexec.CommandContext(ctx, "docker", "save", "-o", f.Name(), tag)
	if err := cmd.Run(); err != nil {
		return 0, err
	}

	info, err := os.Stat(f.Name())
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
