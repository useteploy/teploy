package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupHandlersTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".teploy"), 0755)
	return tmpDir, func() {
		os.Setenv("HOME", origHome)
	}
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(data)
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var resp apiResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp apiResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != "bad input" {
		t.Errorf("expected error 'bad input', got %q", resp.Error)
	}
}

func TestConfigServersHandlers(t *testing.T) {
	_, cleanup := setupHandlersTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// List servers (empty initially)
	req := httptest.NewRequest("GET", "/api/config/servers", nil)
	w := httptest.NewRecorder()
	srv.handleConfigListServers(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Add server — invalid name
	req = httptest.NewRequest("POST", "/api/config/servers",
		jsonBody(t, map[string]string{"name": "; bad", "host": "1.2.3.4"}))
	w = httptest.NewRecorder()
	srv.handleConfigAddServer(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid name: expected 400, got %d", w.Code)
	}

	// Add server — valid
	req = httptest.NewRequest("POST", "/api/config/servers",
		jsonBody(t, map[string]string{"name": "prod", "host": "1.2.3.4", "user": "deploy", "role": "app"}))
	w = httptest.NewRecorder()
	srv.handleConfigAddServer(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Edit server
	req = httptest.NewRequest("PUT", "/api/config/servers/prod",
		jsonBody(t, map[string]string{"host": "5.6.7.8", "user": "admin", "role": "lb"}))
	req.SetPathValue("name", "prod")
	w = httptest.NewRecorder()
	srv.handleConfigEditServer(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("edit: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete server
	req = httptest.NewRequest("DELETE", "/api/config/servers/prod", nil)
	req.SetPathValue("name", "prod")
	w = httptest.NewRecorder()
	srv.handleConfigDeleteServer(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationsHandlers(t *testing.T) {
	_, cleanup := setupHandlersTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Get (empty)
	req := httptest.NewRequest("GET", "/api/config/notifications", nil)
	w := httptest.NewRecorder()
	srv.handleGetNotifications(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Set
	req = httptest.NewRequest("POST", "/api/config/notifications",
		jsonBody(t, notificationConfig{WebhookURL: "https://example.com/hook"}))
	w = httptest.NewRecorder()
	srv.handleSetNotifications(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get again (should have the URL)
	req = httptest.NewRequest("GET", "/api/config/notifications", nil)
	w = httptest.NewRecorder()
	srv.handleGetNotifications(w, req)
	var resp apiResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := json.Marshal(resp.Data)
	var cfg notificationConfig
	json.Unmarshal(data, &cfg)
	if cfg.WebhookURL != "https://example.com/hook" {
		t.Errorf("expected webhook URL, got %q", cfg.WebhookURL)
	}
}

func TestRegistryHandlers(t *testing.T) {
	_, cleanup := setupHandlersTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Add
	req := httptest.NewRequest("POST", "/api/config/registries",
		jsonBody(t, registryEntry{Server: "ghcr.io", Username: "user"}))
	w := httptest.NewRecorder()
	srv.handleAddRegistry(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List
	req = httptest.NewRequest("GET", "/api/config/registries", nil)
	w = httptest.NewRecorder()
	srv.handleListRegistries(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	// Delete
	req = httptest.NewRequest("DELETE", "/api/config/registries/ghcr.io", nil)
	req.SetPathValue("server", "ghcr.io")
	w = httptest.NewRecorder()
	srv.handleDeleteRegistry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List after delete (should be empty)
	req = httptest.NewRequest("GET", "/api/config/registries", nil)
	w = httptest.NewRecorder()
	srv.handleListRegistries(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list after delete: expected 200, got %d", w.Code)
	}
}

func TestParseIntSafe(t *testing.T) {
	if n := parseIntSafe("123"); n != 123 {
		t.Errorf("expected 123, got %d", n)
	}
	if n := parseIntSafe("abc"); n != 0 {
		t.Errorf("expected 0 for non-numeric, got %d", n)
	}
	if n := parseIntSafe(""); n != 0 {
		t.Errorf("expected 0 for empty, got %d", n)
	}
}

func TestConfigServerValidation(t *testing.T) {
	_, cleanup := setupHandlersTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Missing host
	req := httptest.NewRequest("POST", "/api/config/servers",
		jsonBody(t, map[string]string{"name": "prod"}))
	w := httptest.NewRecorder()
	srv.handleConfigAddServer(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing host: expected 400, got %d", w.Code)
	}

	// Edit with invalid name
	req = httptest.NewRequest("PUT", "/api/config/servers/;bad",
		jsonBody(t, map[string]string{"host": "1.2.3.4"}))
	req.SetPathValue("name", ";bad")
	w = httptest.NewRecorder()
	srv.handleConfigEditServer(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid edit name: expected 400, got %d", w.Code)
	}

	// Delete with invalid name
	req = httptest.NewRequest("DELETE", "/api/config/servers/;bad", nil)
	req.SetPathValue("name", ";bad")
	w = httptest.NewRecorder()
	srv.handleConfigDeleteServer(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid delete name: expected 400, got %d", w.Code)
	}
}

func TestRegistryValidation(t *testing.T) {
	_, cleanup := setupHandlersTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Missing server
	req := httptest.NewRequest("POST", "/api/config/registries",
		jsonBody(t, registryEntry{Username: "user"}))
	w := httptest.NewRecorder()
	srv.handleAddRegistry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing server: expected 400, got %d", w.Code)
	}

	// Missing username
	req = httptest.NewRequest("POST", "/api/config/registries",
		jsonBody(t, registryEntry{Server: "ghcr.io"}))
	w = httptest.NewRecorder()
	srv.handleAddRegistry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing username: expected 400, got %d", w.Code)
	}

	// Delete with empty server
	req = httptest.NewRequest("DELETE", "/api/config/registries/", nil)
	req.SetPathValue("server", "")
	w = httptest.NewRecorder()
	srv.handleDeleteRegistry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty server: expected 400, got %d", w.Code)
	}
}

func TestTeployConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	path := teployConfigPath("test.json")
	expected := filepath.Join(tmpDir, ".teploy", "test.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}
