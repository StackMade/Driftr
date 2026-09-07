package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
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

func TestUninstallCmd_WarnsWhenVersionIsGlobalDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	if err := runCmd(t, "default", "node@22.14.0"); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runCmd(t, "uninstall", "node@22.14.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "global default") {
		t.Errorf("output = %q, want a warning that the global default was removed", out)
	}
	if !strings.Contains(out, "driftr default node@") {
		t.Errorf("output = %q, want the remediation command", out)
	}
}

func TestUninstallCmd_WarnsWhenVersionIsPinnedHere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	proj := t.TempDir()
	pkg := `{"name":"app","driftr":{"node":"22.14.0"}}`
	if err := os.WriteFile(filepath.Join(proj, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)

	out := captureStdout(t, func() {
		if err := runCmd(t, "uninstall", "node@22.14.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "package.json") || !strings.Contains(out, "is pinned in") {
		t.Errorf("output = %q, want a warning naming the package.json pin", out)
	}
}

func TestUninstallCmd_BundledToolResolvesToParent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	// npm ships with node; uninstalling it must act on the node install.
	captureStdout(t, func() {
		if err := runCmd(t, "uninstall", "npm@22.14.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	dir, err := platform.ToolVersionDir("node", "22.14.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("node version dir still exists after `uninstall npm@...`: %s", dir)
	}
}

func TestUninstallCmd_InvalidVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	// A traversal attempt must be rejected by version parsing, never reach the
	// filesystem.
	err := runCmd(t, "uninstall", "node@../../etc")
	if err == nil || !strings.Contains(err.Error(), "invalid version") {
		t.Errorf("expected an invalid-version error, got: %v", err)
	}
}
