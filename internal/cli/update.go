package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubRepo   = "teploy/teploy"
	releaseAPI   = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
	downloadBase = "https://github.com/" + githubRepo + "/releases/download"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func newUpdateCmd(currentVersion string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update teploy to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(currentVersion, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "force update even if already on latest")

	return cmd
}

func runUpdate(currentVersion string, force bool) error {
	fmt.Printf("Current version: %s\n", currentVersion)

	// Fetch latest release info.
	fmt.Println("Checking for updates...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	latest, err := fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	latestVersion := strings.TrimPrefix(latest.TagName, "v")
	fmt.Printf("Latest version: %s\n", latestVersion)

	if !force && latestVersion == currentVersion {
		fmt.Println("Already up to date")
		return nil
	}

	// Determine binary name for this platform.
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	binaryName := fmt.Sprintf("teploy-%s-%s", goos, goarch)
	if goos == "windows" {
		binaryName += ".exe"
	}

	downloadURL := fmt.Sprintf("%s/%s/%s", downloadBase, latest.TagName, binaryName)
	fmt.Printf("Downloading %s...\n", downloadURL)

	// Download to temp file.
	tmpFile, err := os.CreateTemp("", "teploy-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := downloadFile(ctx, downloadURL, tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("downloading update: %w", err)
	}
	tmpFile.Close()

	// Make executable.
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	// Verify the downloaded binary works.
	out, err := exec.Command(tmpPath, "version").Output()
	if err != nil {
		return fmt.Errorf("downloaded binary is invalid: %w", err)
	}
	fmt.Printf("  Verified: %s", out)

	// Find current binary path.
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding current binary: %w", err)
	}

	// Replace current binary.
	fmt.Printf("Replacing %s...\n", currentBinary)
	if err := replaceBinary(tmpPath, currentBinary); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	fmt.Printf("Updated to %s\n", latestVersion)
	return nil
}

func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "teploy-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no releases found — update manually from GitHub")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release info: %w", err)
	}
	return &release, nil
}

func downloadFile(ctx context.Context, url string, dest *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "teploy-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned status %d — binary may not exist for %s/%s", resp.StatusCode, runtime.GOOS, runtime.GOARCH)
	}

	_, err = io.Copy(dest, resp.Body)
	return err
}

func replaceBinary(src, dst string) error {
	// Read the new binary.
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Write to destination (overwrite).
	if err := os.WriteFile(dst, data, 0755); err != nil {
		// If permission denied, suggest sudo.
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied — try: sudo cp %s %s", src, dst)
		}
		return err
	}
	return nil
}
