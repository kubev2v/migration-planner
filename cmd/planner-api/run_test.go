package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAgentVersionFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "version with trailing newline",
			content: "main-release-0.32-2d1b0db\n",
			want:    "main-release-0.32-2d1b0db",
		},
		{
			name:    "version without newline",
			content: "v0.32",
			want:    "v0.32",
		},
		{
			name:    "empty file",
			content: "",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "agent-version")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			if got := readAgentVersionFile(path); got != tt.want {
				t.Errorf("readAgentVersionFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadAgentVersionFileMissing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "agent-version")
	if got := readAgentVersionFile(path); got != "" {
		t.Errorf("readAgentVersionFile() = %q, want empty string for missing file", got)
	}
}
