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
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".driftr.toml"), []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { warnIfPinned(root, "node", "22.14.0") })

	if out != "" {
		t.Errorf("expected no warning for a different pinned version, got: %q", out)
	}
}

func TestWarnIfPinned_EveryPinSource(t *testing.T) {
	cases := map[string]struct{ file, content string }{
		".nvmrc":            {".nvmrc", "18.0.0\n"},
		".node-version":     {".node-version", "18\n"},
		"partial toml pin":  {".driftr.toml", "[tools]\nnode = \"18\"\n"},
		"engines.node":      {"package.json", `{"engines":{"node":">=18"}}`},
		"exact driftr json": {"package.json", `{"driftr":{"node":"18.0.0"}}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			fakeNodeInstall(t, "18.0.0")
			proj := t.TempDir()
			if err := os.WriteFile(filepath.Join(proj, c.file), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}

			out := captureStdout(t, func() { warnIfPinned(proj, "node", "18.0.0") })
			if !strings.Contains(out, filepath.Join(proj, c.file)) {
				t.Errorf("output = %q, want a warning naming %s", out, c.file)
			}
		})
	}
}

// engines.node warns about the version the shim runs, the newest installed
// match, not the newest release of the range's lower-bound major.
func TestWarnIfPinned_EnginesWarnsForVersionInUse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "18.0.0")
	fakeNodeInstall(t, "22.14.0")
	proj := t.TempDir()
	pkg := filepath.Join(proj, "package.json")
	if err := os.WriteFile(pkg, []byte(`{"engines":{"node":">=18"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if out := captureStdout(t, func() { warnIfPinned(proj, "node", "22.14.0") }); !strings.Contains(out, pkg) {
		t.Errorf("removing the version in use: output = %q, want a warning naming %s", out, pkg)
	}
	if out := captureStdout(t, func() { warnIfPinned(proj, "node", "18.0.0") }); out != "" {
		t.Errorf("removing an unused version: got warning %q", out)
	}
}

func TestWarnIfPinned_RecordedProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "18.0.0")
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".nvmrc"), []byte("18.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddProject(proj); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { warnIfPinned(t.TempDir(), "node", "18.0.0") })
	if !strings.Contains(out, filepath.Join(proj, ".nvmrc")) {
		t.Errorf("output = %q, want a warning for the recorded project", out)
	}
}

func TestUninstallCmd_PartialVersionNamesExactCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	fakeNodeInstall(t, "22.14.0")

	err := runCmd(t, "uninstall", "node@22")
	if err == nil || !strings.Contains(err.Error(), "driftr uninstall node@22.14.0") {
		t.Errorf("error = %v, want it to name `driftr uninstall node@22.14.0`", err)
	}
	if _, statErr := os.Stat(filepath.Join(os.Getenv("HOME"), ".driftr", "tools", "node", "22.14.0")); statErr != nil {
		t.Errorf("a partial spec must not remove anything: %v", statErr)
	}

	err = runCmd(t, "uninstall", "node@20")
	if err == nil || !strings.Contains(err.Error(), "driftr list node") {
		t.Errorf("error = %v, want it to suggest `driftr list node`", err)
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
	npmDir, err := platform.ToolVersionDir("npm", "10.9.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(npmDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	// npx ships with npm; uninstalling it must act on the npm install, and
	// leave node alone.
	captureStdout(t, func() {
		if err := runCmd(t, "uninstall", "npx@10.9.2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if _, statErr := os.Stat(npmDir); !os.IsNotExist(statErr) {
		t.Errorf("npm version dir still exists after `uninstall npx@...`: %s", npmDir)
	}
	nodeDir, err := platform.ToolVersionDir("node", "22.14.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(nodeDir); statErr != nil {
		t.Errorf("node version dir removed by `uninstall npx@...`: %v", statErr)
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
