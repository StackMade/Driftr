package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/platform"
)

// fakeNodeInstall creates a fake installed node version under $HOME/.driftr.
func fakeNodeInstall(t *testing.T, ver string) {
	t.Helper()
	bin, err := platform.NodeBinary(ver)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runCmd executes the root command with the given args.
func runCmd(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.Execute()
}

func TestDefaultCmd_SetsGlobalDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	if err := runCmd(t, "default", "node@22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Default.GetTool("node"); got != "22.14.0" {
		t.Errorf("global default = %q, want %q", got, "22.14.0")
	}
}

func TestDefaultCmd_PartialVersionResolves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	fakeNodeInstall(t, "22.9.0")

	if err := runCmd(t, "default", "node@22"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Default.GetTool("node"); got != "22.14.0" {
		t.Errorf("partial spec resolved to %q, want latest installed %q", got, "22.14.0")
	}
}

func TestDefaultCmd_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runCmd(t, "default", "node@22.14.0")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected not-installed error, got: %v", err)
	}
}

func TestDefaultCmd_InvalidVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runCmd(t, "default", "node@bogus.version.string")
	if err == nil || !strings.Contains(err.Error(), "invalid version") {
		t.Errorf("expected invalid-version error, got: %v", err)
	}
}

func TestPinCmd_ExistingTOMLFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".driftr.toml"), []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	if err := runCmd(t, "pin", "node@22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.LoadProject(proj)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Tools.GetTool("node"); got != "22.14.0" {
		t.Errorf("pinned version = %q, want %q", got, "22.14.0")
	}
}

func TestPinCmd_ExistingPackageJSONFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	proj := t.TempDir()
	pkg := `{"name": "myapp", "driftr": {"node": "20.0.0"}}`
	if err := os.WriteFile(filepath.Join(proj, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	if err := runCmd(t, "pin", "node@22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.LoadPackageJSON(proj)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("package.json driftr config not found after pin")
	}
	if got := loaded.Driftr.GetTool("node"); got != "22.14.0" {
		t.Errorf("pinned version = %q, want %q", got, "22.14.0")
	}
}

func TestPinCmd_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	err := runCmd(t, "pin", "node@22.14.0")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected not-installed error, got: %v", err)
	}
}

func TestWhichCmd_ProjectSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".driftr.toml"), []byte("[tools]\nnode = \"22.14.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	if err := runCmd(t, "which", "node"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWhichCmd_NothingConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	err := runCmd(t, "which", "node")
	if err == nil || !strings.Contains(err.Error(), "no node version configured") {
		t.Errorf("expected no-version-configured error, got: %v", err)
	}
}

func TestUninstallCmd_RemovesVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	if err := runCmd(t, "uninstall", "node@22.14.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dir, err := platform.ToolVersionDir("node", "22.14.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("version dir still exists after uninstall: %s", dir)
	}
}

func TestUninstallCmd_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	err := runCmd(t, "uninstall", "node@22.14.0")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected not-installed error, got: %v", err)
	}
}

func TestUninstallCmd_UnknownTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runCmd(t, "uninstall", "ruby@3.0.0")
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected unknown-tool error, got: %v", err)
	}
}

func TestUninstallCmd_MissingVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// "node@" parses to an empty version spec; bare "node" is treated as a
	// version for the default tool by parseToolVersion.
	err := runCmd(t, "uninstall", "node@")
	if err == nil || !strings.Contains(err.Error(), "version required") {
		t.Errorf("expected version-required error, got: %v", err)
	}
}

func TestListCmd_Local(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	fakeNodeInstall(t, "20.10.0")

	if err := runCmd(t, "list"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListCmd_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	if err := runCmd(t, "list"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetupCmd_ConfiguresPathInUniversalRcFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	if err := runCmd(t, "setup"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// zsh's universal rc file is .zshenv (sourced by every shell), not the
	// interactive-only .zshrc.
	zshenv := filepath.Join(home, ".zshenv")
	data, err := os.ReadFile(zshenv)
	if err != nil {
		t.Fatalf("expected PATH written to .zshenv: %v", err)
	}
	if !strings.Contains(string(data), filepath.Join(home, ".driftr", "bin")) {
		t.Errorf(".zshenv missing driftr bin PATH export: %q", data)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Errorf("setup must not create/use interactive-only .zshrc")
	}
}

func TestInstallCmd_TrailingAtErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// "pnpm@" is a malformed spec — an explicit "@" with no version. It must
	// error (offline, before any download) rather than installing latest.
	err := runCmd(t, "install", "pnpm@")
	if err == nil || !strings.Contains(err.Error(), "version required after '@'") {
		t.Errorf("expected version-required error for 'pnpm@', got: %v", err)
	}
}

func TestInstallCmd_NoArgsNoPins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// Nothing pinned anywhere up the tree: the command must say so, offline,
	// rather than exiting successfully having installed nothing.
	err := runCmd(t, "install")
	if err == nil || !strings.Contains(err.Error(), "no tool versions pinned") {
		t.Errorf("expected a no-pins error, got: %v", err)
	}
}
