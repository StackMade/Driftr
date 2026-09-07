package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

// pruneEnv gives a test its own driftr home and an empty project directory as
// the working directory, so no config outside the test can pin anything.
func pruneEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	t.Chdir(project)
	return project
}

// nodeInstalled reports whether a fake node version still exists on disk.
func nodeInstalled(t *testing.T, ver string) bool {
	t.Helper()
	dir, err := platform.ToolVersionDir("node", ver)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(dir)
	return err == nil
}

func writePin(t *testing.T, dir, spec string) {
	t.Helper()
	body := "[tools]\nnode = \"" + spec + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".driftr.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrune_KeepsGlobalDefault(t *testing.T) {
	pruneEnv(t)
	fakeNodeInstall(t, "22.14.0")
	fakeNodeInstall(t, "20.0.0")
	if err := runCmd(t, "default", "node@22.14.0"); err != nil {
		t.Fatal(err)
	}

	var err error
	captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nodeInstalled(t, "22.14.0") {
		t.Error("global default 22.14.0 was pruned")
	}
	if nodeInstalled(t, "20.0.0") {
		t.Error("unreferenced 20.0.0 was not pruned")
	}
}

func TestPrune_KeepsProjectPin(t *testing.T) {
	project := pruneEnv(t)
	fakeNodeInstall(t, "22.14.0")
	fakeNodeInstall(t, "20.0.0")
	fakeNodeInstall(t, "18.0.0")
	if err := runCmd(t, "default", "node@22.14.0"); err != nil {
		t.Fatal(err)
	}
	writePin(t, project, "20.0.0")

	var err error
	captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nodeInstalled(t, "20.0.0") {
		t.Error("pinned 20.0.0 was pruned")
	}
	if !nodeInstalled(t, "22.14.0") {
		t.Error("global default 22.14.0 was pruned")
	}
	if nodeInstalled(t, "18.0.0") {
		t.Error("unreferenced 18.0.0 was not pruned")
	}
}

func TestPrune_PartialPinKeepsNewestMatch(t *testing.T) {
	project := pruneEnv(t)
	fakeNodeInstall(t, "24.1.0")
	fakeNodeInstall(t, "24.5.0")
	writePin(t, project, "24")

	var err error
	captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nodeInstalled(t, "24.5.0") {
		t.Error("newest version matching pin 24 was pruned")
	}
	if nodeInstalled(t, "24.1.0") {
		t.Error("older 24.1.0 was not pruned")
	}
}

func TestPrune_DryRunRemovesNothing(t *testing.T) {
	pruneEnv(t)
	fakeNodeInstall(t, "20.0.0")

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "prune", "--dry-run") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nodeInstalled(t, "20.0.0") {
		t.Error("dry run removed 20.0.0")
	}
	if !strings.Contains(out, "20.0.0") {
		t.Errorf("dry run did not list 20.0.0, got: %q", out)
	}
}

func TestPrune_NothingToPrune(t *testing.T) {
	pruneEnv(t)
	fakeNodeInstall(t, "22.14.0")
	if err := runCmd(t, "default", "node@22.14.0"); err != nil {
		t.Fatal(err)
	}

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "prune") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Nothing to prune") {
		t.Errorf("expected a nothing-to-prune message, got: %q", out)
	}
	if !nodeInstalled(t, "22.14.0") {
		t.Error("22.14.0 was removed")
	}
}
