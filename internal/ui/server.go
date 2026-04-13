package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
)

//go:embed static
var staticFiles embed.FS

// Server is the embedded web dashboard HTTP server.
type Server struct {
	pool   *ConnPool
	addr   string
	server *http.Server
}

// NewServer creates a new UI server bound to the given address.
func NewServer(addr string) *Server {
	return &Server{
		pool: NewConnPool(),
		addr: addr,
	}
}

// Start starts the HTTP server and connection pool sweep goroutine.
// It blocks until the server is shut down.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	s.pool.Start()

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}

	log.Printf("teploy ui running at http://%s", s.addr)
	return s.server.Serve(ln)
}

// Stop gracefully shuts down the server and closes all SSH connections.
func (s *Server) Stop() {
	if s.server != nil {
		s.server.Close()
	}
	s.pool.Stop()
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static files
	staticSub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServer(http.FS(staticSub)))

	// Server API
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("GET /api/servers/{name}/status", s.handleServerStatus)

	// Server proxy (Caddy) API
	mux.HandleFunc("GET /api/servers/{name}/proxy", s.handleServerProxy)

	// App API
	mux.HandleFunc("GET /api/apps", s.handleListApps)
	mux.HandleFunc("GET /api/apps/{name}/status", s.handleAppStatus)
	mux.HandleFunc("GET /api/apps/{name}/env", s.handleGetEnv)
	mux.HandleFunc("POST /api/apps/{name}/env", s.handleSetEnv)
	mux.HandleFunc("DELETE /api/apps/{name}/env/{key}", s.handleUnsetEnv)
	mux.HandleFunc("GET /api/apps/{name}/log", s.handleAppLog)
	mux.HandleFunc("GET /api/apps/{name}/accessories", s.handleListAccessories)
	mux.HandleFunc("POST /api/apps/{name}/stop", s.handleAppStop)
	mux.HandleFunc("POST /api/apps/{name}/start", s.handleAppStart)
	mux.HandleFunc("POST /api/apps/{name}/restart", s.handleAppRestart)
	mux.HandleFunc("POST /api/apps/{name}/rollback", s.handleAppRollback)
	mux.HandleFunc("POST /api/apps/{name}/lock", s.handleAppLock)
	mux.HandleFunc("POST /api/apps/{name}/unlock", s.handleAppUnlock)
	mux.HandleFunc("POST /api/apps/{name}/maintenance/{action}", s.handleAppMaintenance)
	mux.HandleFunc("POST /api/apps/{name}/accessories/{acc}/start", s.handleAccessoryStart)
	mux.HandleFunc("POST /api/apps/{name}/accessories/{acc}/stop", s.handleAccessoryStop)

	// Group API
	mux.HandleFunc("GET /api/groups", s.handleListGroups)
	mux.HandleFunc("POST /api/groups", s.handleCreateGroup)
	mux.HandleFunc("PUT /api/groups/{name}", s.handleRenameGroup)
	mux.HandleFunc("DELETE /api/groups/{name}", s.handleDeleteGroup)
	mux.HandleFunc("POST /api/groups/{name}/apps", s.handleAssignApp)
	mux.HandleFunc("DELETE /api/groups/{name}/apps/{app}", s.handleUnassignApp)

	// Project API
	mux.HandleFunc("POST /api/groups/{name}/projects", s.handleCreateProject)
	mux.HandleFunc("PUT /api/groups/{name}/projects/{project}", s.handleRenameProject)
	mux.HandleFunc("DELETE /api/groups/{name}/projects/{project}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/groups/{name}/projects/{project}/apps", s.handleProjectAssignApp)
	mux.HandleFunc("DELETE /api/groups/{name}/projects/{project}/apps/{app}", s.handleProjectUnassignApp)

	// Config API
	mux.HandleFunc("GET /api/config/servers", s.handleConfigListServers)
	mux.HandleFunc("POST /api/config/servers", s.handleConfigAddServer)
	mux.HandleFunc("PUT /api/config/servers/{name}", s.handleConfigEditServer)
	mux.HandleFunc("DELETE /api/config/servers/{name}", s.handleConfigDeleteServer)
	mux.HandleFunc("GET /api/config/notifications", s.handleGetNotifications)
	mux.HandleFunc("POST /api/config/notifications", s.handleSetNotifications)
	mux.HandleFunc("GET /api/config/registries", s.handleListRegistries)
	mux.HandleFunc("POST /api/config/registries", s.handleAddRegistry)
	mux.HandleFunc("DELETE /api/config/registries/{server}", s.handleDeleteRegistry)

	// Deploy
	mux.HandleFunc("POST /api/deploy", s.handleDeploy)

	// WebSocket
	mux.HandleFunc("GET /ws/logs/{name}", s.handleWSLogs)
}

// OpenBrowser opens the default browser to the given URL.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}

// apiResponse is the standard JSON envelope for API responses.
type apiResponse struct {
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(apiResponse{Data: data}); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{Error: message})
}
