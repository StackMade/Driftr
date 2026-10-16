package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

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

func TestUseCmd_UnsetAcceptsToolAtVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	out, err := runCmdOut(t, "use", "--unset", "node@22")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "unset DRIFTR_NODE" {
		t.Errorf("snippet = %q, want %q", got, "unset DRIFTR_NODE")
	}
}

func TestBundledToolsMapToTheirParent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	out, err := runCmdOut(t, "use", "--unset", "npx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "unset DRIFTR_NPM" {
		t.Errorf("snippet = %q, want %q", got, "unset DRIFTR_NPM")
	}

	// The remediation must name a command that works: npx is installed as npm.
	for _, cmd := range []string{"use", "pin", "default"} {
		err := runCmd(t, cmd, "npx@11.0.0")
		if err == nil || !strings.Contains(err.Error(), "driftr install npm@11.0.0") {
			t.Errorf("%s npx@11.0.0: error = %v, want it to suggest `driftr install npm@11.0.0`", cmd, err)
		}
	}
}

func TestUnknownToolListsKnownTools(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, cmd := range []string{"use", "pin", "default"} {
		err := runCmd(t, cmd, "foo@1")
		if err == nil || !strings.Contains(err.Error(), "unknown tool: foo") || !strings.Contains(err.Error(), "bun") {
			t.Errorf("%s foo@1: error = %v, want an unknown-tool error listing the supported tools", cmd, err)
		}
	}
}
