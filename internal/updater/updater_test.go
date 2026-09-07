package updater

import (
	"archive/tar"
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
)

// makeTarGz builds a tar.gz archive with the given file entries.
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
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("extracted binary is not executable: %v", info.Mode())
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

// rewriteTransport sends every request to the test server, preserving the path.
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
	// The override loosens the *initial* URL only. A redirect off HTTPS is
	// still refused, because self-update overwrites the running binary.
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := httpClient.CheckRedirect(req, nil); err == nil {
		t.Error("expected redirect to plaintext HTTP to be refused")
	}
}
