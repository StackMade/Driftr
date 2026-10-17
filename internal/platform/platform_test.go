package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackmade/driftr/v2/internal/testutil"
)

func TestToolBinary_KnownTools(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	tests := []struct {
		tool       string
		version    string
		wantSuffix string
	}{
		{"node", "22.14.0", "tools/node/22.14.0/bin/" + ExeName("node")},
		{"npm", "10.9.2", "tools/npm/10.9.2/bin/npm-cli.js"},
		{"npx", "10.9.2", "tools/npm/10.9.2/bin/npx-cli.js"},
		{"pnpm", "9.15.0", "tools/pnpm/9.15.0/bin/pnpm.cjs"},
		{"pnpx", "9.15.0", "tools/pnpm/9.15.0/bin/pnpx.cjs"},
		{"yarn", "1.22.22", "tools/yarn/1.22.22/bin/yarn.js"},
		{"bun", "1.2.3", "tools/bun/1.2.3/bin/" + ExeName("bun")},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			path, err := ToolBinary(tt.tool, tt.version)
			if err != nil {
				t.Fatalf("ToolBinary(%q, %q) error: %v", tt.tool, tt.version, err)
			}
			if !pathEndsWith(path, filepath.FromSlash(tt.wantSuffix)) {
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
		{"npm", true, true, "npm"},
		{"npx", true, true, "npm"},
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
	testutil.SetHome(t, home)

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
	testutil.SetHome(t, t.TempDir())

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
	testutil.SetHome(t, home)

	nodeDir := filepath.Join(home, ".driftr", "tools", "node")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
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
	testutil.SetHome(t, home)

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

func TestNodePackageBins(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	for v, names := range map[string][]string{
		"20.0.0": {"node", "npm", "tsc", "corepack", "bad;name", ".hidden"},
		"22.0.0": {"node", "tsc"},
	} {
		dir, _ := ToolVersionDir("node", v)
		for _, n := range names {
			if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "bin", PackageBinFile(n)), nil, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}

	bins, err := NodePackageBins()
	if err != nil {
		t.Fatal(err)
	}
	if len(bins) != 2 || len(bins["tsc"]) != 2 || len(bins["corepack"]) != 1 {
		t.Errorf("want tsc under both versions and corepack under one, got %v", bins)
	}
}

func TestIsSafeBinName(t *testing.T) {
	for name, want := range map[string]bool{
		"tsc": true, "vue-cli-service": true, "esbuild_1.2+x": true, "a@b": true,
		"": false, ".bin": false, "-rf": false, "a b": false, "a;b": false, "$(x)": false, "a/b": false,
	} {
		if got := IsSafeBinName(name); got != want {
			t.Errorf("IsSafeBinName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFindOutsideDriftr(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	bin, _ := BinDir()
	writeExec := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ExeName(p), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec(filepath.Join(bin, "nest")) // driftr's own shim

	// A symlinked shim dir and a symlink to the shim file must never count as "elsewhere".
	aliasDir := filepath.Join(t.TempDir(), "shims")
	if err := os.Symlink(bin, aliasDir); err != nil {
		t.Fatal(err)
	}
	fileLinks := t.TempDir()
	if err := os.Symlink(filepath.Join(bin, ExeName("nest")), filepath.Join(fileLinks, ExeName("nest"))); err != nil {
		t.Fatal(err)
	}
	sep := string(os.PathListSeparator)

	t.Setenv("PATH", bin+sep+aliasDir+sep+fileLinks)
	if got := FindOutsideDriftr("nest"); got != "" {
		t.Errorf("an alias of driftr's own shim counted as another install: %s", got)
	}

	// A relative entry is searched from the current directory, as in the shell.
	cwd := t.TempDir()
	writeExec(filepath.Join(cwd, "tools", "nest"))
	t.Chdir(cwd)
	t.Setenv("PATH", "")
	writeExec(filepath.Join(cwd, "nest"))
	if got := FindOutsideDriftr("nest"); filepath.Base(got) != ExeName("nest") || filepath.Base(filepath.Dir(got)) == "tools" {
		t.Errorf("empty PATH should search the current directory, got %q", got)
	}

	t.Setenv("PATH", aliasDir+sep+"tools")
	if got, want := FindOutsideDriftr("nest"), filepath.Join(cwd, "tools", ExeName("nest")); got != want {
		// macOS temp dirs live behind the /var -> /private/var symlink.
		if real, _ := filepath.EvalSymlinks(got); real != want {
			if rw, _ := filepath.EvalSymlinks(want); real != rw {
				t.Errorf("relative PATH entry: got %q, want %q", got, want)
			}
		}
	}
}
