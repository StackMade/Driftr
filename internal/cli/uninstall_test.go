package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn and returns everything it printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestWarnIfPinned_FindsPinInParentDirectory(t *testing.T) {
	root := t.TempDir()
	pinFile := filepath.Join(root, ".driftr.toml")
	if err := os.WriteFile(pinFile, []byte("[tools]\nnode = \"22.14.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "packages", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { warnIfPinned(nested, "node", "22.14.0") })

	if !strings.Contains(out, pinFile) {
		t.Errorf("warning does not name the pinning file %q, got: %q", pinFile, out)
	}
}

func TestWarnIfPinned_SilentWhenVersionDiffers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".driftr.toml"), []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { warnIfPinned(root, "node", "22.14.0") })

	if out != "" {
		t.Errorf("expected no warning for a different pinned version, got: %q", out)
	}
}
