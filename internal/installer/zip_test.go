package installer

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/v2/internal/platform"
)

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
	if info, err := os.Stat(filepath.Join(destDir, "bin", "bun")); err != nil || !platform.IsExecutable(info) {
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

func tmpLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			left = append(left, e.Name())
		}
	}
	return left
}

func TestExtractZipToBin_ConcurrentInstallWins(t *testing.T) {
	archive := writeTestZip(t, map[string]string{"bun-linux-x64/bun": "ours"})
	destDir := filepath.Join(t.TempDir(), "bun", "1.2.3")
	writeFileAll(t, filepath.Join(destDir, "bin", "bun"), "theirs")

	if err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun"); err != nil {
		t.Fatalf("losing the race should not fail: %v", err)
	}
	if got := readString(t, filepath.Join(destDir, "bin", "bun")); got != "theirs" {
		t.Errorf("winner's binary was replaced: %q", got)
	}
	if left := tmpLeftovers(t, filepath.Dir(destDir)); len(left) > 0 {
		t.Errorf("temp dirs left behind: %v", left)
	}
}

func TestExtractZipToBin_DestOccupiedWithoutBinary(t *testing.T) {
	archive := writeTestZip(t, map[string]string{"bun-linux-x64/bun": "ours"})
	destDir := filepath.Join(t.TempDir(), "bun", "1.2.3")
	writeFileAll(t, filepath.Join(destDir, "junk"), "")

	err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
	if err == nil || !strings.Contains(err.Error(), "failed to finalize install") {
		t.Fatalf("got %v, want a finalize error", err)
	}
	if left := tmpLeftovers(t, filepath.Dir(destDir)); len(left) > 0 {
		t.Errorf("temp dirs left behind: %v", left)
	}
}

func TestExtractZipToBin_CorruptDownloads(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "bun-linux-x64/bun", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("BUNBINARYPAYLOAD"))
	zw.Close()
	flipped := bytes.Replace(buf.Bytes(), []byte("BUNBINARYPAYLOAD"), []byte("BUNBINARYPAYLOAX"), 1)

	cases := map[string]struct {
		data []byte
		want string
	}{
		"crc mismatch":    {flipped, "failed to write file"},
		"html error page": {[]byte("<html>Not Found</html>"), "failed to open archive"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "bun.zip")
			writeFileAll(t, archive, string(tc.data))
			destDir := filepath.Join(t.TempDir(), "bun", "1.2.3")

			err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(destDir); !os.IsNotExist(err) {
				t.Errorf("version dir exists after a failed install: %v", err)
			}
		})
	}
}

func TestExtractZipToBin_SkipsSymlinkEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	bin, _ := zw.Create("bun-linux-x64/bun")
	bin.Write([]byte("bun"))
	hdr := &zip.FileHeader{Name: "bun-linux-x64/passwd"}
	hdr.SetMode(os.ModeSymlink | 0o777)
	link, _ := zw.CreateHeader(hdr)
	link.Write([]byte("/etc/passwd"))
	zw.Close()
	archive := filepath.Join(t.TempDir(), "bun.zip")
	writeFileAll(t, archive, buf.String())
	destDir := filepath.Join(t.TempDir(), "bun", "1.2.3")

	if err := ExtractZipToBin(archive, destDir, "bun-linux-x64/", "bun"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(destDir, "bin", "passwd")); !os.IsNotExist(err) {
		t.Errorf("symlink entry was extracted: %v", err)
	}
}

func writeFileAll(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
