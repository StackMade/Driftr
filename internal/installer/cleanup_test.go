package installer

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stackmade/driftr/v2/internal/testutil"
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

// Only the registered tmp dir goes: its name is random, and the version dir may be another install.
func TestInstallCleanup_RemovesRegisteredTmpDirKeepsVersionDir(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	toolsDir := filepath.Join(home, ".driftr", "tools", "node")
	versionDir := filepath.Join(toolsDir, "99.0.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmpDir, err := os.MkdirTemp(toolsDir, "99.0.0.tmp-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "partial"), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanup := &installCleanup{}
	cleanup.setTmpDir(tmpDir)
	cleanup.run()

	if _, err := os.Stat(tmpDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("extraction work dir should have been removed: %s", tmpDir)
	}
	if _, err := os.Stat(versionDir); err != nil {
		t.Errorf("version dir should have been kept: %v", err)
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

func TestInstallCleanup_RunIdempotent(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "driftr-download-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cleanup := &installCleanup{}
	cleanup.setTmpFile(tmpPath)

	cleanup.run()
	cleanup.run()

	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temp file should have been removed")
	}
}

func TestInstallCleanup_ConcurrentAccess(t *testing.T) {
	cleanup := &installCleanup{}

	var wg sync.WaitGroup
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

	if _, err := os.Stat(tmpPath); err != nil {
		t.Errorf("temp file should NOT have been removed after clearTmpFile")
	}
}
