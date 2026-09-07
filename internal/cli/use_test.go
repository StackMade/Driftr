package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// runCmdOut executes the root command and returns what it wrote to stdout.
func runCmdOut(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(io.Discard)
	err := root.Execute()
	return out.String(), err
}

func TestUseCmd_PosixSnippet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	fakeNodeInstall(t, "22.14.0")

	out, err := runCmdOut(t, "use", "node@22.14.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "export DRIFTR_NODE=22.14.0" {
		t.Errorf("snippet = %q, want %q", got, "export DRIFTR_NODE=22.14.0")
	}
}

func TestUseCmd_PartialVersionResolves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/bash")
	fakeNodeInstall(t, "22.9.0")
	fakeNodeInstall(t, "22.14.0")

	out, err := runCmdOut(t, "use", "node@22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "export DRIFTR_NODE=22.14.0" {
		t.Errorf("snippet = %q, want the newest installed 22.x", got)
	}
}

func TestUseCmd_FishSnippet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/usr/bin/fish")
	fakeNodeInstall(t, "22.14.0")

	out, err := runCmdOut(t, "use", "node@22.14.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "set -gx DRIFTR_NODE 22.14.0" {
		t.Errorf("snippet = %q, want fish syntax", got)
	}
}

func TestUseCmd_ShellFlagOverridesDetection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	fakeNodeInstall(t, "22.14.0")

	out, err := runCmdOut(t, "use", "node@22.14.0", "--shell", "fish")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "set -gx DRIFTR_NODE 22.14.0" {
		t.Errorf("snippet = %q, want fish syntax from --shell", got)
	}
}

func TestUseCmd_UnknownShellFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := runCmdOut(t, "use", "node@22.14.0", "--shell", "tcsh")
	if err == nil || !strings.Contains(err.Error(), "unknown shell") {
		t.Errorf("expected unknown-shell error, got: %v", err)
	}
}

func TestUseCmd_Unset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	out, err := runCmdOut(t, "use", "--unset", "node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "unset DRIFTR_NODE" {
		t.Errorf("snippet = %q, want %q", got, "unset DRIFTR_NODE")
	}

	out, err = runCmdOut(t, "use", "--unset", "pnpm", "--shell", "fish")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "set -e DRIFTR_PNPM" {
		t.Errorf("snippet = %q, want %q", got, "set -e DRIFTR_PNPM")
	}
}

func TestUseCmd_MissingVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := runCmdOut(t, "use", "pnpm")
	if err == nil || !strings.Contains(err.Error(), "version required") {
		t.Errorf("expected version-required error, got: %v", err)
	}
}

func TestUseCmd_UnknownTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := runCmdOut(t, "use", "ruby@3.0.0")
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected unknown-tool error, got: %v", err)
	}
}

func TestUseCmd_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := runCmdOut(t, "use", "node@22.14.0")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected not-installed error, got: %v", err)
	}
}
