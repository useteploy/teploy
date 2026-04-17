package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		// Caddyfile mirror: read current, then mv temp into place.
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: "{\n\tadmin 0.0.0.0:2019\n}\n"},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
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

	mirror, ok := mock.Files[tmpCaddyfile]
	if !ok {
		t.Fatal("caddyfile mirror not uploaded")
	}
	got := string(mirror)
	for _, want := range []string{
		"# TEPLOY BEGIN myapp",
		"# TEPLOY END myapp",
		"myapp.com {",
		"reverse_proxy myapp-v1:80",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected mirrored Caddyfile to contain %q\nfull content:\n%s", want, got)
		}
	}
}

func TestSetRoute_UpdateExisting(t *testing.T) {
	existingCaddyfile := "{\n\tadmin 0.0.0.0:2019\n}\n\n" +
		"# TEPLOY BEGIN myapp\n" +
		"myapp.com {\n\treverse_proxy myapp-v1:80\n}\n" +
		"# TEPLOY END myapp\n"

	mock := ssh.NewMockExecutor("1.2.3.4",
		// ensureServer: srv0 exists.
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Output: `{"listen":[":80",":443"],"routes":[]}`},
		// putRouteByID: PATCH succeeds (route exists).
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-myapp", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
		// Caddyfile mirror.
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: existingCaddyfile},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
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

	mirror := string(mock.Files[tmpCaddyfile])
	if strings.Count(mirror, "# TEPLOY BEGIN myapp") != 1 {
		t.Errorf("expected exactly one block for myapp, got content:\n%s", mirror)
	}
	if !strings.Contains(mirror, "reverse_proxy myapp-v2:3000") {
		t.Errorf("expected mirror to contain updated upstream, got:\n%s", mirror)
	}
	if strings.Contains(mirror, "myapp-v1:80") {
		t.Errorf("old block not removed from mirror:\n%s", mirror)
	}
}

func TestRemoveRoute(t *testing.T) {
	existing := "{\n\tadmin 0.0.0.0:2019\n}\n\n" +
		"other.com {\n\treverse_proxy other:80\n}\n\n" +
		"# TEPLOY BEGIN myapp\n" +
		"myapp.com {\n\treverse_proxy myapp-v1:80\n}\n" +
		"# TEPLOY END myapp\n"

	mock := ssh.NewMockExecutor("1.2.3.4",
		// deleteRouteByID: DELETE the route.
		ssh.MockCommand{Match: "curl -sf -X DELETE http://localhost:2019/id/teploy-myapp", Output: ""},
		// Caddyfile mirror removes both the main and lb- variants.
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: existing},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
	)

	client := NewClient(mock)
	if err := client.RemoveRoute(context.Background(), "myapp"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}

	mirror := string(mock.Files[tmpCaddyfile])
	if strings.Contains(mirror, "# TEPLOY BEGIN myapp") {
		t.Errorf("expected myapp block removed, got:\n%s", mirror)
	}
	if !strings.Contains(mirror, "other.com") {
		t.Errorf("expected unrelated content preserved, got:\n%s", mirror)
	}
}

func TestRemoveRoute_NoConfig(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		// deleteRouteByID: DELETE fails (no route) — should be no-op.
		ssh.MockCommand{Match: "curl -sf -X DELETE", Err: fmt.Errorf("404")},
		// Mirror still runs even when no admin-API route existed; both calls
		// read the (empty of Teploy blocks) file and write it back unchanged.
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: "{\n\tadmin 0.0.0.0:2019\n}\n"},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
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
		// Caddyfile mirror.
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: "{\n\tadmin 0.0.0.0:2019\n}\n"},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
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

	mirror := string(mock.Files[tmpCaddyfile])
	for _, want := range []string{
		"# TEPLOY BEGIN lb-myapp",
		"# TEPLOY END lb-myapp",
		"reverse_proxy 10.0.0.1:80 10.0.0.2:80 10.0.0.3:80",
		"lb_policy round_robin",
		"health_uri /up",
	} {
		if !strings.Contains(mirror, want) {
			t.Errorf("expected mirror to contain %q, got:\n%s", want, mirror)
		}
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

func TestRemoveCaddyfileBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		app      string
		expected string
	}{
		{
			name:     "removes single block",
			input:    "a\n\n# TEPLOY BEGIN x\nbody\n# TEPLOY END x\n\nb\n",
			app:      "x",
			expected: "a\n\nb\n",
		},
		{
			name:     "no-op when marker absent",
			input:    "a\n\nb\n",
			app:      "x",
			expected: "a\n\nb\n",
		},
		{
			name:     "removes multiple blocks for same app",
			input:    "# TEPLOY BEGIN x\n1\n# TEPLOY END x\nmid\n# TEPLOY BEGIN x\n2\n# TEPLOY END x\n",
			app:      "x",
			expected: "mid\n",
		},
		{
			name:     "only removes matching app, not others",
			input:    "# TEPLOY BEGIN x\nx\n# TEPLOY END x\n# TEPLOY BEGIN y\ny\n# TEPLOY END y\n",
			app:      "x",
			expected: "# TEPLOY BEGIN y\ny\n# TEPLOY END y\n",
		},
		{
			name:     "malformed (missing end marker) truncates at begin",
			input:    "a\n# TEPLOY BEGIN x\nb\nc\n",
			app:      "x",
			expected: "a\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			begin := fmt.Sprintf(markerBeginFmt, tt.app)
			end := fmt.Sprintf(markerEndFmt, tt.app)
			got := removeCaddyfileBlock(tt.input, begin, end)
			if got != tt.expected {
				t.Errorf("removeCaddyfileBlock:\ninput:    %q\nexpected: %q\ngot:      %q", tt.input, tt.expected, got)
			}
		})
	}
}

func TestReverseProxyBlock(t *testing.T) {
	got := reverseProxyBlock([]string{"example.com"}, "myapp-v1", 8080)
	want := "example.com {\n\treverse_proxy myapp-v1:8080\n}"
	if got != want {
		t.Errorf("reverseProxyBlock:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestReverseProxyBlockMultiHost(t *testing.T) {
	got := reverseProxyBlock([]string{"example.com", "www.example.com"}, "myapp", 80)
	want := "example.com, www.example.com {\n\treverse_proxy myapp:80\n}"
	if got != want {
		t.Errorf("multi-host:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestLoadBalancerBlock(t *testing.T) {
	got := loadBalancerBlock([]string{"example.com"}, []Upstream{{Dial: "a:80"}, {Dial: "b:80"}})
	for _, want := range []string{
		"example.com {",
		"reverse_proxy a:80 b:80 {",
		"lb_policy round_robin",
		"health_uri /up",
		"health_interval 10s",
		"health_timeout 5s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("loadBalancerBlock missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestParseDomains(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"example.com", []string{"example.com"}},
		{"example.com, www.example.com", []string{"example.com", "www.example.com"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  apex.dev  ,  www.apex.dev  ", []string{"apex.dev", "www.apex.dev"}},
		{"", nil},
		{",,,", nil},
	}
	for _, tt := range tests {
		got := parseDomains(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("parseDomains(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseDomains(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSetRoute_MultiHost(t *testing.T) {
	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Output: `{"listen":[":80",":443"]}`},
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-myapp", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: "{\n\tadmin 0.0.0.0:2019\n}\n"},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com, www.myapp.com", "myapp-v1", 80); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	var route Route
	json.Unmarshal(mock.Files[tmpConfig], &route)
	if len(route.Match[0].Host) != 2 || route.Match[0].Host[0] != "myapp.com" || route.Match[0].Host[1] != "www.myapp.com" {
		t.Errorf("expected hosts [myapp.com www.myapp.com], got %v", route.Match[0].Host)
	}

	mirror := string(mock.Files[tmpCaddyfile])
	if !strings.Contains(mirror, "myapp.com, www.myapp.com {") {
		t.Errorf("expected mirror to contain multi-host site block, got:\n%s", mirror)
	}
}

// TestUpsertPreservesAdjacentContent covers the main hazard: subsequent
// upserts must not corrupt unrelated Caddyfile content (other sites, comments,
// global options).
func TestUpsertPreservesAdjacentContent(t *testing.T) {
	initial := "{\n\tadmin 0.0.0.0:2019\n}\n\n" +
		"other.com, www.other.com {\n\treverse_proxy other:80\n}\n"

	mock := ssh.NewMockExecutor("1.2.3.4",
		ssh.MockCommand{Match: "curl -sf http://localhost:2019/config/apps/http/servers/srv0", Output: `{"listen":[":80",":443"]}`},
		ssh.MockCommand{Match: "curl -sf -X PATCH http://localhost:2019/id/teploy-myapp", Output: ""},
		ssh.MockCommand{Match: "rm -f", Output: ""},
		ssh.MockCommand{Match: "cat " + caddyfilePath, Output: initial},
		ssh.MockCommand{Match: "mv " + tmpCaddyfile, Output: ""},
	)

	client := NewClient(mock)
	if err := client.SetRoute(context.Background(), "myapp", "myapp.com", "myapp-v1", 80); err != nil {
		t.Fatalf("SetRoute: %v", err)
	}

	mirror := string(mock.Files[tmpCaddyfile])
	if !strings.Contains(mirror, "other.com, www.other.com") {
		t.Errorf("unrelated site block destroyed:\n%s", mirror)
	}
	if !strings.Contains(mirror, "admin 0.0.0.0:2019") {
		t.Errorf("global options destroyed:\n%s", mirror)
	}
	if !strings.Contains(mirror, "# TEPLOY BEGIN myapp") {
		t.Errorf("new block not added:\n%s", mirror)
	}
}
