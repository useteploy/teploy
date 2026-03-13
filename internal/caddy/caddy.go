package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/useteploy/teploy/internal/ssh"
)

const (
	adminAPI  = "http://localhost:2019"
	tmpConfig = "/tmp/teploy_caddy_config.json"
)

// HTTPApp represents Caddy's HTTP application configuration.
type HTTPApp struct {
	Servers map[string]*HTTPServer `json:"servers"`
}

// HTTPServer is a Caddy HTTP server with listen addresses and routes.
type HTTPServer struct {
	Listen []string `json:"listen"`
	Routes []Route  `json:"routes"`
}

// Route is a single Caddy routing rule identified by an @id.
type Route struct {
	ID     string    `json:"@id,omitempty"`
	Match  []Match   `json:"match,omitempty"`
	Handle []Handler `json:"handle"`
}

// Match defines route matching criteria.
type Match struct {
	Host []string `json:"host"`
}

// Handler defines how a matched request is processed.
type Handler struct {
	Handler       string              `json:"handler"`
	Upstreams     []Upstream          `json:"upstreams,omitempty"`
	HealthChecks  *HealthChecks       `json:"health_checks,omitempty"`
	LoadBalancing *LoadBalancing      `json:"load_balancing,omitempty"`
	StatusCode    string              `json:"status_code,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
}

// Upstream is a reverse proxy target address.
type Upstream struct {
	Dial string `json:"dial"`
}

// HealthChecks configures active health checking for upstreams.
type HealthChecks struct {
	Active *ActiveHealthCheck `json:"active,omitempty"`
}

// ActiveHealthCheck configures how Caddy actively probes upstream health.
type ActiveHealthCheck struct {
	Path     string `json:"path,omitempty"`
	Interval string `json:"interval,omitempty"` // duration string, e.g. "10s"
	Timeout  string `json:"timeout,omitempty"`  // duration string, e.g. "5s"
}

// LoadBalancing configures load balancing strategy.
type LoadBalancing struct {
	SelectionPolicy *SelectionPolicy `json:"selection_policy,omitempty"`
}

// SelectionPolicy defines how an upstream is selected.
type SelectionPolicy struct {
	Policy string `json:"policy,omitempty"` // round_robin, least_conn, etc.
}

// Client communicates with the Caddy admin API on a remote server via SSH.
// The admin API is accessible at localhost:2019 on the server (bound to
// 127.0.0.1 only, never publicly exposed).
type Client struct {
	exec ssh.Executor
}

// NewClient creates a Caddy admin API client.
func NewClient(exec ssh.Executor) *Client {
	return &Client{exec: exec}
}

// SetRoute adds or updates a reverse proxy route for the given app.
// Routes traffic for the domain to the app's Docker network alias at the
// specified port. Caddy provisions HTTPS certificates automatically.
func (c *Client) SetRoute(ctx context.Context, app, domain string, port int) error {
	// Ignore errors from getHTTPApp — Caddy may not have HTTP config yet (first deploy).
	httpApp, _ := c.getHTTPApp(ctx) //nolint:errcheck // expected to fail on first deploy
	if httpApp == nil {
		httpApp = &HTTPApp{}
	}
	if httpApp.Servers == nil {
		httpApp.Servers = map[string]*HTTPServer{}
	}

	srv := httpApp.Servers["srv0"]
	if srv == nil {
		srv = &HTTPServer{Listen: []string{":80", ":443"}}
		httpApp.Servers["srv0"] = srv
	}

	routeID := "teploy-" + app
	newRoute := Route{
		ID:    routeID,
		Match: []Match{{Host: []string{domain}}},
		Handle: []Handler{{
			Handler:   "reverse_proxy",
			Upstreams: []Upstream{{Dial: fmt.Sprintf("%s:%d", app, port)}},
		}},
	}

	found := false
	for i, r := range srv.Routes {
		if r.ID == routeID {
			srv.Routes[i] = newRoute
			found = true
			break
		}
	}
	if !found {
		srv.Routes = append(srv.Routes, newRoute)
	}

	return c.putHTTPApp(ctx, httpApp)
}

// SetLoadBalancer adds or updates a load-balanced reverse proxy route for the
// given app. Traffic for the domain is distributed across multiple upstreams
// using round-robin with active health checks.
func (c *Client) SetLoadBalancer(ctx context.Context, app, domain string, upstreams []Upstream) error {
	httpApp, _ := c.getHTTPApp(ctx) //nolint:errcheck // expected to fail on first deploy
	if httpApp == nil {
		httpApp = &HTTPApp{}
	}
	if httpApp.Servers == nil {
		httpApp.Servers = map[string]*HTTPServer{}
	}

	srv := httpApp.Servers["srv0"]
	if srv == nil {
		srv = &HTTPServer{Listen: []string{":80", ":443"}}
		httpApp.Servers["srv0"] = srv
	}

	routeID := "teploy-lb-" + app
	newRoute := Route{
		ID:    routeID,
		Match: []Match{{Host: []string{domain}}},
		Handle: []Handler{{
			Handler:   "reverse_proxy",
			Upstreams: upstreams,
			HealthChecks: &HealthChecks{
				Active: &ActiveHealthCheck{
					Path:     "/up",
					Interval: "10s",
					Timeout:  "5s",
				},
			},
			LoadBalancing: &LoadBalancing{
				SelectionPolicy: &SelectionPolicy{
					Policy: "round_robin",
				},
			},
		}},
	}

	found := false
	for i, r := range srv.Routes {
		if r.ID == routeID {
			srv.Routes[i] = newRoute
			found = true
			break
		}
	}
	if !found {
		srv.Routes = append(srv.Routes, newRoute)
	}

	return c.putHTTPApp(ctx, httpApp)
}

// maintenancePage is the HTML returned during maintenance mode.
const maintenancePage = `<!DOCTYPE html>
<html><head>
<title>Maintenance</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{font-family:-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f5f5f5}
.box{text-align:center;padding:2rem}
h1{font-size:1.5rem;color:#333}
p{color:#666}
</style>
</head><body>
<div class="box">
<h1>We'll be back soon</h1>
<p>This site is currently undergoing maintenance. Please check back shortly.</p>
</div>
</body></html>`

// SetMaintenance enables maintenance mode for the given app.
// Inserts a 503 static response route before all other routes so it intercepts
// traffic for the domain. The existing reverse proxy route is left in place.
func (c *Client) SetMaintenance(ctx context.Context, app, domain string) error {
	httpApp, _ := c.getHTTPApp(ctx)
	if httpApp == nil {
		httpApp = &HTTPApp{}
	}
	if httpApp.Servers == nil {
		httpApp.Servers = map[string]*HTTPServer{}
	}

	srv := httpApp.Servers["srv0"]
	if srv == nil {
		srv = &HTTPServer{Listen: []string{":80", ":443"}}
		httpApp.Servers["srv0"] = srv
	}

	routeID := "teploy-maint-" + app
	maintRoute := Route{
		ID:    routeID,
		Match: []Match{{Host: []string{domain}}},
		Handle: []Handler{{
			Handler:    "static_response",
			StatusCode: "503",
			Headers: map[string][]string{
				"Content-Type": {"text/html; charset=utf-8"},
				"Retry-After":  {"3600"},
			},
			Body: maintenancePage,
		}},
	}

	// Remove existing maintenance route if present, then prepend.
	filtered := []Route{maintRoute}
	for _, r := range srv.Routes {
		if r.ID != routeID {
			filtered = append(filtered, r)
		}
	}
	srv.Routes = filtered

	return c.putHTTPApp(ctx, httpApp)
}

// RemoveMaintenance disables maintenance mode for the given app.
func (c *Client) RemoveMaintenance(ctx context.Context, app string) error {
	httpApp, err := c.getHTTPApp(ctx)
	if err != nil || httpApp == nil {
		return nil
	}

	srv := httpApp.Servers["srv0"]
	if srv == nil {
		return nil
	}

	routeID := "teploy-maint-" + app
	filtered := make([]Route, 0, len(srv.Routes))
	for _, r := range srv.Routes {
		if r.ID != routeID {
			filtered = append(filtered, r)
		}
	}
	srv.Routes = filtered

	return c.putHTTPApp(ctx, httpApp)
}

// RemoveRoute removes the route for the given app. No-op if no route exists.
func (c *Client) RemoveRoute(ctx context.Context, app string) error {
	httpApp, err := c.getHTTPApp(ctx)
	if err != nil || httpApp == nil {
		return nil
	}

	srv := httpApp.Servers["srv0"]
	if srv == nil {
		return nil
	}

	routeID := "teploy-" + app
	filtered := make([]Route, 0, len(srv.Routes))
	for _, r := range srv.Routes {
		if r.ID != routeID {
			filtered = append(filtered, r)
		}
	}
	srv.Routes = filtered

	return c.putHTTPApp(ctx, httpApp)
}

func (c *Client) getHTTPApp(ctx context.Context) (*HTTPApp, error) {
	output, err := c.exec.Run(ctx, "curl -sf "+adminAPI+"/config/apps/http")
	if err != nil {
		return nil, err
	}

	var app HTTPApp
	if err := json.Unmarshal([]byte(output), &app); err != nil {
		return nil, fmt.Errorf("parsing caddy HTTP config: %w", err)
	}
	return &app, nil
}

func (c *Client) putHTTPApp(ctx context.Context, app *HTTPApp) error {
	body, err := json.Marshal(app)
	if err != nil {
		return fmt.Errorf("marshaling caddy config: %w", err)
	}

	if err := c.exec.Upload(ctx, bytes.NewReader(body), tmpConfig, "0644"); err != nil {
		return fmt.Errorf("uploading caddy config: %w", err)
	}

	cmd := fmt.Sprintf(
		"curl -sf -X PUT %s/config/apps/http -H 'Content-Type: application/json' -d @%s",
		adminAPI, tmpConfig,
	)
	_, err = c.exec.Run(ctx, cmd)

	if err != nil {
		// PUT may fail if path doesn't exist yet — try DELETE + PUT as fallback.
		c.exec.Run(ctx, fmt.Sprintf("curl -sf -X DELETE %s/config/apps/http", adminAPI))
		_, err = c.exec.Run(ctx, cmd)
	}

	// Clean up temp file regardless.
	c.exec.Run(ctx, "rm -f "+tmpConfig)

	if err != nil {
		return fmt.Errorf("applying caddy config: %w", err)
	}

	return nil
}
