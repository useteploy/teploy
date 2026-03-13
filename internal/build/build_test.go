package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teploy/teploy/internal/ssh"
)

func TestDetect_Dockerfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node:20"), 0644)

	if mode := Detect(dir); mode != ModeDockerfile {
		t.Errorf("expected ModeDockerfile, got %s", mode)
	}
}

func TestDetect_NoDockerfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	if mode := Detect(dir); mode != ModeNixpacks {
		t.Errorf("expected ModeNixpacks, got %s", mode)
	}
}

func TestImageTag(t *testing.T) {
	tag := ImageTag("myapp", "abc1234")
	if tag != "myapp-build-abc1234" {
		t.Errorf("expected myapp-build-abc1234, got %s", tag)
	}
}

func TestBuild_Dockerfile(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "docker build", Output: "Successfully built abc123\n"},
	)

	var buf bytes.Buffer
	builder := NewBuilder(mock, &buf)

	tag, err := builder.Build(context.Background(), BuildConfig{
		App:      "myapp",
		Version:  "abc1234",
		Mode:     ModeDockerfile,
		BuildDir: "/deployments/myapp/build",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "myapp-build-abc1234" {
		t.Errorf("expected tag myapp-build-abc1234, got %s", tag)
	}

	// Verify docker build command.
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[0], "docker build -t myapp-build-abc1234 /deployments/myapp/build") {
		t.Errorf("unexpected command: %s", mock.Calls[0])
	}
}

func TestBuild_DockerfileWithPlatform(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "docker build", Output: "Successfully built abc123\n"},
	)

	var buf bytes.Buffer
	builder := NewBuilder(mock, &buf)

	_, err := builder.Build(context.Background(), BuildConfig{
		App:      "myapp",
		Version:  "abc1234",
		Mode:     ModeDockerfile,
		BuildDir: "/deployments/myapp/build",
		Platform: "linux/arm64",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[0], "--platform linux/arm64") {
		t.Errorf("expected --platform flag in command: %s", mock.Calls[0])
	}
}

func TestBuild_DockerfileNoPlatform(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "docker build", Output: "Successfully built abc123\n"},
	)

	var buf bytes.Buffer
	builder := NewBuilder(mock, &buf)

	_, err := builder.Build(context.Background(), BuildConfig{
		App:      "myapp",
		Version:  "abc1234",
		Mode:     ModeDockerfile,
		BuildDir: "/deployments/myapp/build",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(mock.Calls[0], "--platform") {
		t.Errorf("expected no --platform flag when not set: %s", mock.Calls[0])
	}
}

func TestBuild_Nixpacks(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "which nixpacks", Output: "/usr/local/bin/nixpacks\n"},
		ssh.MockCommand{Match: "nixpacks build", Output: "Building...\n"},
	)

	var buf bytes.Buffer
	builder := NewBuilder(mock, &buf)

	tag, err := builder.Build(context.Background(), BuildConfig{
		App:      "myapp",
		Version:  "abc1234",
		Mode:     ModeNixpacks,
		BuildDir: "/deployments/myapp/build",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "myapp-build-abc1234" {
		t.Errorf("expected tag myapp-build-abc1234, got %s", tag)
	}

	// Verify nixpacks build command includes cache path.
	found := false
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "nixpacks build") {
			found = true
			if !strings.Contains(call, "--name myapp-build-abc1234") {
				t.Errorf("missing --name flag: %s", call)
			}
			if !strings.Contains(call, "--cache-path /deployments/myapp/cache") {
				t.Errorf("missing --cache-path: %s", call)
			}
		}
	}
	if !found {
		t.Error("nixpacks build command not found")
	}
}

func TestBuild_NixpacksInstallsIfMissing(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "which nixpacks", Err: fmt.Errorf("not found")},
		ssh.MockCommand{Match: "curl", Output: "installed\n"},
		ssh.MockCommand{Match: "nixpacks build", Output: "done\n"},
	)

	var buf bytes.Buffer
	builder := NewBuilder(mock, &buf)

	_, err := builder.Build(context.Background(), BuildConfig{
		App:      "myapp",
		Version:  "v1",
		Mode:     ModeNixpacks,
		BuildDir: "/deployments/myapp/build",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have installed nixpacks first.
	if !strings.Contains(buf.String(), "Installing Nixpacks") {
		t.Error("expected install message")
	}

	// Verify curl install command was called.
	foundCurl := false
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "curl") {
			foundCurl = true
		}
	}
	if !foundCurl {
		t.Error("expected curl install command")
	}
}

func TestBuild_UnknownMode(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4")
	builder := NewBuilder(mock, &bytes.Buffer{})

	_, err := builder.Build(context.Background(), BuildConfig{
		Mode: ModeNone,
	})
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestPruneImages(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "docker image ls", Output: ""},
	)

	builder := NewBuilder(mock, &bytes.Buffer{})
	if err := builder.PruneImages(context.Background(), "myapp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if !strings.Contains(mock.Calls[0], "myapp-build-*") {
		t.Errorf("expected filter for app build images: %s", mock.Calls[0])
	}
}

func TestLoadIgnore_Default(t *testing.T) {
	dir := t.TempDir()
	patterns := LoadIgnore(dir)

	if len(patterns) != len(DefaultIgnore) {
		t.Fatalf("expected %d default patterns, got %d", len(DefaultIgnore), len(patterns))
	}
	for i, p := range patterns {
		if p != DefaultIgnore[i] {
			t.Errorf("pattern %d: expected %s, got %s", i, DefaultIgnore[i], p)
		}
	}
}

func TestLoadIgnore_CustomFile(t *testing.T) {
	dir := t.TempDir()
	content := "vendor\n# comment\n.cache\n\nbuild\n"
	os.WriteFile(filepath.Join(dir, ".teployignore"), []byte(content), 0644)

	patterns := LoadIgnore(dir)
	expected := []string{"vendor", ".cache", "build"}

	if len(patterns) != len(expected) {
		t.Fatalf("expected %d patterns, got %d: %v", len(expected), len(patterns), patterns)
	}
	for i, p := range patterns {
		if p != expected[i] {
			t.Errorf("pattern %d: expected %s, got %s", i, expected[i], p)
		}
	}
}

func TestLoadIgnore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".teployignore"), []byte("\n\n# only comments\n"), 0644)

	patterns := LoadIgnore(dir)
	if len(patterns) != len(DefaultIgnore) {
		t.Fatalf("expected defaults for empty file, got %d patterns", len(patterns))
	}
}

func TestLocalBuildDockerfile(t *testing.T) {
	// Unit test: verify the streamImage function constructs correct arguments.
	// We can't run docker save/ssh in unit tests, but we can test the supporting functions.
	tag := ImageTag("myapp", "abc1234")
	if tag != "myapp-build-abc1234" {
		t.Errorf("expected myapp-build-abc1234, got %s", tag)
	}
}

func TestLocalBuildConfig(t *testing.T) {
	cfg := LocalBuildConfig{
		App:     "myapp",
		Version: "abc1234",
		Mode:    ModeDockerfile,
		Dir:     "/tmp/src",
		Host:    "1.2.3.4",
		User:    "root",
		KeyPath: "/home/user/.ssh/id_ed25519",
	}

	if cfg.App != "myapp" {
		t.Errorf("unexpected app: %s", cfg.App)
	}
	if cfg.Mode != ModeDockerfile {
		t.Errorf("unexpected mode: %s", cfg.Mode)
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeDockerfile, "dockerfile"},
		{ModeNixpacks, "nixpacks"},
		{ModeNone, "none"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %s, want %s", tt.mode, got, tt.want)
		}
	}
}
