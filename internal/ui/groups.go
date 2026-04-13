package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// groupsData is the top-level structure of ~/.teploy/groups.json.
type groupsData struct {
	Groups []group `json:"groups"`
}

type project struct {
	Name string   `json:"name"`
	Apps []string `json:"apps"`
}

type group struct {
	Name     string    `json:"name"`
	Apps     []string  `json:"apps"`
	Projects []project `json:"projects,omitempty"`
}

func groupsFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".teploy", "groups.json")
}

func loadGroups() (groupsData, error) {
	raw, err := os.ReadFile(groupsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return groupsData{Groups: []group{}}, nil
		}
		return groupsData{}, err
	}

	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return groupsData{Groups: []group{}}, nil
	}

	// Handle both formats: {"groups": [...]} or bare [...]
	if raw[0] == '[' {
		var groups []group
		if err := json.Unmarshal(raw, &groups); err != nil {
			return groupsData{}, err
		}
		return groupsData{Groups: groups}, nil
	}

	var data groupsData
	if err := json.Unmarshal(raw, &data); err != nil {
		return groupsData{}, err
	}
	if data.Groups == nil {
		data.Groups = []group{}
	}
	return data, nil
}

func saveGroups(data groupsData) error {
	path := groupsFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sort.Slice(data.Groups, func(i, j int) bool {
		return data.Groups[i].Name < data.Groups[j].Name
	})
	writeJSON(w, http.StatusOK, data.Groups)
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateGroupName(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, g := range data.Groups {
		if g.Name == body.Name {
			writeError(w, http.StatusConflict, fmt.Sprintf("group %q already exists", body.Name))
			return
		}
	}

	data.Groups = append(data.Groups, group{Name: body.Name, Apps: []string{}})
	if err := saveGroups(data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	if err := validateGroupName(oldName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateGroupName(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	found := false
	for i, g := range data.Groups {
		if g.Name == oldName {
			data.Groups[i].Name = body.Name
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", oldName))
		return
	}

	if err := saveGroups(data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := validateGroupName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filtered := make([]group, 0, len(data.Groups))
	found := false
	for _, g := range data.Groups {
		if g.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, g)
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", name))
		return
	}

	data.Groups = filtered
	if err := saveGroups(data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAssignApp(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateAppName(body.App); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	found := false
	for i, g := range data.Groups {
		if g.Name == groupName {
			for _, a := range g.Apps {
				if a == body.App {
					writeError(w, http.StatusConflict, fmt.Sprintf("app %q already in group %q", body.App, groupName))
					return
				}
			}
			data.Groups[i].Apps = append(data.Groups[i].Apps, body.App)
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
		return
	}

	if err := saveGroups(data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

func (s *Server) handleUnassignApp(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	appName := r.PathValue("app")
	if err := validateAppName(appName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	found := false
	for i, g := range data.Groups {
		if g.Name == groupName {
			apps := make([]string, 0, len(g.Apps))
			for _, a := range g.Apps {
				if a != appName {
					apps = append(apps, a)
				} else {
					found = true
				}
			}
			data.Groups[i].Apps = apps
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found in group %q", appName, groupName))
		return
	}

	if err := saveGroups(data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateProjectName(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, g := range data.Groups {
		if g.Name == groupName {
			for _, p := range g.Projects {
				if p.Name == body.Name {
					writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists in group %q", body.Name, groupName))
					return
				}
			}
			data.Groups[i].Projects = append(data.Groups[i].Projects, project{Name: body.Name, Apps: []string{}})
			if err := saveGroups(data); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
}

func (s *Server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectName := r.PathValue("project")
	if err := validateProjectName(projectName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateProjectName(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, g := range data.Groups {
		if g.Name == groupName {
			for j, p := range g.Projects {
				if p.Name == projectName {
					data.Groups[i].Projects[j].Name = body.Name
					if err := saveGroups(data); err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
					writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
					return
				}
			}
			writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found in group %q", projectName, groupName))
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectName := r.PathValue("project")
	if err := validateProjectName(projectName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, g := range data.Groups {
		if g.Name == groupName {
			filtered := make([]project, 0, len(g.Projects))
			found := false
			for _, p := range g.Projects {
				if p.Name == projectName {
					found = true
					continue
				}
				filtered = append(filtered, p)
			}
			if !found {
				writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found in group %q", projectName, groupName))
				return
			}
			data.Groups[i].Projects = filtered
			if err := saveGroups(data); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
}

func (s *Server) handleProjectAssignApp(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectName := r.PathValue("project")
	if err := validateProjectName(projectName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateAppName(body.App); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, g := range data.Groups {
		if g.Name == groupName {
			for j, p := range g.Projects {
				if p.Name == projectName {
					for _, a := range p.Apps {
						if a == body.App {
							writeError(w, http.StatusConflict, fmt.Sprintf("app %q already in project %q", body.App, projectName))
							return
						}
					}
					data.Groups[i].Projects[j].Apps = append(data.Groups[i].Projects[j].Apps, body.App)
					if err := saveGroups(data); err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
					writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
					return
				}
			}
			writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found in group %q", projectName, groupName))
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
}

func (s *Server) handleProjectUnassignApp(w http.ResponseWriter, r *http.Request) {
	groupName := r.PathValue("name")
	if err := validateGroupName(groupName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectName := r.PathValue("project")
	if err := validateProjectName(projectName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	appName := r.PathValue("app")
	if err := validateAppName(appName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := loadGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i, g := range data.Groups {
		if g.Name == groupName {
			for j, p := range g.Projects {
				if p.Name == projectName {
					apps := make([]string, 0, len(p.Apps))
					found := false
					for _, a := range p.Apps {
						if a != appName {
							apps = append(apps, a)
						} else {
							found = true
						}
					}
					if !found {
						writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found in project %q", appName, projectName))
						return
					}
					data.Groups[i].Projects[j].Apps = apps
					if err := saveGroups(data); err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
					writeJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
					return
				}
			}
			writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found in group %q", projectName, groupName))
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("group %q not found", groupName))
}
