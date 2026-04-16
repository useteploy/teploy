package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/useteploy/teploy/internal/ssh"
)

func TestSetRoute_FirstDeploy(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// ensureServer: check if srv0 exists — fails (first deploy).
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Err: fmt.Errorf("404")},
		// ensureServer: create srv0.
		ssh.MockCommand{Match: "curl -sf -X PUT http://localhost:2019/config/apps/http/servers/srv0", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
		// putRouteByID: PATCH to update existing route — fails (doesn't exist yet).
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-myapp", Err: fmt.Errorf("not found")},
		// putRouteByID: POST to append new route.
		ssh.MockCommand{Match: "curl -sf -X POST http://localhost:2019/config/apps/http/servers/srv0/routes", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", "myapp-v1", 80); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	data, ok := mock.Files[tmpConfig]
	if !ok {
		t.Fatal("config not uploaded")
	}

	var route Route
	if err := json.Unmarshal(data, &route); err != nil {
		t.Fatalf("parsing uploaded config: %v", err)
	}

	if route.ID != "teploy-myapp" {
		t.Errorf("expected ID teploy-myapp, got %s", route.ID)
	}
	if route.Match[0].Host[0] != "myapp.com" {
		t.Errorf("expected host myapp.com, got %s", route.Match[0].Host[0])
	}
	if route.Handle[0].Upstreams[0].Dial != "myapp-v1:80" {
		t.Errorf("expected upstream myapp-v1:80, got %s", route.Handle[0].Upstreams[0].Dial)
	}
}

func TestSetRoute_UpdateExisting(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// ensureServer: srv0 exists.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Output: `{"listen":[":80",":443"],"routes":[]}`},
		// putRouteByID: PATCH succeeds (route exists).
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-myapp", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", "myapp-v2", 3000); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	var route Route
	json.Unmarshal(mock.Files[tmpConfig], &route)

	if route.Handle[0].Upstreams[0].Dial != "myapp-v2:3000" {
		t.Errorf("expected updated upstream myapp-v2:3000, got %s", route.Handle[0].Upstreams[0].Dial)
	}
}

func TestRemoveRoute(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// deleteRouteByID: DELETE the route.
		ssh.MockCommand{Match: "curl -sf -X DELETE http://localhost:2019/id/teploy-myapp", Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveRoute(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}
}

func TestRemoveRoute_NoConfig(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// deleteRouteByID: DELETE fails (no route) — should be no-op.
		ssh.MockCommand{Match: "curl -sf -X DELETE", Err: fmt.Errorf("404")},
	)

	client := NewClient(mock)
	if err := client.RemoveRoute(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveRoute should be no-op when no config: %v", err)
	}
}

func TestSetLoadBalancer_FirstDeploy(t *testing.T) {
	mock := ssh.NewMockExecutor("10.0.0.100",
		// ensureServer: srv0 doesn't exist.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Err: fmt.Errorf("404")},
		// ensureServer: create srv0.
		ssh.MockCommand{Match: "curl -sf -X PUT http://localhost:2019/config/apps/http/servers/srv0", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
		// putRouteByID: PATCH fails (new route).
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-lb-myapp", Err: fmt.Errorf("not found")},
		// putRouteByID: POST to append.
		ssh.MockCommand{Match: "curl -sf -X POST http://localhost:2019/config/apps/http/servers/srv0/routes", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	upstreams := []Upstream{
		{Dial: "10.0.0.1:80"},
		{Dial: "10.0.0.2:80"},
		{Dial: "10.0.0.3:80"},
	}

	if err := client.SetLoadBalancer(context.Background(), "myapp", "myapp.com", upstreams); err != nil {
		t.Fatalf("SetLoadBalancer: %v", err)
	}

	data, ok := mock.Files[tmpConfig]
	if !ok {
		t.Fatal("config not uploaded")
	}

	var route Route
	if err := json.Unmarshal(data, &route); err != nil {
		t.Fatalf("parsing uploaded config: %v", err)
	}

	if route.ID != "teploy-lb-myapp" {
		t.Errorf("expected ID teploy-lb-myapp, got %s", route.ID)
	}
	if route.Match[0].Host[0] != "myapp.com" {
		t.Errorf("expected host myapp.com, got %s", route.Match[0].Host[0])
	}

	handler := route.Handle[0]
	if len(handler.Upstreams) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(handler.Upstreams))
	}
	if handler.Upstreams[0].Dial != "10.0.0.1:80" {
		t.Errorf("expected upstream 10.0.0.1:80, got %s", handler.Upstreams[0].Dial)
	}

	if handler.HealthChecks == nil || handler.HealthChecks.Active == nil {
		t.Fatal("expected active health checks")
	}
	if handler.HealthChecks.Active.Path != "/up" {
		t.Errorf("expected health check path /up, got %s", handler.HealthChecks.Active.Path)
	}

	if handler.LoadBalancing == nil || handler.LoadBalancing.SelectionPolicy == nil {
		t.Fatal("expected load balancing config")
	}
	if handler.LoadBalancing.SelectionPolicy.Policy != "round_robin" {
		t.Errorf("expected round_robin policy, got %s", handler.LoadBalancing.SelectionPolicy.Policy)
	}
}

func TestSetMaintenance(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:80"}]}]}]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		// ensureServer: srv0 exists.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Output: `{"listen":[":80",":443"]}`},
		// deleteRouteByID for existing maint route (cleanup).
		ssh.MockCommand{Match: "curl -sf -X DELETE http://localhost:2019/id/teploy-maint-myapp", Err: fmt.Errorf("not found")},
		// getHTTPApp to read current routes for prepend.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		// PUT the full routes array with maint prepended.
		ssh.MockCommand{Match: "curl -sf -X PUT http://localhost:2019/config/apps/http/servers/srv0/routes", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetMaintenance(context.Background(), "myapp", "myapp.com"); err != nil {
		t.Fatalf("SetMaintenance: %v", err)
	}

	var routes []Route
	json.Unmarshal(mock.Files[tmpConfig], &routes)

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes (maint + original), got %d", len(routes))
	}

	if routes[0].ID != "teploy-maint-myapp" {
		t.Errorf("expected maintenance route first, got %s", routes[0].ID)
	}
	if routes[0].Handle[0].Handler != "static_response" {
		t.Errorf("expected static_response handler, got %s", routes[0].Handle[0].Handler)
	}
	if routes[0].Handle[0].StatusCode != "503" {
		t.Errorf("expected 503, got %s", routes[0].Handle[0].StatusCode)
	}

	if routes[1].ID != "teploy-myapp" {
		t.Errorf("expected original route preserved, got %s", routes[1].ID)
	}
}

func TestRemoveMaintenance(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf -X DELETE http://localhost:2019/id/teploy-maint-myapp", Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveMaintenance(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveMaintenance: %v", err)
	}
}
