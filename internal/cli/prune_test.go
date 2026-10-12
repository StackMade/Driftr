package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/resolver"
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

func TestPrune_KeepsVersionPinnedByRecordedProject(t *testing.T) {
	pruneEnv(t)
	fakeNodeInstall(t, "20.0.0")
	fakeNodeInstall(t, "18.0.0")
	other := t.TempDir()
	writePin(t, other, "20")
	if err := config.AddProject(other); err != nil {
		t.Fatal(err)
	}

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nodeInstalled(t, "20.0.0") {
		t.Error("20.0.0, pinned by a recorded project elsewhere, was pruned")
	}
	if nodeInstalled(t, "18.0.0") {
		t.Error("unreferenced 18.0.0 was not pruned")
	}
	if !strings.Contains(out, "Checked 1 recorded project(s)") {
		t.Errorf("expected the checked-project count, got: %q", out)
	}
}

func TestPrune_ForgetsProjectsThatAreGone(t *testing.T) {
	pruneEnv(t)
	gone := filepath.Join(t.TempDir(), "deleted-checkout")
	if err := config.AddProject(gone); err != nil {
		t.Fatal(err)
	}

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "prune", "--dry-run") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, gone) {
		t.Errorf("dry run did not list the gone project, got: %q", out)
	}
	if dirs, _ := config.LoadProjects(); len(dirs) != 1 {
		t.Errorf("dry run changed the registry: %v", dirs)
	}

	captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirs, _ := config.LoadProjects(); len(dirs) != 0 {
		t.Errorf("gone project was not forgotten: %v", dirs)
	}
}

func TestPrune_UnreadableRecordedProjectRemovesNothing(t *testing.T) {
	pruneEnv(t)
	fakeNodeInstall(t, "20.0.0")
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, ".driftr.toml"), []byte("[tools\nnode = "), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddProject(broken); err != nil {
		t.Fatal(err)
	}

	var err error
	captureStdout(t, func() { err = runCmd(t, "prune", "-y") })
	if err == nil || !strings.Contains(err.Error(), "refusing to prune") {
		t.Fatalf("expected prune to refuse, got: %v", err)
	}
	if !nodeInstalled(t, "20.0.0") {
		t.Error("20.0.0 was removed although a recorded project could not be read")
	}
	if dirs, _ := config.LoadProjects(); len(dirs) != 1 {
		t.Errorf("unreadable project was dropped from the registry: %v", dirs)
	}
}

func TestPinCmd_RecordsProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	proj := t.TempDir()
	t.Chdir(proj)

	if err := runCmd(t, "pin", "node@22.14.0"); err != nil {
		t.Fatal(err)
	}

	dirs, err := config.LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || !sameDir(t, dirs[0], proj) {
		t.Errorf("registry = %v, want [%s]", dirs, proj)
	}
}

func TestRecordProject_UsesNearestPinDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	recordProject([]resolver.Pin{
		{Tool: "node", Version: "22", Dir: "/work/repo"},
		{Tool: "pnpm", Version: "9", Dir: "/work/repo/app"},
	})

	dirs, err := config.LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != "/work/repo/app" {
		t.Errorf("registry = %v, want [/work/repo/app]", dirs)
	}
}

// sameDir compares two paths after resolving symlinks, since temp dirs on
// macOS live behind /var -> /private/var.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		t.Fatal(err)
	}
	return ra == rb
}
