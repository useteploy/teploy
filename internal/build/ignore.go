package build

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultIgnore contains the default patterns excluded from rsync.
var DefaultIgnore = []string{
	"node_modules",
	".env",
	".git",
	".teployignore",
}

// LoadIgnore reads .teployignore from the given directory.
// Returns the parsed patterns, or DefaultIgnore if the file doesn't exist.
func LoadIgnore(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, ".teployignore"))
	if err != nil {
		return DefaultIgnore
	}

	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	if len(patterns) == 0 {
		return DefaultIgnore
	}
	return patterns
}
