package template

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultRepoURL points to the community template repository.
	DefaultRepoURL = "https://raw.githubusercontent.com/teploy/templates/main"
	indexFile      = "index.json"
)

// Info describes a template in the index.
type Info struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Accessories []string `json:"accessories,omitempty"`
	Variables   []string `json:"variables,omitempty"`
}

// Registry fetches and manages templates from the community repo.
type Registry struct {
	baseURL string
	client  *http.Client
}

// NewRegistry creates a template registry with the default repo URL.
func NewRegistry() *Registry {
	return &Registry{
		baseURL: DefaultRepoURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// SetBaseURL overrides the template repository URL (useful for testing).
func (r *Registry) SetBaseURL(url string) {
	r.baseURL = url
}

// List fetches the template index and returns all available templates.
func (r *Registry) List(ctx context.Context) ([]Info, error) {
	url := r.baseURL + "/" + indexFile
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching template index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("template index returned %d", resp.StatusCode)
	}

	var index []Info
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("parsing template index: %w", err)
	}
	return index, nil
}

// Fetch downloads a template and applies variable substitution.
func (r *Registry) Fetch(ctx context.Context, name string, vars map[string]string) (string, error) {
	url := r.baseURL + "/" + name + "/teploy.yml"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching template %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("template %q not found", name)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("template fetch returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading template: %w", err)
	}

	content := string(body)

	// Apply variable substitution.
	for k, v := range vars {
		content = strings.ReplaceAll(content, "{{"+k+"}}", v)
	}

	// Generate secrets.
	content = GenerateSecrets(content)

	return content, nil
}

// GenerateSecrets replaces "generate" env values with random 64-char hex strings.
func GenerateSecrets(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ": generate") || strings.HasSuffix(trimmed, ": \"generate\"") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			key := strings.TrimSuffix(strings.TrimSuffix(trimmed, ": generate"), ": \"generate\"")
			secret := RandomHex(32)
			lines = append(lines, fmt.Sprintf("%s%s: %s", indent, key, secret))
		} else {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// RandomHex generates a random hex string of n bytes (2n chars).
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
