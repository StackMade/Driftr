package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNvmrc(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"exact version", "22.14.0\n", "22.14.0"},
		{"with v prefix", "v22.14.0\n", "22.14.0"},
		{"partial major", "22\n", "22"},
		{"partial major minor", "22.14\n", "22.14"},
		{"with whitespace", "  22.14.0  \n", "22.14.0"},
		{"comment then version", "# my project\n22.14.0\n", "22.14.0"},
		{"lts star", "lts/*\n", "lts/*"},
		{"lts name", "lts/hydrogen\n", "lts/hydrogen"},
		{"LTS uppercase", "LTS/iron\n", "LTS/iron"},
		{"bare lts", "lts\n", "lts"},
		{"empty file", "", ""},
		{"only comments", "# nothing\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := LoadNvmrc(dir)
			if err != nil {
				t.Fatalf("LoadNvmrc() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadNvmrc() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadNvmrc_MissingFile(t *testing.T) {
	got, err := LoadNvmrc(t.TempDir())
	if err != nil {
		t.Fatalf("LoadNvmrc() error: %v", err)
	}
	if got != "" {
		t.Errorf("LoadNvmrc() = %q, want empty string", got)
	}
}
