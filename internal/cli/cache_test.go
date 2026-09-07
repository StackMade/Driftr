package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

// writeCacheFiles fills the cache directory with files of the given sizes and
// returns the directory.
func writeCacheFiles(t *testing.T, sizes ...int) string {
	t.Helper()
	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, size := range sizes {
		name := filepath.Join(cacheDir, "node-v"+string(rune('a'+i))+".tar.gz")
		if err := os.WriteFile(name, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cacheDir
}

func TestCacheClean_RemovesCachedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cacheDir := writeCacheFiles(t, 1024, 2048)

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "cache", "clean") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(cacheDir); !os.IsNotExist(statErr) {
		t.Errorf("cache directory still exists: %s", cacheDir)
	}
	if !strings.Contains(out, "Removed 2 cached file(s)") {
		t.Errorf("output does not report the removed files: %q", out)
	}
	if !strings.Contains(out, "3.0 KB") {
		t.Errorf("output does not report the freed space: %q", out)
	}
}

// Installed versions live outside the cache and must survive a clean.
func TestCacheClean_KeepsInstalledVersions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCacheFiles(t, 10)
	fakeNodeInstall(t, "22.14.0")

	captureStdout(t, func() {
		if err := runCmd(t, "cache", "clean"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	bin, err := platform.NodeBinary("22.14.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Errorf("cache clean removed an installed version: %v", err)
	}
}

func TestCacheClean_NothingCached(t *testing.T) {
	tests := []struct {
		name        string
		createEmpty bool
	}{
		{"no cache directory", false},
		{"empty cache directory", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if tt.createEmpty {
				writeCacheFiles(t)
			}

			var err error
			out := captureStdout(t, func() { err = runCmd(t, "cache", "clean") })
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, "already empty") {
				t.Errorf("expected an already-empty message, got: %q", out)
			}
		})
	}
}

func TestCacheDir_PrintsCachePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}

	var runErr error
	out := captureStdout(t, func() { runErr = runCmd(t, "cache", "dir") })
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if strings.TrimSpace(out) != want {
		t.Errorf("cache dir printed %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1024*1024 - 1, "1024.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{3 * 1024 * 1024 / 2, "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatSize(tt.bytes); got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
