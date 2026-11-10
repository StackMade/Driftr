package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/v2/internal/testutil"
)

func skillFile(t *testing.T, name string) string {
	t.Helper()
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range all {
		if h.Name == name {
			return h.File
		}
	}
	t.Fatalf("no harness %s", name)
	return ""
}

func wantStatus(t *testing.T, file string, want Status) {
	t.Helper()
	got, err := StatusOf(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("status of %s = %q, want %q", file, got, want)
	}
}

func TestSkillStartsWithFrontmatter(t *testing.T) {
	// Agents only parse frontmatter that opens on line 1, and Cursor needs
	// name to match the directory.
	if !strings.HasPrefix(skill(), "---\nname: driftr\ndescription: ") {
		t.Fatalf("bad frontmatter: %.60q", skill())
	}
}

func TestDetection(t *testing.T) {
	home := testutil.SetHome(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range all {
		if h.Detected != (h.Name == "codex") {
			t.Errorf("%s detected = %v", h.Name, h.Detected)
		}
	}
}

func TestInstallUninstall(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	file := skillFile(t, "claude")
	wantStatus(t, file, Missing)

	if err := Install(file); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, file, Current)

	removed, err := Uninstall(file)
	if err != nil || !removed {
		t.Fatalf("Uninstall = %v, %v", removed, err)
	}
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatalf("skill directory left behind: %v", err)
	}
}

func TestForeignSkillIsNeverTouched(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	file := skillFile(t, "claude")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	const mine = "my own notes"
	if err := os.WriteFile(file, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(file); err == nil || !strings.Contains(err.Error(), "not written by driftr") {
		t.Fatalf("Install = %v", err)
	}
	if removed, err := Uninstall(file); err != nil || removed {
		t.Fatalf("Uninstall = %v, %v", removed, err)
	}
	Refresh()

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != mine {
		t.Fatalf("foreign skill changed to %q", got)
	}
}

func TestDanglingSymlinkIsNeverTouched(t *testing.T) {
	testutil.SkipOnWindows(t, "creating a symlink needs privileges")
	testutil.SetHome(t, t.TempDir())
	file := skillFile(t, "claude")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	const target = "/nonexistent/my-skill.md"
	if err := os.Symlink(target, file); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, file, Foreign)

	if err := Install(file); err == nil || !strings.Contains(err.Error(), "not written by driftr") {
		t.Fatalf("Install = %v", err)
	}
	if removed, err := Uninstall(file); err != nil || removed {
		t.Fatalf("Uninstall = %v, %v", removed, err)
	}

	got, err := os.Readlink(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("symlink now points at %q", got)
	}
}

func TestUninstallKeepsOtherFiles(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	file := skillFile(t, "claude")
	if err := Install(file); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(file), "notes.md")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("other file removed: %v", err)
	}
}

func TestRefreshUpdatesOnlyManagedCopies(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	file := skillFile(t, "claude")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(managedMarker+"\nan older guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, file, Outdated)

	Refresh()

	wantStatus(t, file, Current)
	wantStatus(t, skillFile(t, "codex"), Missing)
}
