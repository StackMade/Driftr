package installer

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestZip builds a zip archive on disk from a name → content map.
func writeTestZip(t *testing.T, entries map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(0o755)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractZipToBin_Success(t *testing.T) {
	archive := writeTestZip(t, map[string]string{
		"bun-linux-x64/bun":       "#!/bin/sh\necho fake\n",
		"bun-linux-x64/LICENSE":   "MIT",
		"unrelated/other":         "ignored",
		"bun-linux-x64/sub/thing": "nested",
	})
	destDir := filepath.Join(t.TempDir(), "tools", "bun", "1.2.3")

	if err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "bin", "bun"))
	if err != nil {
		t.Fatalf("binary not extracted: %v", err)
	}
	if !strings.Contains(string(got), "fake") {
		t.Errorf("unexpected binary content: %q", got)
	}
	if info, err := os.Stat(filepath.Join(destDir, "bin", "bun")); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("binary is not executable: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "bin", "sub", "thing")); err != nil {
		t.Errorf("nested entry not extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "bin", "other")); err == nil {
		t.Error("entry outside the prefix was extracted")
	}
}

func TestExtractZipToBin_RejectsTraversal(t *testing.T) {
	archive := writeTestZip(t, map[string]string{
		"bun-linux-x64/../../escaped": "pwned",
		"bun-linux-x64/bun":           "ok",
	})
	parent := t.TempDir()
	destDir := filepath.Join(parent, "1.2.3")

	err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected traversal to be rejected, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped")); err == nil {
		t.Error("traversal entry was written outside the destination")
	}
	if _, err := os.Stat(destDir); err == nil {
		t.Error("failed extraction left a version directory behind")
	}
}

func TestExtractZipToBin_MissingBinary(t *testing.T) {
	archive := writeTestZip(t, map[string]string{
		"bun-linux-x64/README": "no binary here",
	})
	parent := t.TempDir()
	destDir := filepath.Join(parent, "1.2.3")

	err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing-binary error, got: %v", err)
	}
	if _, err := os.Stat(destDir); err == nil {
		t.Error("failed extraction left a version directory behind")
	}
	// No leftover work dirs either.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("temp work dir left behind: %v", entries)
	}
}

func TestExtractZipToBin_WrongLayout(t *testing.T) {
	archive := writeTestZip(t, map[string]string{"something/else": "x"})
	destDir := filepath.Join(t.TempDir(), "1.2.3")

	err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	if err == nil || !strings.Contains(err.Error(), "unexpected layout") {
		t.Fatalf("expected layout error, got: %v", err)
	}
}
