package config

import (
	"os"
	"path/filepath"
	"strings"
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
		{"trailing comment", "22.14.0 # ci\n", "22.14.0"},
		{"trailing comment without space", "v22.14.0#ci\n", "22.14.0"},
		{"byte order mark", "\ufeff22.14.0\n", "22.14.0"},
		{"byte order mark before comment", "\ufeff# pinned\n22.14.0\n", "22.14.0"},
		{"lts with trailing comment", "lts/iron # ci\n", "lts/iron"},
		{"nvm node alias", "node\n", "latest"},
		{"nvm stable alias", "Stable\n", "latest"},
		{"nvm system alias", "system\n", ""},
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

// The parser must never return a version with whitespace, a comment, or an embedded newline.
func FuzzParseVersionFile(f *testing.F) {
	seeds := []string{
		"22.14.0\n", "v22.14.0\n", "22\n", "22.14\n", "  22.14.0  \n",
		"# my project\n22.14.0\n", "lts/*\n", "lts/hydrogen\n", "LTS/iron\n",
		"lts\n", "", "# nothing\n", "\n\n\n", "   ", "#", "v", "vv22",
		"\x00\n22.14.0", "\r\n22.14.0\r\n", "22.14.0\n\n# trailing",
		"ltsvv", "\t lts/jod \t", "22.14.0 # ci", "\ufeffv#", "２２.１４.０",
		strings.Repeat("#\n", 500) + "22.14.0",
		strings.Repeat("2", 4000),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		got, err := parseVersionFile(content)
		if err != nil {
			t.Fatalf("parseVersionFile(%q) unexpected error: %v", content, err)
		}
		if got == "" {
			return
		}
		if strings.TrimSpace(got) != got {
			t.Errorf("parseVersionFile(%q) = %q, want no surrounding whitespace", content, got)
		}
		// A lone CR is not a line separator, so "0\r0" comes back verbatim by contract.
		if strings.Contains(got, "\n") {
			t.Errorf("parseVersionFile(%q) = %q, want a single line", content, got)
		}
		if strings.Contains(got, "#") {
			t.Errorf("parseVersionFile(%q) = %q, want no comment", content, got)
		}
	})
}

func TestSaveVersionFile(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"bare with newline", "22.1.0\n", "24.2.0\n"},
		{"leading v kept", "v22.1.0\n", "v24.2.0\n"},
		{"no trailing newline stays without", "22.1.0", "24.2.0"},
		{"comments and blank lines kept", "# pinned for CI\n\n  v22.1.0  \n", "# pinned for CI\n\n  v24.2.0  \n"},
		{"CRLF kept", "v22.1.0\r\n", "v24.2.0\r\n"},
		{"trailing comment kept", "22.1.0 # ci\n", "24.2.0 # ci\n"},
		{"byte order mark kept", "\ufeffv22.1.0\n", "\ufeffv24.2.0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".nvmrc")
			if err := os.WriteFile(path, []byte(tt.in), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := SaveVersionFile(path, "24.2.0"); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// .node-version is read like .nvmrc: both go through parseVersionFile.
func TestLoadNodeVersion_CommentAndBOM(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".node-version"), []byte("\ufeff22.14.0 # ci\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadNodeVersion(dir)
	if err != nil || got != "22.14.0" {
		t.Errorf("LoadNodeVersion() = %q, %v, want 22.14.0", got, err)
	}
}

func TestSaveVersionFile_NoVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".node-version")
	if err := os.WriteFile(path, []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveVersionFile(path, "24.2.0"); err == nil {
		t.Error("expected an error for a file with no version line")
	}
}
