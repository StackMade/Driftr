package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/testutil"
)

func TestLoadProjects_MissingRegistry(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirs != nil {
		t.Errorf("expected nil, got %v", dirs)
	}
}

func TestAddProject_RecordsOnce(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	for _, dir := range []string{"/work/a", "/work/b", "/work/a", "/work/b/"} {
		if err := AddProject(dir); err != nil {
			t.Fatalf("AddProject(%q): %v", dir, err)
		}
	}

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/work/a", "/work/b"}; !slices.Equal(dirs, want) {
		t.Errorf("got %v, want %v", dirs, want)
	}
}

func TestLoadProjects_SkipsBlankLines(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	path, err := platform.ProjectsRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("/work/a\n\n  /work/b  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/work/a", "/work/b"}; !slices.Equal(dirs, want) {
		t.Errorf("got %v, want %v", dirs, want)
	}
}

func TestRemoveProjects_DropsOnlyGone(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	for _, dir := range []string{"/work/a", "/work/gone", "/work/b"} {
		if err := AddProject(dir); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveProjects([]string{"/work/gone", "/work/never-listed"}); err != nil {
		t.Fatal(err)
	}

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/work/a", "/work/b"}; !slices.Equal(dirs, want) {
		t.Errorf("got %v, want %v", dirs, want)
	}
}

// Concurrent adds once lost all but one or two projects, and prune deleted versions they pinned.
func TestAddProject_ConcurrentCallsAllRecorded(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if err := AddProject(fmt.Sprintf("/work/p%d", i)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != n {
		t.Errorf("recorded %d of %d projects: %v", len(dirs), n, dirs)
	}
}

func TestLoadProjects_DedupesAndCleans(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	path, err := platform.ProjectsRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("/work/a\n/work/b/\n/work/a\n/work/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/work/a", "/work/b"}; !slices.Equal(dirs, want) {
		t.Errorf("got %v, want %v", dirs, want)
	}
}
