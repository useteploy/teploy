package autodeploy

import (
	"strings"
	"testing"
)

func TestGenerateListener_SecretWithSingleQuote(t *testing.T) {
	// A secret containing a single quote should be properly escaped
	// to prevent shell injection.
	listener := generateListener("myapp", "secret'with'quotes", "/deployments/myapp/autodeploy.sh")

	// The secret should NOT appear as an unescaped single-quoted string
	// that would break the bash syntax.
	if strings.Contains(listener, "hmac 'secret'with'quotes'") {
		t.Error("single quotes in secret must be escaped")
	}

	// The escaped version should be present.
	if !strings.Contains(listener, "secret") {
		t.Error("listener should still contain the secret reference")
	}

	// Should still have the HMAC validation block.
	if !strings.Contains(listener, "openssl dgst -sha256") {
		t.Error("listener should contain HMAC validation")
	}
}

func TestGenerateListener_EmptySecret(t *testing.T) {
	listener := generateListener("myapp", "", "/deployments/myapp/autodeploy.sh")

	// Should NOT contain any HMAC validation when secret is empty.
	if strings.Contains(listener, "openssl") {
		t.Error("listener should not validate secrets when none provided")
	}
}

func TestGenerateListener_SecretWithShellMetachars(t *testing.T) {
	// Test various shell metacharacters that could be dangerous.
	dangerous := []string{
		"secret$(whoami)",
		"secret`id`",
		"secret;rm -rf /",
		"secret|cat /etc/passwd",
		"secret\ninjected",
	}

	for _, secret := range dangerous {
		listener := generateListener("myapp", secret, "/deployments/myapp/autodeploy.sh")
		// Should still produce valid output (not crash).
		if listener == "" {
			t.Errorf("generateListener returned empty for secret %q", secret)
		}
		// Should contain the HMAC block.
		if !strings.Contains(listener, "openssl dgst") {
			t.Errorf("listener missing HMAC block for secret %q", secret)
		}
	}
}

func TestGenerateScript_SpecialBranch(t *testing.T) {
	// Branch names with special chars should be safely embedded.
	script := generateScript(Config{
		App:    "myapp",
		Branch: "feature/my-branch",
	})

	if !strings.Contains(script, `BRANCH="feature/my-branch"`) {
		t.Error("script should contain the branch name with slash")
	}
}
