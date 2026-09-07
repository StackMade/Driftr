package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestToolBinary_KnownTools(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		tool       string
		version    string
		wantSuffix string
	}{
		{"node", "22.14.0", "tools/node/22.14.0/bin/node"},
		{"npm", "22.14.0", "tools/node/22.14.0/bin/npm"},
		{"npx", "22.14.0", "tools/node/22.14.0/bin/npx"},
		{"pnpm", "9.15.0", "tools/pnpm/9.15.0/bin/pnpm.cjs"},
		{"pnpx", "9.15.0", "tools/pnpm/9.15.0/bin/pnpx.cjs"},
		{"yarn", "1.22.22", "tools/yarn/1.22.22/bin/yarn.js"},
		{"bun", "1.2.3", "tools/bun/1.2.3/bin/bun"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			path, err := ToolBinary(tt.tool, tt.version)
			if err != nil {
				t.Fatalf("ToolBinary(%q, %q) error: %v", tt.tool, tt.version, err)
			}
			if !pathEndsWith(path, tt.wantSuffix) {
				t.Errorf("ToolBinary(%q, %q) = %q, want suffix %q", tt.tool, tt.version, path, tt.wantSuffix)
			}
		})
	}
}

func TestToolBinary_UnknownTool(t *testing.T) {
	_, err := ToolBinary("unknown", "1.0.0")
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestLookupTool(t *testing.T) {
	tests := []struct {
		tool       string
		wantOk     bool
		wantNode   bool
		wantParent string
	}{
		{"node", true, false, "node"},
		{"npm", true, false, "node"},
		{"pnpm", true, true, "pnpm"},
		{"yarn", true, true, "yarn"},
		{"bun", true, false, "bun"},
		{"unknown", false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			entry, ok := LookupTool(tt.tool)
			if ok != tt.wantOk {
				t.Fatalf("LookupTool(%q) ok = %v, want %v", tt.tool, ok, tt.wantOk)
			}
			if !ok {
				return
			}
			if entry.NeedsNode != tt.wantNode {
				t.Errorf("LookupTool(%q).NeedsNode = %v, want %v", tt.tool, entry.NeedsNode, tt.wantNode)
			}
			if entry.Parent != tt.wantParent {
				t.Errorf("LookupTool(%q).Parent = %q, want %q", tt.tool, entry.Parent, tt.wantParent)
			}
		})
	}
}

func TestListToolVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create fake installed versions.
	nodeDir := filepath.Join(home, ".driftr", "tools", "node")
	for _, v := range []string{"20.11.0", "22.14.0", "24.0.0"} {
		if err := os.MkdirAll(filepath.Join(nodeDir, v), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	versions, err := ListToolVersions("node")
	if err != nil {
		t.Fatalf("ListToolVersions() error: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("ListToolVersions() returned %d versions, want 3", len(versions))
	}
}

func TestListToolVersions_NoDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	versions, err := ListToolVersions("node")
	if err != nil {
		t.Fatalf("ListToolVersions() error: %v", err)
	}
	if versions != nil {
		t.Errorf("expected nil for non-existent dir, got %v", versions)
	}
}

func TestListToolVersions_IgnoresFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	nodeDir := filepath.Join(home, ".driftr", "tools", "node")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a version dir and a stray file.
	os.MkdirAll(filepath.Join(nodeDir, "22.14.0"), 0o755)
	os.WriteFile(filepath.Join(nodeDir, ".DS_Store"), []byte{}, 0o644)

	versions, err := ListToolVersions("node")
	if err != nil {
		t.Fatalf("ListToolVersions() error: %v", err)
	}
	if len(versions) != 1 || versions[0] != "22.14.0" {
		t.Errorf("ListToolVersions() = %v, want [22.14.0]", versions)
	}
}

func TestEnsureToolDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := EnsureToolDirs("node", "pnpm"); err != nil {
		t.Fatalf("EnsureToolDirs() error: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(home, ".driftr", "bin"),
		filepath.Join(home, ".driftr", "config"),
		filepath.Join(home, ".driftr", "cache"),
		filepath.Join(home, ".driftr", "tools", "node"),
		filepath.Join(home, ".driftr", "tools", "pnpm"),
	} {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected directory %s to exist", dir)
		}
	}
}

func pathEndsWith(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
