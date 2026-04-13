package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/useteploy/teploy/internal/ssh"
)

// testPool creates a ConnPool with a mock injected for the given server name.
// This bypasses the real SSH connect path for unit testing.
func testPool(serverName string, mock *ssh.MockExecutor) *ConnPool {
	pool := NewConnPool()
	pool.conns[serverName] = &poolEntry{
		exec:     mock,
		lastUsed: time.Now(),
	}
	return pool
}

func TestConnPoolGetExisting(t *testing.T) {
	mock := ssh.NewMockExecutor("example.com",
		ssh.MockCommand{Match: "echo ok", Output: "ok"},
	)
	pool := testPool("test-server", mock)

	exec, err := pool.Get(context.Background(), "test-server")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if exec != mock {
		t.Error("expected to get the mock executor back")
	}
}

func TestConnPoolGetReconnectsOnDeadConnection(t *testing.T) {
	dead := ssh.NewMockExecutor("example.com") // no "echo ok" command registered → will error
	pool := testPool("test-server", dead)

	// Get will try health check, fail, and try to reconnect via config.ResolveServer.
	// Since we can't mock config.ResolveServer easily, we just verify the dead connection
	// is removed from the pool after the health check fails.
	_, err := pool.Get(context.Background(), "test-server")
	if err == nil {
		t.Fatal("expected error when reconnecting without real server config")
	}

	pool.mu.Lock()
	_, stillThere := pool.conns["test-server"]
	pool.mu.Unlock()
	if stillThere {
		t.Error("dead connection should have been removed from pool")
	}
}

func TestConnPoolSweep(t *testing.T) {
	mock := ssh.NewMockExecutor("example.com")
	pool := NewConnPool()
	pool.conns["old-server"] = &poolEntry{
		exec:     mock,
		lastUsed: time.Now().Add(-10 * time.Minute), // well past TTL
	}
	pool.conns["fresh-server"] = &poolEntry{
		exec:     ssh.NewMockExecutor("fresh.com"),
		lastUsed: time.Now(),
	}

	pool.sweep()

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if _, ok := pool.conns["old-server"]; ok {
		t.Error("expired connection should have been swept")
	}
	if _, ok := pool.conns["fresh-server"]; !ok {
		t.Error("fresh connection should not have been swept")
	}
}

func TestConnPoolStop(t *testing.T) {
	mock := ssh.NewMockExecutor("example.com",
		ssh.MockCommand{Match: "echo ok", Output: "ok"},
	)
	pool := testPool("test-server", mock)
	pool.Start()
	pool.Stop()

	pool.mu.Lock()
	count := len(pool.conns)
	pool.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 connections after Stop, got %d", count)
	}
}

func TestConnPoolConcurrentAccess(t *testing.T) {
	mock := ssh.NewMockExecutor("example.com",
		ssh.MockCommand{Match: "echo ok", Output: "ok"},
	)
	pool := testPool("test-server", mock)
	pool.Start()
	defer pool.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Get(context.Background(), "test-server")
		}()
	}
	wg.Wait()
}
