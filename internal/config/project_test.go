package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackmade/driftr/v2/internal/platform"
	"github.com/stackmade/driftr/v2/internal/testutil"
)

func writeProjectConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test project config: %v", err)
	}
}

func TestLoadProject_NoFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject() unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when no file exists, got %+v", cfg)
	}
}

func TestLoadProject_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `[tools]
node = "22.14.0"
`)

	cfg, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if got := cfg.Tools.GetTool("node"); got != "22.14.0" {
		t.Errorf("Tools.GetTool(node) = %q, want %q", got, "22.14.0")
	}
}

func TestLoadProject_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `this is not valid toml {{{`)

	_, err := LoadProject(dir)
	if err == nil {
		t.Error("expected error for invalid TOML, got nil")
	}
}

func TestSaveAndLoadProject(t *testing.T) {
	dir := t.TempDir()

	cfg := &ProjectConfig{}
	cfg.Tools.SetTool("node", "24.0.1")

	if err := SaveProject(dir, cfg); err != nil {
		t.Fatalf("SaveProject() error: %v", err)
	}

	loaded, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config after save")
	}
	if got := loaded.Tools.GetTool("node"); got != "24.0.1" {
		t.Errorf("Tools.GetTool(node) = %q, want %q", got, "24.0.1")
	}
}

func TestSaveAndLoadProject_MultiTool(t *testing.T) {
	dir := t.TempDir()

	cfg := &ProjectConfig{}
	cfg.Tools.SetTool("node", "24.0.1")
	cfg.Tools.SetTool("pnpm", "9.15.0")

	if err := SaveProject(dir, cfg); err != nil {
		t.Fatalf("SaveProject() error: %v", err)
	}

	loaded, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config after save")
	}
	if got := loaded.Tools.GetTool("node"); got != "24.0.1" {
		t.Errorf("Tools.GetTool(node) = %q, want %q", got, "24.0.1")
	}
	if got := loaded.Tools.GetTool("pnpm"); got != "9.15.0" {
		t.Errorf("Tools.GetTool(pnpm) = %q, want %q", got, "9.15.0")
	}
}

func TestLoadProject_EmptyNode(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `[tools]
`)

	cfg, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for existing file with empty tools")
	}
	if got := cfg.Tools.GetTool("node"); got != "" {
		t.Errorf("Tools.GetTool(node) = %q, want empty", got)
	}
}

// Writing in place let a concurrent shim read see a half-written file; a rename makes a new inode.
func TestSaves_ReplaceTheFileAtomically(t *testing.T) {
	testutil.SkipOnWindows(t, "os.SameFile reopens the path, so a replaced file looks the same")
	testutil.SetHome(t, t.TempDir())
	dir := t.TempDir()
	writeProjectConfig(t, dir, "[tools]\nnode = \"20.0.0\"\n")
	writePackageJSON(t, dir, `{"name": "x"}`)
	if err := SaveGlobal(&GlobalConfig{}); err != nil {
		t.Fatal(err)
	}
	globalPath, err := platform.GlobalConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		path string
		save func() error
	}{
		{filepath.Join(dir, ProjectConfigFile), func() error {
			return SaveProject(dir, &ProjectConfig{Tools: ToolsConfig{"node": "22.0.0"}})
		}},
		{filepath.Join(dir, "package.json"), func() error { return SavePackageJSONTool(dir, "node", "22.0.0") }},
		{filepath.Join(dir, "package.json"), func() error { return RemoveDriftrFromPackageJSON(dir) }},
		{globalPath, func() error { return SaveGlobal(&GlobalConfig{Default: DefaultConfig{Node: "22.0.0"}}) }},
	} {
		before, err := os.Stat(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := tt.save(); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(before, after) {
			t.Errorf("%s was rewritten in place", tt.path)
		}
	}
}

func TestSavePackageJSONTool_KeepsModeAndSymlink(t *testing.T) {
	testutil.SkipOnWindows(t, "permission bits and symlinks")
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(target, []byte(`{"name": "x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "package.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := SavePackageJSONTool(dir, "node", "22.0.0"); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink replaced by a file: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	pkg, err := LoadPackageJSON(dir)
	if err != nil || pkg == nil || pkg.Driftr.GetTool("node") != "22.0.0" {
		t.Errorf("pin not written through the symlink: %+v, %v", pkg, err)
	}
}
