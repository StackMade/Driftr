package nodeenv

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirSize_SumsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 100)
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), 250)
	writeFile(t, filepath.Join(dir, "sub", "deep", "c.txt"), 50)

	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 400 {
		t.Errorf("DirSize = %d, want 400", got)
	}
}

func TestDirSize_MissingPathIsZero(t *testing.T) {
	got, err := DirSize(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("DirSize of missing path = %d, want 0", got)
	}
}

func TestDirSize_EmptyDir(t *testing.T) {
	got, err := DirSize(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("DirSize of empty dir = %d, want 0", got)
	}
}

func TestDirSize_SingleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	writeFile(t, path, 123)

	got, err := DirSize(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 123 {
		t.Errorf("DirSize of a file = %d, want 123", got)
	}
}

// An unreadable subdirectory is a real error, unlike a missing path.
func TestDirSize_UnreadableDirIsError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "a.txt"), 10)
	sub := filepath.Join(dir, "sub")
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	got, err := DirSize(dir)
	if err == nil {
		t.Fatalf("DirSize = %d, want a permission error", got)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("DirSize error = %v, want a permission error", err)
	}
	if got != 0 {
		t.Errorf("DirSize on error = %d, want 0", got)
	}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}
