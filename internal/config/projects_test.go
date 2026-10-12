package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

func TestLoadProjects_MissingRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dirs, err := LoadProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirs != nil {
		t.Errorf("expected nil, got %v", dirs)
	}
}

func TestAddProject_RecordsOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())
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
