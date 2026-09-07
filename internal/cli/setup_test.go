package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManualPathInstructions_ShowsExportLine(t *testing.T) {
	var buf bytes.Buffer
	manualPathInstructions(&buf, "/home/someone/.driftr/bin")

	out := buf.String()
	// The line must be copy-pasteable as-is into a shell profile.
	if !strings.Contains(out, `export PATH="/home/someone/.driftr/bin:$PATH"`) {
		t.Errorf("output = %q, want a prepending export line for the bin dir", out)
	}
	if !strings.Contains(out, "shell profile") {
		t.Errorf("output = %q, want it to say where the line goes", out)
	}
	if !strings.Contains(out, "restart your shell") && !strings.Contains(out, "Then restart") {
		t.Errorf("output = %q, want the restart hint", out)
	}
}

func TestConfigureSetupPath_AlreadyConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")

	binDir := filepath.Join(home, ".driftr", "bin")
	zshenv := filepath.Join(home, ".zshenv")
	if err := os.WriteFile(zshenv, []byte("export PATH=\""+binDir+":$PATH\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A legacy entry left behind in the interactive-only rc file.
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export PATH=\""+binDir+":$PATH\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	configureSetupPath(&buf, binDir)

	out := buf.String()
	if !strings.Contains(out, "already configured") {
		t.Errorf("output = %q, want it to report the PATH as already configured", out)
	}
	if !strings.Contains(out, ".zshenv") {
		t.Errorf("output = %q, want it to name the universal rc file", out)
	}
	if !strings.Contains(out, ".zshrc") {
		t.Errorf("output = %q, want a note about the legacy .zshrc entry", out)
	}
	// Nothing may be appended when the PATH is already there.
	data, err := os.ReadFile(zshenv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), binDir) != 1 {
		t.Errorf(".zshenv = %q, want the export left untouched", data)
	}
}
