package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupGroupsTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".teploy"), 0755)
	return tmpDir, func() {
		os.Setenv("HOME", origHome)
	}
}

func TestGroupsCRUD(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Create group
	body := `{"name":"Production"}`
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List groups
	req = httptest.NewRequest("GET", "/api/groups", nil)
	w = httptest.NewRecorder()
	srv.handleListGroups(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list groups: expected 200, got %d", w.Code)
	}
	var listResp apiResponse
	json.Unmarshal(w.Body.Bytes(), &listResp)

	// Duplicate group should fail
	req = httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate group: expected 409, got %d", w.Code)
	}

	// Delete group
	req = httptest.NewRequest("DELETE", "/api/groups/Production", nil)
	req.SetPathValue("name", "Production")
	w = httptest.NewRecorder()
	srv.handleDeleteGroup(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete group: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGroupAssignUnassign(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Create group first
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(`{"name":"Staging"}`))
	w := httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}

	// Assign app
	req = httptest.NewRequest("POST", "/api/groups/Staging/apps", bytes.NewBufferString(`{"app":"myapp"}`))
	req.SetPathValue("name", "Staging")
	w = httptest.NewRecorder()
	srv.handleAssignApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("assign: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Duplicate assign should fail
	req = httptest.NewRequest("POST", "/api/groups/Staging/apps", bytes.NewBufferString(`{"app":"myapp"}`))
	req.SetPathValue("name", "Staging")
	w = httptest.NewRecorder()
	srv.handleAssignApp(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("dup assign: expected 409, got %d", w.Code)
	}

	// Unassign app
	req = httptest.NewRequest("DELETE", "/api/groups/Staging/apps/myapp", nil)
	req.SetPathValue("name", "Staging")
	req.SetPathValue("app", "myapp")
	w = httptest.NewRecorder()
	srv.handleUnassignApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unassign: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProjectCRUD(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Create group first
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(`{"name":"Production"}`))
	w := httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d", w.Code)
	}

	// Create project
	req = httptest.NewRequest("POST", "/api/groups/Production/projects", bytes.NewBufferString(`{"name":"E-Commerce"}`))
	req.SetPathValue("name", "Production")
	w = httptest.NewRecorder()
	srv.handleCreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Duplicate project should fail
	req = httptest.NewRequest("POST", "/api/groups/Production/projects", bytes.NewBufferString(`{"name":"E-Commerce"}`))
	req.SetPathValue("name", "Production")
	w = httptest.NewRecorder()
	srv.handleCreateProject(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("dup project: expected 409, got %d", w.Code)
	}

	// Rename project
	req = httptest.NewRequest("PUT", "/api/groups/Production/projects/E-Commerce", bytes.NewBufferString(`{"name":"Store"}`))
	req.SetPathValue("name", "Production")
	req.SetPathValue("project", "E-Commerce")
	w = httptest.NewRecorder()
	srv.handleRenameProject(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rename project: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify rename persisted
	req = httptest.NewRequest("GET", "/api/groups", nil)
	w = httptest.NewRecorder()
	srv.handleListGroups(w, req)
	var listResp struct {
		Data []group `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 || len(listResp.Data[0].Projects) != 1 || listResp.Data[0].Projects[0].Name != "Store" {
		t.Fatalf("rename not persisted: %+v", listResp.Data)
	}

	// Delete project
	req = httptest.NewRequest("DELETE", "/api/groups/Production/projects/Store", nil)
	req.SetPathValue("name", "Production")
	req.SetPathValue("project", "Store")
	w = httptest.NewRecorder()
	srv.handleDeleteProject(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete project: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete non-existent project should 404
	req = httptest.NewRequest("DELETE", "/api/groups/Production/projects/Store", nil)
	req.SetPathValue("name", "Production")
	req.SetPathValue("project", "Store")
	w = httptest.NewRecorder()
	srv.handleDeleteProject(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing project: expected 404, got %d", w.Code)
	}
}

func TestProjectAppAssignment(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Setup: group + project
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(`{"name":"Staging"}`))
	w := httptest.NewRecorder()
	srv.handleCreateGroup(w, req)

	req = httptest.NewRequest("POST", "/api/groups/Staging/projects", bytes.NewBufferString(`{"name":"Backend"}`))
	req.SetPathValue("name", "Staging")
	w = httptest.NewRecorder()
	srv.handleCreateProject(w, req)

	// Assign app to project
	req = httptest.NewRequest("POST", "/api/groups/Staging/projects/Backend/apps", bytes.NewBufferString(`{"app":"api-server"}`))
	req.SetPathValue("name", "Staging")
	req.SetPathValue("project", "Backend")
	w = httptest.NewRecorder()
	srv.handleProjectAssignApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("assign app: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Duplicate assign should fail
	req = httptest.NewRequest("POST", "/api/groups/Staging/projects/Backend/apps", bytes.NewBufferString(`{"app":"api-server"}`))
	req.SetPathValue("name", "Staging")
	req.SetPathValue("project", "Backend")
	w = httptest.NewRecorder()
	srv.handleProjectAssignApp(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("dup assign: expected 409, got %d", w.Code)
	}

	// Unassign app from project
	req = httptest.NewRequest("DELETE", "/api/groups/Staging/projects/Backend/apps/api-server", nil)
	req.SetPathValue("name", "Staging")
	req.SetPathValue("project", "Backend")
	req.SetPathValue("app", "api-server")
	w = httptest.NewRecorder()
	srv.handleProjectUnassignApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unassign: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Unassign non-existent app should 404
	req = httptest.NewRequest("DELETE", "/api/groups/Staging/projects/Backend/apps/api-server", nil)
	req.SetPathValue("name", "Staging")
	req.SetPathValue("project", "Backend")
	req.SetPathValue("app", "api-server")
	w = httptest.NewRecorder()
	srv.handleProjectUnassignApp(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unassign missing: expected 404, got %d", w.Code)
	}
}

func TestProjectInNonExistentGroup(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Create project in non-existent group
	req := httptest.NewRequest("POST", "/api/groups/NoSuchGroup/projects", bytes.NewBufferString(`{"name":"Test"}`))
	req.SetPathValue("name", "NoSuchGroup")
	w := httptest.NewRecorder()
	srv.handleCreateProject(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGroupValidation(t *testing.T) {
	_, cleanup := setupGroupsTest(t)
	defer cleanup()

	srv := NewServer("127.0.0.1:0")

	// Invalid group name
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(`{"name":"; rm -rf /"}`))
	w := httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("injection: expected 400, got %d", w.Code)
	}

	// Empty name
	req = httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(`{"name":""}`))
	w = httptest.NewRecorder()
	srv.handleCreateGroup(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty: expected 400, got %d", w.Code)
	}
}
