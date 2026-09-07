package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallTool_UnknownTool(t *testing.T) {
	_, err := installTool("ruby", "3.0.0", false)
	if err == nil {
		t.Fatal("expected an error for an unsupported tool")
	}
	if !strings.Contains(err.Error(), "unknown tool: ruby") {
		t.Errorf("error = %v, want it to name the tool", err)
	}
	// The message has to list what is supported, otherwise the user is stuck.
	for _, tool := range []string{"node", "pnpm", "yarn", "bun"} {
		if !strings.Contains(err.Error(), tool) {
			t.Errorf("error = %v, want it to list %q as supported", err, tool)
		}
	}
}

func TestInstallCmd_UnknownToolWithVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// Must fail on the tool name alone — offline, before any network call.
	err := runCmd(t, "install", "ruby@3.0.0")
	if err == nil || !strings.Contains(err.Error(), "unknown tool: ruby") {
		t.Errorf("expected an unknown-tool error, got: %v", err)
	}
}

func TestInstallCmd_ProjectPinsUnknownTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".driftr.toml"), []byte("[tools]\nruby = \"3.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	// An unsupported tool in .driftr.toml is not a pin the resolver reports, so
	// the command must say there is nothing to install rather than exiting 0
	// having installed nothing, or attempting a download for "ruby".
	err := runCmd(t, "install")
	if err == nil || !strings.Contains(err.Error(), "no tool versions pinned") {
		t.Errorf("expected a no-pins error, got: %v", err)
	}
}
