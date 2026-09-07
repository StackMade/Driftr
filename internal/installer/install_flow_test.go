package installer

import (
	"archive/tar"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

// serveNodeDist starts a fake nodejs.org/dist serving one archive and the
// SHASUMS256.txt line for it, and points the installer's HTTP client at it.
// A shasum of "" makes SHASUMS256.txt list a hash that does not match.
func serveNodeDist(t *testing.T, version string, archive []byte, shasum string) {
	t.Helper()

	filename := ArchiveFilename(version)
	if shasum == "" {
		shasum = strings.Repeat("0", 64)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/SHASUMS256.txt"):
			fmt.Fprintf(w, "%s  %s\n", shasum, filename)
		case strings.HasSuffix(r.URL.Path, filename):
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	swapHTTPClient(t, srv)
}

// nodeTarball builds a Node.js release archive for version, with the given
// content as the node binary.
func nodeTarball(t *testing.T, version, binary string) []byte {
	t.Helper()
	path := buildTarGz(t, []tarEntry{
		{Name: nodePrefix(version) + "bin/", Typeflag: tar.TypeDir},
		{Name: nodePrefix(version) + "bin/node", Data: []byte(binary), Typeflag: tar.TypeReg},
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestVerifyChecksum_Match(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	content := []byte("archive bytes")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	serveNodeDist(t, "99.0.0", content, sum)

	archive := filepath.Join(t.TempDir(), ArchiveFilename("99.0.0"))
	if err := os.WriteFile(archive, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyChecksum(archive, "99.0.0", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	serveNodeDist(t, "99.0.0", nil, "")

	archive := filepath.Join(t.TempDir(), ArchiveFilename("99.0.0"))
	if err := os.WriteFile(archive, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksum(archive, "99.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got: %v", err)
	}
}

func TestVerifyChecksum_FilenameAbsentFromShasums(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "deadbeef  some-other-file.tar.gz\n")
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	archive := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := os.WriteFile(archive, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksum(archive, "99.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "checksum not found") {
		t.Fatalf("expected a checksum-not-found error, got: %v", err)
	}
}

func TestFetchExpectedChecksum_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	_, err := FetchExpectedChecksum("99.0.0", "node.tar.gz")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected an HTTP 500 error, got: %v", err)
	}
}

func TestInstall_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	archive := nodeTarball(t, "99.0.0", "#!/bin/sh\necho 99\n")
	serveNodeDist(t, "99.0.0", archive, fmt.Sprintf("%x", sha256.Sum256(archive)))

	got, err := Install("99.0.0", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "99.0.0" {
		t.Errorf("Install() = %q, want %q", got, "99.0.0")
	}

	bin, err := platform.NodeBinary("99.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Errorf("node binary not installed: %v", err)
	}
}

// A corrupt download must not be left in the cache, or every retry would
// re-verify the same bad bytes and fail again.
func TestInstall_ChecksumMismatchRemovesCachedArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	archive := nodeTarball(t, "99.0.0", "#!/bin/sh\n")
	serveNodeDist(t, "99.0.0", archive, "") // published hash does not match

	_, err := Install("99.0.0", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got: %v", err)
	}

	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(cacheDir, ArchiveFilename("99.0.0"))
	if _, statErr := os.Stat(cached); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("corrupt archive still cached at %s", cached)
	}

	versionDir, err := platform.NodeVersionDir("99.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(versionDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("version dir left behind after a failed install: %s", versionDir)
	}
}

// An archive that passes its checksum but unpacks to the wrong layout must
// leave no version directory: a half-written one would look installed to the
// resolver.
func TestInstall_ExtractionFailureWipesVersionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := buildTarGz(t, []tarEntry{
		{Name: "unexpected-layout/readme", Data: []byte("no node here"), Typeflag: tar.TypeReg},
	})
	archive, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	serveNodeDist(t, "99.0.0", archive, fmt.Sprintf("%x", sha256.Sum256(archive)))

	if _, err := Install("99.0.0", false); err == nil {
		t.Fatal("expected extraction to fail, got nil")
	}

	versionDir, err := platform.NodeVersionDir("99.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(versionDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("partial install left behind: %s", versionDir)
	}
}

// An already-installed version is a no-op, and must stay one even with no
// network available at all.
func TestInstall_AlreadyInstalledSkipsNetwork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request for an already-installed version: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	bin, err := platform.NodeBinary("99.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Install("99.0.0", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "99.0.0" {
		t.Errorf("Install() = %q, want %q", got, "99.0.0")
	}
}

func TestInstall_InvalidVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := Install("not.a.version", false); err == nil ||
		!strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("expected an invalid-version error, got: %v", err)
	}
}

func TestListInstalledVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, v := range []string{"20.11.0", "22.14.0"} {
		if err := os.MkdirAll(filepath.Join(home, ".driftr", "tools", "node", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// An interrupted extraction leaves this behind; it is not an install.
	if err := os.MkdirAll(filepath.Join(home, ".driftr", "tools", "node", "23.0.0.tmp-123"), 0o755); err != nil {
		t.Fatal(err)
	}

	versions, err := ListInstalledVersions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("ListInstalledVersions() = %v, want the two complete installs", versions)
	}

	pnpm, err := ListInstalledToolVersions("pnpm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pnpm) != 0 {
		t.Errorf("ListInstalledToolVersions(pnpm) = %v, want empty", pnpm)
	}
}
