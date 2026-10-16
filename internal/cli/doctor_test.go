package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/testutil"
)

func writeExe(t *testing.T, dir, tool string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, platform.ExeName(tool)), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCheckShimShadowing_ShimWins(t *testing.T) {
	shimDir := t.TempDir()
	writeExe(t, shimDir, "node")
	t.Setenv("PATH", shimDir)

	if got := checkShimShadowing(shimDir, false); got != 0 {
		t.Errorf("shim is first on PATH, want 0 shadowed, got %d", got)
	}
}

func TestCheckShimShadowing_OtherInstallShadows(t *testing.T) {
	shimDir := t.TempDir()
	otherDir := t.TempDir()
	writeExe(t, shimDir, "node")
	writeExe(t, otherDir, "node") // earlier in PATH → wins
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+shimDir)

	if got := checkShimShadowing(shimDir, false); got != 1 {
		t.Errorf("node shadowed by earlier install, want 1, got %d", got)
	}
}

func TestCheckShimShadowing_SkipsWhenBinDirNotOnPath(t *testing.T) {
	shimDir := t.TempDir()
	otherDir := t.TempDir()
	writeExe(t, otherDir, "node")
	// checkPath owns the shimDir-not-on-PATH diagnosis, so the shadowing check stays silent.
	t.Setenv("PATH", otherDir)

	if got := checkShimShadowing(shimDir, false); got != 0 {
		t.Errorf("bin dir not on PATH, want 0 (skip), got %d", got)
	}
}

func TestCheckShimShadowing_NoToolsOnPath(t *testing.T) {
	shimDir := t.TempDir() // on PATH but contains no tool binaries
	t.Setenv("PATH", shimDir)

	if got := checkShimShadowing(shimDir, false); got != 0 {
		t.Errorf("no managed tools on PATH, want 0, got %d", got)
	}
}

func TestCheckShimShadowing_FixAppendsGuardToZshrc(t *testing.T) {
	testutil.SkipOnWindows(t, "shell rc files")
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")

	shimDir := filepath.Join(home, ".driftr", "bin")
	otherDir := t.TempDir()
	writeExe(t, shimDir, "node")
	writeExe(t, otherDir, "node") // shadows the shim
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+shimDir)

	// fix=true resolves the shadow by writing a precedence guard → 0 issues.
	if got := checkShimShadowing(shimDir, true); got != 0 {
		t.Errorf("with --fix the guard should resolve shadowing, got %d issues", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("expected guard written to .zshrc: %v", err)
	}
	if !strings.Contains(string(data), shimDir) || !strings.Contains(string(data), "precedence guard") {
		t.Errorf(".zshrc missing precedence guard: %q", data)
	}
}
