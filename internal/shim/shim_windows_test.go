package shim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
	"github.com/stackmade/driftr/v2/internal/testutil"
)

// writeNodeBin puts a package bin's .cmd in a node version's bin dir, the way
// a global npm install leaves it.
func writeNodeBin(t *testing.T, version, name string) {
	t.Helper()
	dir, _ := platform.NodeVersionDir(version)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", name+".cmd"), []byte("@ECHO off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateShims_Windows(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	writeNodeBin(t, "22.0.0", "tsc")
	binDir, _ := ShimDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A recorded package shim whose bin is gone goes; an .exe nobody recorded stays.
	for _, name := range []string{"gone.exe", "mine.exe"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(binDir, manifestFile), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := GenerateShims("test"); err != nil {
		t.Fatal(err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range append(ShimTools(), "tsc") {
		got, err := os.ReadFile(filepath.Join(binDir, tool+".exe"))
		if err != nil {
			t.Errorf("shim for %s: %v", tool, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("shim for %s is not a copy of the running driftr", tool)
		}
	}
	if _, err := os.Stat(filepath.Join(binDir, "gone.exe")); !os.IsNotExist(err) {
		t.Errorf("stale shim kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "mine.exe")); err != nil {
		t.Errorf("unrecorded file removed: %v", err)
	}
	manifest, _ := os.ReadFile(filepath.Join(binDir, manifestFile))
	if string(manifest) != "tsc\n" {
		t.Errorf("manifest = %q, want only tsc", manifest)
	}

	// A second run keeps the shims: they already are the running binary or a copy of it.
	if _, err := GenerateShims("test"); err != nil {
		t.Fatalf("regenerating: %v", err)
	}
}

func TestGenerateShims_WindowsNeverOverwritesAForeignFile(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	writeNodeBin(t, "22.0.0", "driftr")
	binDir, _ := ShimDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userBinary := []byte("MZ not a shim")
	if err := os.WriteFile(filepath.Join(binDir, "driftr.exe"), userBinary, 0o755); err != nil {
		t.Fatal(err)
	}

	skipped, err := GenerateShims("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "driftr: ") {
		t.Errorf("want driftr reported as skipped, got %q", skipped)
	}
	if got, _ := os.ReadFile(filepath.Join(binDir, "driftr.exe")); string(got) != string(userBinary) {
		t.Errorf("file in the bin dir was overwritten: %q", got)
	}
}

// A shim that is running cannot be replaced in place; writeShim moves it aside.
func TestWriteShim_ReplacesARunningShim(t *testing.T) {
	binDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "driftr.exe")
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(binDir, "node.exe")
	if err := ioutil.CopyFile(self, path); err != nil {
		t.Fatal(err)
	}
	running := exec.Command(path, "-test.run=^TestHelperSleep$")
	running.Env = append(os.Environ(), "DRIFTR_TEST_SLEEP=1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = running.Process.Kill()
		_ = running.Wait()
	}()

	if err := writeShim(binDir, "node", src); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("shim = %q, want the new binary", got)
	}
	if old, _ := filepath.Glob(path + ".*.old"); len(old) != 1 {
		t.Errorf("want the running shim moved aside, got %q", old)
	}
}

// TestHelperSleep is the process TestWriteShim_ReplacesARunningShim keeps running.
func TestHelperSleep(t *testing.T) {
	if os.Getenv("DRIFTR_TEST_SLEEP") == "" {
		t.Skip("helper process")
	}
	time.Sleep(time.Minute)
}

func TestInvokedAs(t *testing.T) {
	// The test binary is not in a bin dir, so it runs as driftr.
	testutil.SetHome(t, t.TempDir())
	if tool, ok := InvokedAs(); ok {
		t.Errorf("InvokedAs() = %q outside the bin dir", tool)
	}
}

func TestShimTool(t *testing.T) {
	for file, want := range map[string]string{"node.exe": "node", "TSC.EXE": "TSC", "tsc.cmd": "", ".driftr-version": ""} {
		if got := shimTool(file); got != want {
			t.Errorf("shimTool(%q) = %q, want %q", file, got, want)
		}
	}
}
