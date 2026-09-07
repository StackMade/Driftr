package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

// tarEntry describes one entry for buildTarGz. Link is the symlink target
// when Typeflag is tar.TypeSymlink.
type tarEntry struct {
	Name     string
	Data     []byte
	Typeflag byte
	Link     string
}

// buildTarGz creates a tar.gz archive file from ordered entries and returns its path.
func buildTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     0o755,
			Typeflag: e.Typeflag,
			Linkname: e.Link,
		}
		if e.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.Data))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", e.Name, err)
		}
		if e.Typeflag == tar.TypeReg {
			if _, err := tw.Write(e.Data); err != nil {
				t.Fatalf("write data %s: %v", e.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func nodePrefix(version string) string {
	return fmt.Sprintf("node-v%s-%s-%s/", version, platform.OS(), platform.Arch())
}

func TestExtract_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const ver = "99.0.0"
	prefix := nodePrefix(ver)

	archive := buildTarGz(t, []tarEntry{
		{Name: prefix, Typeflag: tar.TypeDir},
		{Name: prefix + "bin/", Typeflag: tar.TypeDir},
		{Name: prefix + "bin/node", Data: []byte("node binary"), Typeflag: tar.TypeReg},
		{Name: prefix + "README.md", Data: []byte("readme"), Typeflag: tar.TypeReg},
		{Name: "unrelated/stray.txt", Data: []byte("skipped"), Typeflag: tar.TypeReg},
	})

	if err := Extract(archive, ver, false, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeBin, err := platform.NodeBinary(ver)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(nodeBin)
	if err != nil {
		t.Fatalf("node binary not extracted: %v", err)
	}
	if string(got) != "node binary" {
		t.Errorf("content mismatch: got %q", got)
	}

	// Entries outside the expected prefix must not be extracted.
	destDir, err := platform.NodeVersionDir(ver)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "stray.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stray entry outside archive prefix was extracted")
	}
}

func TestExtract_MissingNodeBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const ver = "99.0.0"
	prefix := nodePrefix(ver)

	archive := buildTarGz(t, []tarEntry{
		{Name: prefix + "README.md", Data: []byte("readme"), Typeflag: tar.TypeReg},
	})

	err := Extract(archive, ver, false, nil)
	if err == nil || !strings.Contains(err.Error(), "node binary not found") {
		t.Fatalf("expected node-binary-not-found error, got: %v", err)
	}

	// The version dir must not exist — a broken install must never look valid.
	destDir, err := platform.NodeVersionDir(ver)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(destDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("version dir left behind after failed extraction: %s", destDir)
	}
}

func TestExtract_PathTraversalBlocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const ver = "99.0.0"
	prefix := nodePrefix(ver)

	archive := buildTarGz(t, []tarEntry{
		{Name: prefix + "../../../escape.txt", Data: []byte("escaped"), Typeflag: tar.TypeReg},
		{Name: prefix + "bin/node", Data: []byte("node binary"), Typeflag: tar.TypeReg},
	})

	// Whether Extract errors or skips the entry, nothing may be written
	// outside the version directory.
	_ = Extract(archive, ver, false, nil)

	// filepath.Glob has no recursive **, so walk the whole tree.
	err := filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "escape.txt" {
			t.Errorf("path traversal entry escaped the sandbox: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExtract_AlreadyExtracted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const ver = "99.0.0"

	// Pre-create the node binary so Extract should be a no-op.
	nodeBin, err := platform.NodeBinary(ver)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(nodeBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodeBin, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A corrupt archive proves the file is never opened on the skip path.
	archive := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	if err := os.WriteFile(archive, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Extract(archive, ver, false, nil); err != nil {
		t.Errorf("expected skip for already-extracted version, got: %v", err)
	}
}

func TestExtractRegistryPackage_Success(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "pnpm", "9.0.0")

	archive := buildTarGz(t, []tarEntry{
		{Name: "package/package.json", Data: []byte(`{"name":"pnpm"}`), Typeflag: tar.TypeReg},
		{Name: "package/bin/pnpm.cjs", Data: []byte("#!/usr/bin/env node"), Typeflag: tar.TypeReg},
	})

	binaryPath := filepath.Join(destDir, "bin", "pnpm.cjs")
	if err := ExtractRegistryPackage(archive, destDir, binaryPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("binary not extracted: %v", err)
	}
	if string(got) != "#!/usr/bin/env node" {
		t.Errorf("content mismatch: got %q", got)
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		t.Errorf("package.json not extracted with prefix stripped: %v", err)
	}
}

func TestExtractRegistryPackage_TraversalBlocked(t *testing.T) {
	parent := t.TempDir()
	destDir := filepath.Join(parent, "tool", "1.0.0")

	archive := buildTarGz(t, []tarEntry{
		{Name: "package/../../escape.txt", Data: []byte("escaped"), Typeflag: tar.TypeReg},
	})

	binaryPath := filepath.Join(destDir, "bin")
	if err := ExtractRegistryPackage(archive, destDir, binaryPath); err == nil {
		t.Error("expected error for traversal entry, got nil")
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("traversal entry escaped the sandbox")
	}
}

func TestExtractRegistryPackage_CorruptArchive(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "corrupt.tgz")
	if err := os.WriteFile(archive, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(t.TempDir(), "dest")

	if err := ExtractRegistryPackage(archive, destDir, filepath.Join(destDir, "bin")); err == nil {
		t.Error("expected error for corrupt archive, got nil")
	}
}

func TestRemoveCorruptArchive_Removes(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(archive, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("checksum mismatch")

	err := removeCorruptArchive(archive, cause)
	if !errors.Is(err, cause) {
		t.Errorf("expected cause to be returned, got: %v", err)
	}
	if _, statErr := os.Stat(archive); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("corrupt archive was not removed")
	}
}

func TestRemoveCorruptArchive_AlreadyGone(t *testing.T) {
	cause := errors.New("checksum mismatch")
	err := removeCorruptArchive(filepath.Join(t.TempDir(), "missing.tar.gz"), cause)
	if !errors.Is(err, cause) {
		t.Errorf("expected plain cause for missing file, got: %v", err)
	}
}

func TestRemoveCorruptArchive_Undeletable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	roDir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(roDir, "bad.tar.gz")
	if err := os.WriteFile(archive, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	cause := errors.New("checksum mismatch")
	err := removeCorruptArchive(archive, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("wrapped error must preserve cause, got: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to remove corrupt archive") {
		t.Errorf("expected remediation hint in error, got: %v", err)
	}
}

func TestExtract_WrongArchiveLayout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const ver = "99.0.0"

	// No entry carries the expected node-v<ver>-<os>-<arch>/ prefix.
	archive := buildTarGz(t, []tarEntry{
		{Name: "something-else/bin/node", Data: []byte("node"), Typeflag: tar.TypeReg},
	})

	err := Extract(archive, ver, false, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected layout") {
		t.Fatalf("expected unexpected-layout error, got: %v", err)
	}

	destDir, err := platform.NodeVersionDir(ver)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(destDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("version dir left behind after layout error: %s", destDir)
	}
}
