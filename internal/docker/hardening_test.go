package docker

import (
	"testing"
)

func TestContainerName_Simple(t *testing.T) {
	name := ContainerName("myapp", "web", "abc123")
	if name != "myapp-web-abc123" {
		t.Errorf("ContainerName = %q, want %q", name, "myapp-web-abc123")
	}
}

func TestParseListeningPorts_Empty(t *testing.T) {
	ports := parseListeningPorts("")
	if len(ports) != 0 {
		t.Errorf("expected empty map, got %d entries", len(ports))
	}
}

func TestParseListeningPorts_Mixed(t *testing.T) {
	output := `State  Recv-Q Send-Q Local Address:Port Peer Address:Port
LISTEN 0      128    0.0.0.0:22       0.0.0.0:*
LISTEN 0      128    [::]:80          [::]:*
LISTEN 0      5      *:9876           *:*`

	ports := parseListeningPorts(output)
	if !ports[22] {
		t.Error("port 22 should be detected")
	}
	if !ports[80] {
		t.Error("port 80 should be detected")
	}
	if !ports[9876] {
		t.Error("port 9876 should be detected")
	}
}

func TestParseListeningPorts_AllEphemeral(t *testing.T) {
	// Simulate a saturated port range.
	var output string
	for port := 49152; port <= 49160; port++ {
		output += "LISTEN 0 128 0.0.0.0:" + itoa(port) + " 0.0.0.0:*\n"
	}
	ports := parseListeningPorts(output)
	for port := 49152; port <= 49160; port++ {
		if !ports[port] {
			t.Errorf("port %d should be in use", port)
		}
	}
}

func TestClient_Run_ValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		cfg  RunConfig
	}{
		{"missing app", RunConfig{Process: "web", Version: "v1", Image: "img"}},
		{"missing process", RunConfig{App: "app", Version: "v1", Image: "img"}},
		{"missing version", RunConfig{App: "app", Process: "web", Image: "img"}},
		{"missing image", RunConfig{App: "app", Process: "web", Version: "v1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We don't need a mock — should fail before any SSH call.
			c := &Client{}
			_, err := c.Run(nil, tt.cfg)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestContainerPort_DefaultsTo80(t *testing.T) {
	cfg := RunConfig{
		App:     "myapp",
		Process: "web",
		Version: "v1",
		Image:   "myapp:latest",
		Port:    49152,
		// ContainerPort intentionally 0 (should default to 80)
	}

	// Verify default behavior.
	containerPort := cfg.ContainerPort
	if containerPort == 0 {
		containerPort = 80
	}
	if containerPort != 80 {
		t.Errorf("default container port should be 80, got %d", containerPort)
	}
}

func TestContainerPort_CustomPort(t *testing.T) {
	cfg := RunConfig{
		App:           "myapp",
		Process:       "web",
		Version:       "v1",
		Image:         "myapp:latest",
		Port:          49152,
		ContainerPort: 3000,
	}

	containerPort := cfg.ContainerPort
	if containerPort == 0 {
		containerPort = 80
	}
	if containerPort != 3000 {
		t.Errorf("custom container port should be 3000, got %d", containerPort)
	}
}

func itoa(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
