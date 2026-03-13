package build

import (
	"testing"
)

func TestCountSharedLayers(t *testing.T) {
	tests := []struct {
		a, b []string
		want int
	}{
		{
			[]string{"sha256:aaa", "sha256:bbb", "sha256:ccc"},
			[]string{"sha256:aaa", "sha256:bbb", "sha256:ddd"},
			2,
		},
		{
			[]string{"sha256:aaa", "sha256:bbb"},
			[]string{"sha256:aaa", "sha256:bbb"},
			2,
		},
		{
			[]string{"sha256:aaa"},
			[]string{"sha256:bbb"},
			0,
		},
		{nil, nil, 0},
		{
			[]string{"sha256:aaa", "sha256:bbb", "sha256:ccc"},
			[]string{"sha256:aaa"},
			1,
		},
	}

	for _, tt := range tests {
		got := countSharedLayers(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("countSharedLayers(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{524288000, "500.0 MB"},
	}

	for _, tt := range tests {
		got := humanSize(tt.bytes)
		if got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
