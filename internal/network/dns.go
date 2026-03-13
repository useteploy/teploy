package network

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/teploy/teploy/internal/ssh"
)

const (
	dnsBeginMarker = "# BEGIN TEPLOY"
	dnsEndMarker   = "# END TEPLOY"
)

// UpdateDNS updates /etc/hosts entries on a server to map app names to VPN IPs.
// entries maps hostname to IP, e.g. {"myapp": "100.64.0.1", "myapp-postgres": "100.64.0.2"}.
// Entries are placed between BEGIN/END TEPLOY markers so they can be updated idempotently.
func UpdateDNS(ctx context.Context, exec ssh.Executor, entries map[string]string) error {
	if len(entries) == 0 {
		return nil
	}

	// Read current /etc/hosts.
	current, err := exec.Run(ctx, "cat /etc/hosts")
	if err != nil {
		return fmt.Errorf("reading /etc/hosts: %w", err)
	}

	// Build the new teploy block.
	block := buildDNSBlock(entries)

	// Replace or append the teploy block.
	updated := replaceDNSBlock(current, block)

	// Write back via a temp file for atomicity.
	cmd := fmt.Sprintf("cat > /tmp/teploy_hosts << 'TEPLOY_HOSTS_EOF'\n%s\nTEPLOY_HOSTS_EOF\ncp /tmp/teploy_hosts /etc/hosts && rm /tmp/teploy_hosts", updated)
	if _, err := exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("updating /etc/hosts: %w", err)
	}

	return nil
}

// buildDNSBlock creates the teploy DNS block content from a map of hostname->IP entries.
func buildDNSBlock(entries map[string]string) string {
	// Sort keys for deterministic output.
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(dnsBeginMarker)
	b.WriteString("\n")
	for _, name := range names {
		fmt.Fprintf(&b, "%s %s\n", entries[name], name)
	}
	b.WriteString(dnsEndMarker)
	return b.String()
}

// replaceDNSBlock replaces the existing teploy block in hosts content, or appends it.
func replaceDNSBlock(hosts, block string) string {
	beginIdx := strings.Index(hosts, dnsBeginMarker)
	endIdx := strings.Index(hosts, dnsEndMarker)

	if beginIdx >= 0 && endIdx >= 0 {
		// Replace existing block.
		before := hosts[:beginIdx]
		after := hosts[endIdx+len(dnsEndMarker):]
		return before + block + after
	}

	// No existing block — append.
	trimmed := strings.TrimRight(hosts, "\n")
	return trimmed + "\n\n" + block + "\n"
}
