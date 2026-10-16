package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/testutil"
)

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write data: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVerifyChecksum_Valid(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "driftr_1.0.0_linux_amd64.tar.gz")
	content := []byte("archive content")
	if err := os.WriteFile(archive, content, 0o644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "checksums.txt")
	line := fmt.Sprintf("%s  driftr_1.0.0_linux_amd64.tar.gz\n", sha256Hex(content))
	if err := os.WriteFile(checksums, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifyChecksum(archive, checksums); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "driftr_1.0.0_linux_amd64.tar.gz")
	if err := os.WriteFile(archive, []byte("actual content"), 0o644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "checksums.txt")
	line := sha256Hex([]byte("different content")) + "  driftr_1.0.0_linux_amd64.tar.gz\n"
	if err := os.WriteFile(checksums, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(archive, checksums)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}
}

func TestVerifyChecksum_NoEntryForArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "driftr_1.0.0_linux_amd64.tar.gz")
	if err := os.WriteFile(archive, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksums, []byte("abc123  other_file.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(archive, checksums)
	if err == nil || !strings.Contains(err.Error(), "no checksum found") {
		t.Errorf("expected no-checksum error, got: %v", err)
	}
}

func TestVerifyChecksum_MissingChecksumsFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := os.WriteFile(archive, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifyChecksum(archive, filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("expected error for missing checksums file, got nil")
	}
}

func TestExtractBinary_Found(t *testing.T) {
	dir := t.TempDir()
	binContent := []byte("#!/bin/sh\necho driftr\n")
	archiveData := makeTarGz(t, map[string][]byte{
		"README.md": []byte("readme"),
		"driftr":    binContent,
	})
	archive := filepath.Join(dir, "update.tar.gz")
	if err := os.WriteFile(archive, archiveData, 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "driftr-new")
	if err := extractBinary(archive, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binContent) {
		t.Errorf("extracted content mismatch: got %q", got)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !platform.IsExecutable(info) {
		t.Errorf("extracted binary is not executable: %v", info.Mode())
	}
}

// Windows releases are zips holding driftr.exe.
func TestExtractBinary_Zip(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string]string{"README.md": "readme", "driftr.exe": "MZ driftr"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "driftr_2.0.0_windows_amd64.zip")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "driftr-new")
	if err := extractBinary(archive, dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "MZ driftr" {
		t.Errorf("extracted %q, want driftr.exe", got)
	}
}

func TestExtractBinary_NotInArchive(t *testing.T) {
	dir := t.TempDir()
	archiveData := makeTarGz(t, map[string][]byte{"README.md": []byte("readme")})
	archive := filepath.Join(dir, "update.tar.gz")
	if err := os.WriteFile(archive, archiveData, 0o644); err != nil {
		t.Fatal(err)
	}

	err := extractBinary(archive, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "not found in archive") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestExtractBinary_CorruptArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "corrupt.tar.gz")
	if err := os.WriteFile(archive, []byte("not gzip data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := extractBinary(archive, filepath.Join(dir, "out")); err == nil {
		t.Error("expected error for corrupt archive, got nil")
	}
}

func TestReplaceBinary_Success(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new")
	oldPath := filepath.Join(dir, "old")
	if err := os.WriteFile(newPath, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(newPath, oldPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("binary not replaced: got %q", got)
	}
}

func TestReplaceBinary_TargetDirReadOnly(t *testing.T) {
	testutil.SkipOnWindows(t, "directory permission bits")
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	srcDir := t.TempDir()
	roDir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	newPath := filepath.Join(srcDir, "new")
	if err := os.WriteFile(newPath, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(newPath, filepath.Join(roDir, "old")); err == nil {
		t.Error("expected error replacing into read-only dir, got nil")
	}
}

func TestDownloadFile_Success(t *testing.T) {
	content := []byte("downloaded payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out")
	if err := downloadFile(srv.URL, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q", got)
	}
}

// A slow but steady download outlives the client timeout; a stalled one hits the idle timeout.
func TestDownloadFile_IdleTimeout(t *testing.T) {
	origTimeout, origIdle := httpClient.Timeout, downloadIdleTimeout
	t.Cleanup(func() { httpClient.Timeout, downloadIdleTimeout = origTimeout, origIdle })
	httpClient.Timeout = 100 * time.Millisecond
	downloadIdleTimeout = 300 * time.Millisecond

	chunks := func(n int, pause time.Duration) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for i := 0; i < n; i++ {
				w.Write([]byte("x"))
				w.(http.Flusher).Flush()
				select {
				case <-time.After(pause):
				case <-r.Context().Done():
					return
				}
			}
		}
	}

	slow := httptest.NewServer(chunks(6, 50*time.Millisecond))
	defer slow.Close()
	if err := downloadFile(slow.URL, filepath.Join(t.TempDir(), "out")); err != nil {
		t.Fatalf("slow but steady download failed: %v", err)
	}

	stalled := httptest.NewServer(chunks(2, 2*time.Second))
	defer stalled.Close()
	start := time.Now()
	if err := downloadFile(stalled.URL, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("stalled download succeeded, want an error")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("stalled download took %s, want it cut off near the idle timeout", d)
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	err := downloadFile(srv.URL, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 error, got: %v", err)
	}
}

type rewriteTransport struct{ target string }

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(rt.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestFetchLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v1.2.3"}`)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	defer func() { httpClient = orig }()

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("got %q, want %q", got, "1.2.3")
	}
}

func TestFetchLatestVersion_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = &http.Client{Transport: &rewriteTransport{target: srv.URL}}
	defer func() { httpClient = orig }()

	_, err := fetchLatestVersion()
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Errorf("expected status 403 error, got: %v", err)
	}
}

func TestBaseURLs_Default(t *testing.T) {
	// No env set: self-update must talk to GitHub and nowhere else.
	t.Setenv("DRIFTR_UPDATE_API", "")
	t.Setenv("DRIFTR_UPDATE_MIRROR", "")

	if got := apiBase(); got != "https://api.github.com/repos/stackmade/driftr" {
		t.Errorf("apiBase() = %q", got)
	}
	if got := releaseBase(); got != "https://github.com/stackmade/driftr/releases/download" {
		t.Errorf("releaseBase() = %q", got)
	}
}

func TestBaseURLs_Override(t *testing.T) {
	// Trailing slashes are trimmed so callers can always append "/...".
	t.Setenv("DRIFTR_UPDATE_API", "http://127.0.0.1:1234/update/api/")
	t.Setenv("DRIFTR_UPDATE_MIRROR", "http://127.0.0.1:1234/update/download//")

	if got := apiBase(); got != "http://127.0.0.1:1234/update/api" {
		t.Errorf("apiBase() = %q", got)
	}
	if got := releaseBase(); got != "http://127.0.0.1:1234/update/download" {
		t.Errorf("releaseBase() = %q", got)
	}
}

func TestFetchLatestVersion_UsesOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/update/api/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer srv.Close()

	t.Setenv("DRIFTR_UPDATE_API", srv.URL+"/update/api")

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got != "9.9.9" {
		t.Errorf("got %q, want 9.9.9", got)
	}
}

func TestRedirectGuardRejectsPlaintext(t *testing.T) {
	// The override loosens only the initial URL: self-update overwrites the binary, so no downgrade.
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := httpClient.CheckRedirect(req, nil); err == nil {
		t.Error("expected redirect to plaintext HTTP to be refused")
	}
}

func TestReplaceBinary_FallbackFailureLeavesNoTempFile(t *testing.T) {
	testutil.SkipOnWindows(t, "the target is moved aside first, so a directory there is no obstacle")
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new")
	oldPath := filepath.Join(dir, "driftr")
	if err := os.WriteFile(newPath, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(oldPath, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := replaceBinary(newPath, oldPath)
	if err == nil || !strings.Contains(err.Error(), "atomic replace also failed") {
		t.Fatalf("got %v, want the fallback rename error", err)
	}
	if _, err := os.Stat(oldPath + ".driftr-update"); !os.IsNotExist(err) {
		t.Errorf("temp copy left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldPath, "keep")); err != nil {
		t.Errorf("target was damaged: %v", err)
	}
}
