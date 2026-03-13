package ssh

import (
	"context"
	"strings"
	"testing"
)

func TestMockExecutor_Run(t *testing.T) {
	mock := NewMockExecutor("1.2.3.4",
		MockCommand{Match: "docker ps", Output: "CONTAINER ID  IMAGE  STATUS"},
		MockCommand{Match: "echo", Output: "hello"},
	)

	ctx := context.Background()

	// Match first command
	out, err := mock.Run(ctx, "docker ps -a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "CONTAINER ID  IMAGE  STATUS" {
		t.Fatalf("unexpected output: %q", out)
	}

	// Match second command
	out, err = mock.Run(ctx, "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello" {
		t.Fatalf("unexpected output: %q", out)
	}

	// Unmatched command
	_, err = mock.Run(ctx, "unknown command")
	if err == nil {
		t.Fatal("expected error for unmatched command")
	}

	// Verify call recording
	if len(mock.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(mock.Calls))
	}
}

func TestMockExecutor_Upload(t *testing.T) {
	mock := NewMockExecutor("1.2.3.4")
	ctx := context.Background()

	content := strings.NewReader("SECRET=value123")
	err := mock.Upload(ctx, content, "/deployments/myapp/.env", "0600")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file content was recorded
	data, ok := mock.Files["/deployments/myapp/.env"]
	if !ok {
		t.Fatal("expected file to be recorded")
	}
	if string(data) != "SECRET=value123" {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	// Verify call was recorded
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Calls))
	}
	if mock.Calls[0] != "UPLOAD:/deployments/myapp/.env (mode 0600)" {
		t.Fatalf("unexpected call: %s", mock.Calls[0])
	}
}

func TestMockExecutor_Host(t *testing.T) {
	mock := NewMockExecutor("10.0.0.1")
	if mock.Host() != "10.0.0.1" {
		t.Fatalf("expected host 10.0.0.1, got %s", mock.Host())
	}
}
