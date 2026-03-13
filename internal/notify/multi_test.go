package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMultiNotifier_FiltersByEvent(t *testing.T) {
	var deployHits, healthHits int32

	deploySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&deployHits, 1)
		w.WriteHeader(200)
	}))
	defer deploySrv.Close()

	healthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&healthHits, 1)
		w.WriteHeader(200)
	}))
	defer healthSrv.Close()

	n := NewMultiNotifier([]Channel{
		{Type: "webhook", URL: deploySrv.URL, Events: []string{"deploy", "rollback"}},
		{Type: "webhook", URL: healthSrv.URL, Events: []string{"health_failure"}},
	})

	// Send a deploy event — should only hit deploy webhook.
	n.Send(context.Background(), Payload{Type: "deploy", App: "myapp"})
	if atomic.LoadInt32(&deployHits) != 1 {
		t.Errorf("deploy webhook: expected 1 hit, got %d", deployHits)
	}
	if atomic.LoadInt32(&healthHits) != 0 {
		t.Errorf("health webhook: expected 0 hits, got %d", healthHits)
	}

	// Send a health_failure event — should only hit health webhook.
	n.Send(context.Background(), Payload{Type: "health_failure", App: "myapp"})
	if atomic.LoadInt32(&deployHits) != 1 {
		t.Errorf("deploy webhook: expected 1 hit, got %d", deployHits)
	}
	if atomic.LoadInt32(&healthHits) != 1 {
		t.Errorf("health webhook: expected 1 hit, got %d", healthHits)
	}
}

func TestMultiNotifier_AllEventsWhenEmpty(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n := NewMultiNotifier([]Channel{
		{Type: "webhook", URL: srv.URL}, // no events filter = all
	})

	n.Send(context.Background(), Payload{Type: "deploy", App: "myapp"})
	n.Send(context.Background(), Payload{Type: "rollback", App: "myapp"})
	n.Send(context.Background(), Payload{Type: "health_failure", App: "myapp"})

	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("expected 3 hits, got %d", hits)
	}
}

func TestMultiNotifier_NilWhenEmpty(t *testing.T) {
	n := NewMultiNotifier(nil)
	if n != nil {
		t.Error("expected nil notifier for empty channels")
	}
}

func TestMatchesEvent(t *testing.T) {
	tests := []struct {
		filter []string
		event  string
		want   bool
	}{
		{nil, "deploy", true},
		{[]string{}, "deploy", true},
		{[]string{"deploy"}, "deploy", true},
		{[]string{"deploy"}, "rollback", false},
		{[]string{"deploy", "rollback"}, "rollback", true},
	}

	for _, tt := range tests {
		got := matchesEvent(tt.filter, tt.event)
		if got != tt.want {
			t.Errorf("matchesEvent(%v, %q) = %v, want %v", tt.filter, tt.event, got, tt.want)
		}
	}
}
