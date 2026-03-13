package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCompose_BasicMapping(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  web:
    build: .
    ports: ["3000:3000"]
    depends_on: [db, redis]
  worker:
    build: .
    command: npm run worker
  db:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data
    environment:
      POSTGRES_PASSWORD: pass
  redis:
    image: redis:7
volumes:
  pgdata:
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644)

	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("LoadCompose: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Web + worker processes.
	if cfg.Processes["web"] != "" {
		t.Errorf("web process should have empty command, got %q", cfg.Processes["web"])
	}
	if cfg.Processes["worker"] != "npm run worker" {
		t.Errorf("worker process command = %q, want 'npm run worker'", cfg.Processes["worker"])
	}

	// Accessories.
	pg, ok := cfg.Accessories["db"]
	if !ok {
		t.Fatal("expected postgres accessory 'db'")
	}
	if pg.Image != "postgres:16" {
		t.Errorf("postgres image = %s, want postgres:16", pg.Image)
	}
	if pg.Port != 5432 {
		t.Errorf("postgres port = %d, want 5432", pg.Port)
	}
	if pg.Env["POSTGRES_PASSWORD"] != "pass" {
		t.Errorf("postgres env = %v", pg.Env)
	}

	redis, ok := cfg.Accessories["redis"]
	if !ok {
		t.Fatal("expected redis accessory")
	}
	if redis.Image != "redis:7" {
		t.Errorf("redis image = %s, want redis:7", redis.Image)
	}
	if redis.Port != 6379 {
		t.Errorf("redis port = %d, want 6379", redis.Port)
	}

	// No pre-built image (uses build context).
	if cfg.Image != "" {
		t.Errorf("expected no image for build-based service, got %s", cfg.Image)
	}
}

func TestLoadCompose_NoComposeFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}
}

func TestLoadCompose_ComposeYml(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  app:
    image: myapp:latest
    ports: ["8080:8080"]
`
	os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0644)

	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("LoadCompose: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config from compose.yml")
	}
	if cfg.Image != "myapp:latest" {
		t.Errorf("expected image myapp:latest, got %s", cfg.Image)
	}
}

func TestLoadCompose_NoPorts(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  worker:
    build: .
    command: npm run worker
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644)

	_, err := LoadCompose(dir)
	if err == nil {
		t.Fatal("expected error when no service has ports")
	}
}

func TestLoadCompose_ImageService(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  web:
    image: ghcr.io/myorg/myapp:v1
    ports: ["3000:3000"]
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644)

	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("LoadCompose: %v", err)
	}
	if cfg.Image != "ghcr.io/myorg/myapp:v1" {
		t.Errorf("expected image from compose, got %s", cfg.Image)
	}
}

func TestLoadCompose_BuildContextStruct(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  web:
    build:
      context: .
      dockerfile: Dockerfile.prod
    ports: ["3000:3000"]
  worker:
    build:
      context: .
    command: node worker.js
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644)

	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("LoadCompose: %v", err)
	}
	if cfg.Processes["worker"] != "node worker.js" {
		t.Errorf("expected worker process, got %q", cfg.Processes["worker"])
	}
}

func TestLoadCompose_EnvironmentList(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  web:
    build: .
    ports: ["3000:3000"]
  db:
    image: postgres:16
    environment:
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=mydb
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644)

	cfg, err := LoadCompose(dir)
	if err != nil {
		t.Fatalf("LoadCompose: %v", err)
	}
	pg := cfg.Accessories["db"]
	if pg.Env["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("expected POSTGRES_PASSWORD=secret, got %v", pg.Env)
	}
	if pg.Env["POSTGRES_DB"] != "mydb" {
		t.Errorf("expected POSTGRES_DB=mydb, got %v", pg.Env)
	}
}

func TestLoadCompose_TeployYmlWins(t *testing.T) {
	dir := t.TempDir()

	// Write both files.
	os.WriteFile(filepath.Join(dir, "teploy.yml"), []byte("app: myapp\ndomain: myapp.com\n"), 0644)
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: other\n    ports: ['3000:3000']\n"), 0644)

	cfg, err := LoadApp(dir)
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	// teploy.yml should win.
	if cfg.App != "myapp" {
		t.Errorf("expected app from teploy.yml, got %s", cfg.App)
	}
}

func TestIsAccessoryImage(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"postgres:16", true},
		{"redis:7", true},
		{"mysql:8", true},
		{"mongo:latest", true},
		{"myapp:latest", false},
		{"ghcr.io/myorg/myapp:v1", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isAccessoryImage(tt.image); got != tt.want {
			t.Errorf("isAccessoryImage(%q) = %v, want %v", tt.image, got, tt.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	// String command.
	if got := parseCommand("npm run worker"); got != "npm run worker" {
		t.Errorf("string command: got %q", got)
	}

	// List command.
	list := []interface{}{"npm", "run", "worker"}
	if got := parseCommand(list); got != "npm run worker" {
		t.Errorf("list command: got %q", got)
	}

	// Nil command.
	if got := parseCommand(nil); got != "" {
		t.Errorf("nil command: got %q", got)
	}
}
