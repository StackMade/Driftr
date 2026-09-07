package installer

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestInstallCleanup_RemovesTmpFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "driftr-download-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cleanup := &installCleanup{}
	cleanup.setTmpFile(tmpPath)
	cleanup.run()

	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file should have been removed: %s", tmpPath)
	}
}

func TestInstallCleanup_RemovesVersionDir(t *testing.T) {
	// Point HOME to a temp dir so platform.NodeVersionDir resolves there.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a fake version directory structure under ~/.driftr/tools/node/99.0.0.
	versionDir := filepath.Join(home, ".driftr", "tools", "node", "99.0.0")
	binDir := filepath.Join(versionDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "node"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup := &installCleanup{version: "99.0.0"}
	cleanup.run()

	if _, err := os.Stat(versionDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("version directory should have been removed: %s", versionDir)
	}
}

// The extraction work dir is named by os.MkdirTemp, so its suffix is random
// and cleanup can only remove the directory Extract told it about. An earlier
// version reconstructed the name from the pid and therefore never matched.
func TestInstallCleanup_RemovesRegisteredTmpDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	toolsDir := filepath.Join(home, ".driftr", "tools", "node")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmpDir, err := os.MkdirTemp(toolsDir, "99.0.0.tmp-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "partial"), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanup := &installCleanup{version: "99.0.0"}
	cleanup.setTmpDir(tmpDir)
	cleanup.run()

	if _, err := os.Stat(tmpDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("extraction work dir should have been removed: %s", tmpDir)
	}
}

func TestInstallCleanup_ClearTmpDirPreventsRemoval(t *testing.T) {
	dir := t.TempDir()
	tmpDir := filepath.Join(dir, "99.0.0.tmp-abc")
	if err := os.Mkdir(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup := &installCleanup{}
	cleanup.setTmpDir(tmpDir)
	cleanup.clearTmpDir()
	cleanup.run()

	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("cleared work dir should have survived: %v", err)
	}
}

func TestInstallCleanup_RemovesBothTmpFileAndVersionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create temp file.
	tmpFile, err := os.CreateTemp(t.TempDir(), "driftr-download-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Create version dir.
	versionDir := filepath.Join(home, ".driftr", "tools", "node", "99.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanup := &installCleanup{}
	cleanup.setTmpFile(tmpPath)
	cleanup.setVersion("99.0.0")
	cleanup.run()

	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file should have been removed")
	}
	if _, err := os.Stat(versionDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("version directory should have been removed")
	}
}

func TestInstallCleanup_RunIdempotent(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "driftr-download-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cleanup := &installCleanup{}
	cleanup.setTmpFile(tmpPath)

	// Call run twice — should not panic or error.
	cleanup.run()
	cleanup.run()

	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file should have been removed")
	}
}

func TestInstallCleanup_ConcurrentAccess(t *testing.T) {
	cleanup := &installCleanup{}

	var wg sync.WaitGroup
	// Hammer set/clear/run concurrently to test for races.
	for range 50 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			cleanup.setTmpFile("/tmp/fake")
		}()
		go func() {
			defer wg.Done()
			cleanup.clearTmpFile()
		}()
		go func() {
			defer wg.Done()
			cleanup.run()
		}()
	}
	wg.Wait()
}

func TestInstallCleanup_ClearPreventsRemoval(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "driftr-download-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cleanup := &installCleanup{}
	cleanup.setTmpFile(tmpPath)
	cleanup.clearTmpFile()
	cleanup.run()

	// File should still exist since we cleared it before running cleanup.
	if _, err := os.Stat(tmpPath); err != nil {
		t.Errorf("temp file should NOT have been removed after clearTmpFile")
	}
}
