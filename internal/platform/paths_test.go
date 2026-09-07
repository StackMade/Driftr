package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPaths_LayoutUnderHome pins the on-disk storage layout: every path helper
// must stay inside ~/.driftr and keep the directory names the shims, docs, and
// installer all assume.
func TestPaths_LayoutUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".driftr")

	tests := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"DriftrHome", DriftrHome, root},
		{"BinDir", BinDir, filepath.Join(root, "bin")},
		{"ToolsDir", ToolsDir, filepath.Join(root, "tools")},
		{"CacheDir", CacheDir, filepath.Join(root, "cache")},
		{"StoresDir", StoresDir, filepath.Join(root, "stores")},
		{"PnpmStoreDir", PnpmStoreDir, filepath.Join(root, "stores", "pnpm")},
		{"GlobalConfigPath", GlobalConfigPath, filepath.Join(root, "config", "config.toml")},
		{"NodeVersionDir", func() (string, error) { return NodeVersionDir("22.14.0") },
			filepath.Join(root, "tools", "node", "22.14.0")},
		{"ToolVersionDir", func() (string, error) { return ToolVersionDir("bun", "1.2.3") },
			filepath.Join(root, "tools", "bun", "1.2.3")},
		{"NodeBinary", func() (string, error) { return NodeBinary("22.14.0") },
			filepath.Join(root, "tools", "node", "22.14.0", "bin", "node")},
		{"NpmBinary", func() (string, error) { return NpmBinary("22.14.0") },
			filepath.Join(root, "tools", "node", "22.14.0", "bin", "npm")},
		{"NpxBinary", func() (string, error) { return NpxBinary("22.14.0") },
			filepath.Join(root, "tools", "node", "22.14.0", "bin", "npx")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if err != nil {
				t.Fatalf("%s() error: %v", tt.name, err)
			}
			if got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// Without a home directory there is nowhere to store anything; every helper
// must report that rather than falling back to a relative path.
func TestPaths_NoHomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home directory is not derived from HOME on windows")
	}
	t.Setenv("HOME", "")

	tests := map[string]func() (string, error){
		"DriftrHome":       DriftrHome,
		"BinDir":           BinDir,
		"ToolsDir":         ToolsDir,
		"CacheDir":         CacheDir,
		"StoresDir":        StoresDir,
		"PnpmStoreDir":     PnpmStoreDir,
		"GlobalConfigPath": GlobalConfigPath,
		"NodeBinary":       func() (string, error) { return NodeBinary("22.14.0") },
		"ToolVersionDir":   func() (string, error) { return ToolVersionDir("node", "22.14.0") },
	}

	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := fn()
			if err == nil {
				t.Fatalf("%s() = %q, want an error when HOME is unset", name, got)
			}
			if got != "" {
				t.Errorf("%s() returned %q alongside an error", name, got)
			}
		})
	}

	if err := EnsureDirs(); err == nil {
		t.Error("EnsureDirs() = nil, want an error when HOME is unset")
	}
	if _, err := ListToolVersions("node"); err == nil {
		t.Error("ListToolVersions() = nil error, want an error when HOME is unset")
	}
}

func TestEnsureDirs_CreatesNodeLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, dir := range []string{"bin", "config", "cache", filepath.Join("tools", "node")} {
		path := filepath.Join(home, ".driftr", dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", path)
		}
	}
}

func TestEnsureToolDirs_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := EnsureToolDirs("bun"); err != nil {
		t.Fatalf("first EnsureToolDirs() error: %v", err)
	}
	// A file dropped in afterwards must survive a second call.
	marker := filepath.Join(home, ".driftr", "tools", "bun", "1.2.3")
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureToolDirs("bun"); err != nil {
		t.Fatalf("second EnsureToolDirs() error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("existing install was disturbed: %v", err)
	}
}

// A file where a directory belongs cannot be repaired silently — the error has
// to name the path so the user can remove it.
func TestEnsureToolDirs_FileInTheWay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".driftr"), 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(home, ".driftr", "bin")
	if err := os.WriteFile(binPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := EnsureToolDirs("node")
	if err == nil {
		t.Fatal("expected an error when a file occupies the bin directory")
	}
	if !strings.Contains(err.Error(), binPath) {
		t.Errorf("error does not name the offending path %q: %v", binPath, err)
	}
}

// Arch and OS name the current platform the way nodejs.org does — the download
// URL is built from them, so a wrong value means every install 404s.
func TestArchAndOS_NodeDistNaming(t *testing.T) {
	archNames := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}
	wantArch, ok := archNames[runtime.GOARCH]
	if !ok {
		wantArch = runtime.GOARCH
	}
	if got := Arch(); got != wantArch {
		t.Errorf("Arch() = %q, want %q for GOARCH %q", got, wantArch, runtime.GOARCH)
	}

	if got := OS(); got != runtime.GOOS {
		t.Errorf("OS() = %q, want %q", got, runtime.GOOS)
	}

	if got := ArchiveExt(); got != "tar.gz" {
		t.Errorf("ArchiveExt() = %q, want %q", got, "tar.gz")
	}
}

func TestListToolVersions_SkipsExtractionWorkDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	nodeDir := filepath.Join(home, ".driftr", "tools", "node")
	for _, name := range []string{"22.14.0", "22.15.0.tmp-4711"} {
		if err := os.MkdirAll(filepath.Join(nodeDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := ListToolVersions("node")
	if err != nil {
		t.Fatalf("ListToolVersions() error: %v", err)
	}
	if len(versions) != 1 || versions[0] != "22.14.0" {
		t.Errorf("ListToolVersions() = %v, want [22.14.0]", versions)
	}
}
