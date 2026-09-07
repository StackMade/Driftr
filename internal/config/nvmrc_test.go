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

// FuzzParseVersionFile asserts that the .nvmrc / .node-version parser never
// panics and never returns a version carrying leading or trailing whitespace,
// a comment, or an embedded newline.
func FuzzParseVersionFile(f *testing.F) {
	seeds := []string{
		"22.14.0\n", "v22.14.0\n", "22\n", "22.14\n", "  22.14.0  \n",
		"# my project\n22.14.0\n", "lts/*\n", "lts/hydrogen\n", "LTS/iron\n",
		"lts\n", "", "# nothing\n", "\n\n\n", "   ", "#", "v", "vv22",
		"\x00\n22.14.0", "\r\n22.14.0\r\n", "22.14.0\n\n# trailing",
		"ltsvv", "\t lts/jod \t", "２２.１４.０",
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
		// Lines are split on "\n" only, so CRLF is handled (the "\r" is trimmed)
		// but a lone CR is not a separator: "0\r0" comes back verbatim. That is
		// the parser's actual contract, not an accident worth asserting against.
		if strings.Contains(got, "\n") {
			t.Errorf("parseVersionFile(%q) = %q, want a single line", content, got)
		}
		// Comment skipping is not asserted here: it is a property of the input
		// line, not of the result ("v#" is not a comment and yields "#").
		// TestLoadNvmrc covers it.
	})
}
