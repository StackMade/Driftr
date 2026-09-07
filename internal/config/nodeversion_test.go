package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNodeVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"exact version", "22.14.0\n", "22.14.0"},
		{"with v prefix", "v22.14.0\n", "22.14.0"},
		{"partial major", "22\n", "22"},
		{"with whitespace", "  22.14.0  \n", "22.14.0"},
		{"empty file", "", ""},
		{"multi-line", "22.14.0\n18.0.0\n", "22.14.0"},
		{"comment then version", "# pinned node\n22.14.0\n", "22.14.0"},
		{"only comment", "# nothing\n", ""},
		{"lts star", "lts/*\n", "lts/*"},
		{"lts named", "lts/hydrogen\n", "lts/hydrogen"},
		{"CRLF", "22.14.0\r\n", "22.14.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".node-version"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := LoadNodeVersion(dir)
			if err != nil {
				t.Fatalf("LoadNodeVersion() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadNodeVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadNodeVersion_MissingFile(t *testing.T) {
	got, err := LoadNodeVersion(t.TempDir())
	if err != nil {
		t.Fatalf("LoadNodeVersion() error: %v", err)
	}
	if got != "" {
		t.Errorf("LoadNodeVersion() = %q, want empty string", got)
	}
}
