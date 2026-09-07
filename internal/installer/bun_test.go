package installer

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
)

// bunFixture serves a fake GitHub release index and download host for bun.
// Point the installer at it with the DRIFTR_BUN_* overrides.
type bunFixture struct {
	asset    string
	archive  []byte
	shasums  string // served as SHASUMS256.txt; empty means 404
	releases string // served as the release index JSON
}

func newBunFixture(t *testing.T, versions ...string) *bunFixture {
	t.Helper()

	asset, err := BunAssetName()
	if err != nil {
		t.Skipf("bun has no build for this platform: %v", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: strings.TrimSuffix(asset, ".zip") + "/bun", Method: zip.Deflate}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("#!/bin/sh\necho 1.2.3\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	var index bytes.Buffer
	index.WriteByte('[')
	for i, v := range versions {
		if i > 0 {
			index.WriteByte(',')
		}
		fmt.Fprintf(&index, `{"tag_name":"bun-v%s","draft":false,"prerelease":false}`, v)
	}
	index.WriteByte(']')

	return &bunFixture{
		asset:    asset,
		archive:  buf.Bytes(),
		shasums:  fmt.Sprintf("%x  %s\n", sha256.Sum256(buf.Bytes()), asset),
		releases: index.String(),
	}
}

// serve starts the fixture and points the installer's env overrides at it.
func (f *bunFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, f.releases)
		case strings.HasSuffix(r.URL.Path, "/SHASUMS256.txt"):
			if f.shasums == "" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, f.shasums)
		case strings.HasSuffix(r.URL.Path, f.asset):
			_, _ = w.Write(f.archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("DRIFTR_BUN_RELEASES", srv.URL+"/releases")
	t.Setenv("DRIFTR_BUN_MIRROR", srv.URL)
	return srv
}

func TestInstallBun_RejectsLTS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := InstallBun("lts", false); err == nil {
		t.Fatal("InstallBun(lts) expected error, got nil")
	}
}

func TestInstallBun_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.serve(t)

	got, err := InstallBun("1.2.3", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("InstallBun returned %q, want 1.2.3", got)
	}

	binPath, err := platform.ToolBinary("bun", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("bun binary not installed: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("bun binary is not executable")
	}

	// A second install is a no-op that must not fail.
	if _, err := InstallBun("1.2.3", false); err != nil {
		t.Errorf("reinstall failed: %v", err)
	}
}

func TestInstallBun_ResolvesPartialAndLatest(t *testing.T) {
	for _, spec := range []string{"1", "1.2", "latest"} {
		t.Run(spec, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			// Newest first, as the GitHub API returns them.
			f := newBunFixture(t, "1.2.3", "1.1.0", "0.9.0")
			f.serve(t)

			got, err := InstallBun(spec, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "1.2.3" {
				t.Errorf("InstallBun(%q) = %q, want 1.2.3", spec, got)
			}
		})
	}
}

func TestInstallBun_UnknownVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.serve(t)

	_, err := InstallBun("7", false)
	if err == nil || !strings.Contains(err.Error(), "no bun release found matching") {
		t.Fatalf("expected no-match error, got: %v", err)
	}
}

func TestInstallBun_ChecksumMismatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = fmt.Sprintf("%064d  %s\n", 0, f.asset)
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got: %v", err)
	}

	// The corrupt archive must not survive in the cache.
	cacheDir, cErr := platform.CacheDir()
	if cErr != nil {
		t.Fatal(cErr)
	}
	entries, cErr := os.ReadDir(cacheDir)
	if cErr != nil {
		t.Fatal(cErr)
	}
	if len(entries) != 0 {
		t.Errorf("corrupt archive left in cache: %v", entries)
	}

	// And nothing may look installed.
	dir, dErr := platform.ToolVersionDir("bun", "1.2.3")
	if dErr != nil {
		t.Fatal(dErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "bun")); err == nil {
		t.Error("bun was installed despite a checksum mismatch")
	}
}

func TestInstallBun_MissingChecksums(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = "" // release publishes no SHASUMS256.txt
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "refusing to install unverified package") {
		t.Fatalf("expected refusal to install unverified, got: %v", err)
	}
}

func TestInstallBun_ChecksumMissingForAsset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = "0000  bun-some-other-platform.zip\n"
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "refusing to install unverified package") {
		t.Fatalf("expected refusal to install unverified, got: %v", err)
	}
}

func TestListBunReleases_FiltersNonVersionTags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := newBunFixture(t)
	f.releases = `[
		{"tag_name":"bun-v1.2.3"},
		{"tag_name":"bun-v1.2.2","draft":true},
		{"tag_name":"bun-v1.2.1","prerelease":true},
		{"tag_name":"canary"},
		{"tag_name":"bun-v1.2.0"}
	]`
	f.serve(t)

	got, err := ListBunReleases()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"1.2.3", "1.2.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ListBunReleases() = %v, want %v", got, want)
	}
}

func TestBunEnvOverrides(t *testing.T) {
	t.Setenv("DRIFTR_BUN_MIRROR", "http://127.0.0.1:9124/")
	t.Setenv("DRIFTR_BUN_RELEASES", "http://127.0.0.1:9124/releases/")
	if got := bunDownloadBase(); got != "http://127.0.0.1:9124" {
		t.Errorf("bunDownloadBase() = %q, want trailing slash trimmed", got)
	}
	if got := bunReleasesURL(); got != "http://127.0.0.1:9124/releases" {
		t.Errorf("bunReleasesURL() = %q, want trailing slash trimmed", got)
	}
}
