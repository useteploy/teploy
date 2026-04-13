package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/useteploy/teploy/internal/config"
	"github.com/useteploy/teploy/internal/ssh"
)

const (
	connTTL       = 5 * time.Minute
	sweepInterval = 1 * time.Minute
)

type poolEntry struct {
	exec     ssh.Executor
	lastUsed time.Time
}

// ConnPool manages a pool of SSH connections to servers.
// Connections are lazily created and expire after 5 minutes of inactivity.
type ConnPool struct {
	mu      sync.Mutex
	conns   map[string]*poolEntry
	stop    chan struct{}
	stopped chan struct{}
}

// NewConnPool creates a new connection pool. Call Start() to begin the sweep goroutine.
func NewConnPool() *ConnPool {
	return &ConnPool{
		conns:   make(map[string]*poolEntry),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// Start begins the background goroutine that sweeps expired connections.
func (p *ConnPool) Start() {
	go p.sweepLoop()
}

// Stop stops the sweep goroutine and closes all connections.
func (p *ConnPool) Stop() {
	close(p.stop)
	<-p.stopped
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, entry := range p.conns {
		entry.exec.Close()
		delete(p.conns, name)
	}
}

// Get returns an executor for the named server, creating a new connection if needed.
// Existing connections are health-checked before reuse.
func (p *ConnPool) Get(ctx context.Context, serverName string) (ssh.Executor, error) {
	p.mu.Lock()
	entry, ok := p.conns[serverName]
	if ok {
		entry.lastUsed = time.Now()
		p.mu.Unlock()

		// Health check: verify connection is still alive.
		if _, err := entry.exec.Run(ctx, "echo ok"); err == nil {
			return entry.exec, nil
		}
		// Connection dead — remove and reconnect.
		p.mu.Lock()
		delete(p.conns, serverName)
		entry.exec.Close()
		p.mu.Unlock()
	} else {
		p.mu.Unlock()
	}

	// Connect using config.ResolveServer.
	host, user, key, err := config.ResolveServer(serverName, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("resolving server %q: %w", serverName, err)
	}

	exec, err := ssh.Connect(ctx, ssh.ConnectConfig{
		Host:    host,
		User:    user,
		KeyPath: key,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", serverName, err)
	}

	p.mu.Lock()
	p.conns[serverName] = &poolEntry{
		exec:     exec,
		lastUsed: time.Now(),
	}
	p.mu.Unlock()

	return exec, nil
}

// sweepLoop removes connections that haven't been used within the TTL.
func (p *ConnPool) sweepLoop() {
	defer close(p.stopped)
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.sweep()
		}
	}
}

func (p *ConnPool) sweep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cutoff := time.Now().Add(-connTTL)
	for name, entry := range p.conns {
		if entry.lastUsed.Before(cutoff) {
			entry.exec.Close()
			delete(p.conns, name)
		}
	}
}
