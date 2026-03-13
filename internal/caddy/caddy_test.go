package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/teploy/teploy/internal/ssh"
)

func TestSetRoute_FirstDeploy(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// No existing HTTP config.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Err: fmt.Errorf("404")},
		// PUT new config.
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", 49152); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	data, ok := mock.Files[tmpConfig]
	if !ok {
		t.Fatal("config not uploaded")
	}

	var app HTTPApp
	if err := json.Unmarshal(data, &app); err != nil {
		t.Fatalf("parsing uploaded config: %v", err)
	}

	srv := app.Servers["srv0"]
	if srv == nil {
		t.Fatal("expected srv0 server")
	}
	if len(srv.Listen) != 2 {
		t.Errorf("expected 2 listen addresses, got %d", len(srv.Listen))
	}
	if len(srv.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(srv.Routes))
	}

	r := srv.Routes[0]
	if r.ID != "teploy-myapp" {
		t.Errorf("expected ID teploy-myapp, got %s", r.ID)
	}
	if r.Match[0].Host[0] != "myapp.com" {
		t.Errorf("expected host myapp.com, got %s", r.Match[0].Host[0])
	}
	if r.Handle[0].Upstreams[0].Dial != "myapp:49152" {
		t.Errorf("expected upstream myapp:49152, got %s", r.Handle[0].Upstreams[0].Dial)
	}
}

func TestSetRoute_UpdateExisting(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:49152"}]}]}]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", 49153); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route (updated, not duplicated), got %d", len(routes))
	}
	if routes[0].Handle[0].Upstreams[0].Dial != "myapp:49153" {
		t.Errorf("expected updated upstream myapp:49153, got %s", routes[0].Handle[0].Upstreams[0].Dial)
	}
}

func TestSetRoute_PreserveOtherRoutes(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-otherapp","match":[{"host":["other.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"otherapp:49152"}]}]}]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", 49153); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	ids := map[string]bool{}
	for _, r := range routes {
		ids[r.ID] = true
	}
	if !ids["teploy-otherapp"] {
		t.Error("otherapp route should be preserved")
	}
	if !ids["teploy-myapp"] {
		t.Error("myapp route should be added")
	}
}

func TestRemoveRoute(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[` +
		`{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:49152"}]}]},` +
		`{"@id":"teploy-otherapp","match":[{"host":["other.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"otherapp:49153"}]}]}` +
		`]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveRoute(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after removal, got %d", len(routes))
	}
	if routes[0].ID != "teploy-otherapp" {
		t.Errorf("expected otherapp to remain, got %s", routes[0].ID)
	}
}

func TestRemoveRoute_NoConfig(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl", Err: fmt.Errorf("404")},
	)

	client := NewClient(mock)
	if err := client.RemoveRoute(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveRoute should be no-op when no config: %v", err)
	}
}

func TestSetLoadBalancer_FirstDeploy(t *testing.T) {
	mock := ssh.NewMockExecutor("10.0.0.100",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Err: fmt.Errorf("404")},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
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

	var app HTTPApp
	if err := json.Unmarshal(data, &app); err != nil {
		t.Fatalf("parsing uploaded config: %v", err)
	}

	srv := app.Servers["srv0"]
	if srv == nil {
		t.Fatal("expected srv0 server")
	}
	if len(srv.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(srv.Routes))
	}

	r := srv.Routes[0]
	if r.ID != "teploy-lb-myapp" {
		t.Errorf("expected ID teploy-lb-myapp, got %s", r.ID)
	}
	if r.Match[0].Host[0] != "myapp.com" {
		t.Errorf("expected host myapp.com, got %s", r.Match[0].Host[0])
	}

	handler := r.Handle[0]
	if len(handler.Upstreams) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(handler.Upstreams))
	}
	if handler.Upstreams[0].Dial != "10.0.0.1:80" {
		t.Errorf("expected upstream 10.0.0.1:80, got %s", handler.Upstreams[0].Dial)
	}
	if handler.Upstreams[1].Dial != "10.0.0.2:80" {
		t.Errorf("expected upstream 10.0.0.2:80, got %s", handler.Upstreams[1].Dial)
	}
	if handler.Upstreams[2].Dial != "10.0.0.3:80" {
		t.Errorf("expected upstream 10.0.0.3:80, got %s", handler.Upstreams[2].Dial)
	}

	// Verify health checks.
	if handler.HealthChecks == nil || handler.HealthChecks.Active == nil {
		t.Fatal("expected active health checks")
	}
	if handler.HealthChecks.Active.Path != "/up" {
		t.Errorf("expected health check path /up, got %s", handler.HealthChecks.Active.Path)
	}
	if handler.HealthChecks.Active.Interval != "10s" {
		t.Errorf("expected interval 10s, got %s", handler.HealthChecks.Active.Interval)
	}

	// Verify load balancing.
	if handler.LoadBalancing == nil || handler.LoadBalancing.SelectionPolicy == nil {
		t.Fatal("expected load balancing config")
	}
	if handler.LoadBalancing.SelectionPolicy.Policy != "round_robin" {
		t.Errorf("expected round_robin policy, got %s", handler.LoadBalancing.SelectionPolicy.Policy)
	}
}

func TestSetLoadBalancer_UpdateExisting(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-lb-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"10.0.0.1:80"}]}]}]}}}`

	mock := ssh.NewMockExecutor("10.0.0.100",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	upstreams := []Upstream{
		{Dial: "10.0.0.1:80"},
		{Dial: "10.0.0.2:80"},
	}

	if err := client.SetLoadBalancer(context.Background(), "myapp", "myapp.com", upstreams); err != nil {
		t.Fatalf("SetLoadBalancer: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route (updated, not duplicated), got %d", len(routes))
	}
	if len(routes[0].Handle[0].Upstreams) != 2 {
		t.Fatalf("expected 2 upstreams after update, got %d", len(routes[0].Handle[0].Upstreams))
	}
}

func TestSetLoadBalancer_PreservesOtherRoutes(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-otherapp","match":[{"host":["other.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"otherapp:49152"}]}]}]}}}`

	mock := ssh.NewMockExecutor("10.0.0.100",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	upstreams := []Upstream{{Dial: "10.0.0.1:80"}}

	if err := client.SetLoadBalancer(context.Background(), "myapp", "myapp.com", upstreams); err != nil {
		t.Fatalf("SetLoadBalancer: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	ids := map[string]bool{}
	for _, r := range routes {
		ids[r.ID] = true
	}
	if !ids["teploy-otherapp"] {
		t.Error("otherapp route should be preserved")
	}
	if !ids["teploy-lb-myapp"] {
		t.Error("myapp LB route should be added")
	}
}

func TestSetMaintenance(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:49152"}]}]}]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X DELETE", Output: ""},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetMaintenance(context.Background(), "myapp", "myapp.com"); err != nil {
		t.Fatalf("SetMaintenance: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes (maint + original), got %d", len(routes))
	}

	// Maintenance route should be first (takes priority).
	if routes[0].ID != "teploy-maint-myapp" {
		t.Errorf("expected maintenance route first, got %s", routes[0].ID)
	}
	if routes[0].Handle[0].Handler != "static_response" {
		t.Errorf("expected static_response handler, got %s", routes[0].Handle[0].Handler)
	}
	if routes[0].Handle[0].StatusCode != "503" {
		t.Errorf("expected 503, got %s", routes[0].Handle[0].StatusCode)
	}

	// Original route should still be there.
	if routes[1].ID != "teploy-myapp" {
		t.Errorf("expected original route preserved, got %s", routes[1].ID)
	}
}

func TestRemoveMaintenance(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[` +
		`{"@id":"teploy-maint-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"static_response","status_code":"503"}]},` +
		`{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:49152"}]}]}` +
		`]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X DELETE", Output: ""},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveMaintenance(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveMaintenance: %v", err)
	}

	var app HTTPApp
	json.Unmarshal(mock.Files[tmpConfig], &app)

	routes := app.Servers["srv0"].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after removing maintenance, got %d", len(routes))
	}
	if routes[0].ID != "teploy-myapp" {
		t.Errorf("expected original route to remain, got %s", routes[0].ID)
	}
}

func TestRemoveMaintenance_NoMaintRoute(t *testing.T) {
	existing := `{"servers":{"srv0":{"listen":[":80",":443"],"routes":[{"@id":"teploy-myapp","match":[{"host":["myapp.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"myapp:49152"}]}]}]}}}`

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http", Output: existing},
		ssh.MockCommand{Match: "curl -sf -X DELETE", Output: ""},
		ssh.MockCommand{Match: "curl -sf -X PUT", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveMaintenance(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveMaintenance should be no-op: %v", err)
	}
}
