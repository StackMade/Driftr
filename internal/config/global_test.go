package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/testutil"
)

func writeGlobal(t *testing.T, content string) {
	t.Helper()
	testutil.SetHome(t, t.TempDir())
	path, err := platform.GlobalConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGlobal_MissingFile(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Default.GetTool("node"); got != "" {
		t.Errorf("default node = %q, want empty", got)
	}
}

func TestGlobal_RoundTrip(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	cfg := &GlobalConfig{AutoInstall: true}
	cfg.Default.SetTool("node", "20.0.0")
	cfg.Default.SetTool("pnpm", "9.15.0")
	if err := SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Default.SetTool("node", "22.14.0")
	if err := SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.AutoInstall {
		t.Error("auto_install lost in round trip")
	}
	if loaded.Default.Node != "22.14.0" {
		t.Errorf("legacy node field = %q, want it kept in sync for older driftr builds", loaded.Default.Node)
	}
	for tool, want := range map[string]string{"node": "22.14.0", "pnpm": "9.15.0", "yarn": ""} {
		if got := loaded.Default.GetTool(tool); got != want {
			t.Errorf("GetTool(%q) = %q, want %q", tool, got, want)
		}
	}
}

func TestLoadGlobal_FileFromOlderDriftr(t *testing.T) {
	writeGlobal(t, "[default]\nnode = \"20.11.0\"\n")

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Default.GetTool("node"); got != "20.11.0" {
		t.Errorf("GetTool(node) = %q, want the legacy field", got)
	}
	if got := cfg.Default.GetTool("pnpm"); got != "" {
		t.Errorf("GetTool(pnpm) = %q, want empty", got)
	}
}

func TestLoadGlobal_ToolsTableBeatsLegacyField(t *testing.T) {
	writeGlobal(t, "[default]\nnode = \"18.0.0\"\n\n[default.tools]\nnode = \"22.14.0\"\n")

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Default.GetTool("node"); got != "22.14.0" {
		t.Errorf("GetTool(node) = %q, want 22.14.0", got)
	}
}

func TestLoadGlobal_HandEditedSyntaxError(t *testing.T) {
	writeGlobal(t, "[default]\nnode = 22.14.0\"\n")

	if _, err := LoadGlobal(); err == nil || !strings.Contains(err.Error(), "failed to parse global config") {
		t.Errorf("LoadGlobal() error = %v, want a parse error", err)
	}
}
