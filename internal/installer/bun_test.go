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

	"github.com/stackmade/driftr/v2/internal/platform"
	"github.com/stackmade/driftr/v2/internal/testutil"
	"github.com/stackmade/driftr/v2/internal/version"
)

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
	hdr := &zip.FileHeader{Name: strings.TrimSuffix(asset, ".zip") + "/" + platform.ExeName("bun"), Method: zip.Deflate}
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
	testutil.SetHome(t, t.TempDir())

	if _, err := InstallBun("lts", false); err == nil {
		t.Fatal("InstallBun(lts) expected error, got nil")
	}
}

func TestInstallBun_Success(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	srv := f.serve(t)

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
	if !platform.IsExecutable(info) {
		t.Error("bun binary is not executable")
	}

	srv.Close()
	if _, err := InstallBun("1.2.3", false); err != nil {
		t.Errorf("reinstall failed: %v", err)
	}
}

func TestInstallBun_ResolvesPartialAndLatest(t *testing.T) {
	for _, spec := range []string{"1", "1.2", "latest"} {
		t.Run(spec, func(t *testing.T) {
			testutil.SetHome(t, t.TempDir())
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
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.serve(t)

	_, err := InstallBun("7", false)
	if err == nil || !strings.Contains(err.Error(), "no bun release found matching") {
		t.Fatalf("expected no-match error, got: %v", err)
	}
}

func TestInstallBun_ChecksumMismatch(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = fmt.Sprintf("%064d  %s\n", 0, f.asset)
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got: %v", err)
	}

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

	dir, dErr := platform.ToolVersionDir("bun", "1.2.3")
	if dErr != nil {
		t.Fatal(dErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "bun")); err == nil {
		t.Error("bun was installed despite a checksum mismatch")
	}
}

func TestInstallBun_MissingChecksums(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = "" // release publishes no SHASUMS256.txt
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "refusing to install unverified package") {
		t.Fatalf("expected refusal to install unverified, got: %v", err)
	}

	// The archive was never shown to be bad, so the retry reuses it.
	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "bun-v1.2.3-"+f.asset)); err != nil {
		t.Errorf("unverified archive dropped from the cache: %v", err)
	}
}

func TestInstallBun_ChecksumMissingForAsset(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	f.shasums = "0000  bun-some-other-platform.zip\n"
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "refusing to install unverified package") {
		t.Fatalf("expected refusal to install unverified, got: %v", err)
	}
}

func TestListBunReleases_FiltersNonVersionTags(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
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
	t.Setenv("DRIFTR_BUN_MIRROR", "")
	t.Setenv("DRIFTR_BUN_RELEASES", "")
	if bunDownloadBase() != defaultBunDownloadBase || bunReleasesURL() != defaultBunReleasesURL {
		t.Errorf("unset overrides: got %q, %q", bunDownloadBase(), bunReleasesURL())
	}

	t.Setenv("DRIFTR_BUN_MIRROR", "http://127.0.0.1:9124/")
	t.Setenv("DRIFTR_BUN_RELEASES", "http://127.0.0.1:9124/releases/")
	if got := bunDownloadBase(); got != "http://127.0.0.1:9124" {
		t.Errorf("bunDownloadBase() = %q, want trailing slash trimmed", got)
	}
	if got := bunReleasesURL(); got != "http://127.0.0.1:9124/releases" {
		t.Errorf("bunReleasesURL() = %q, want trailing slash trimmed", got)
	}
}

func TestInstallBun_GitHubRateLimit(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_BUN_RELEASES", srv.URL)

	_, err := InstallBun("1", false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("got %v, want the HTTP status", err)
	}
}

func TestInstallBun_NoBuildForPlatform(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	f := newBunFixture(t, "1.2.3")
	asset := f.asset
	f.asset = "bun-other-platform.zip"
	f.serve(t)

	_, err := InstallBun("1.2.3", false)
	if err == nil || !strings.Contains(err.Error(), "has no "+asset+" build") || !strings.Contains(err.Error(), "driftr list --remote bun") {
		t.Fatalf("got %v, want a missing-build error with the remedy", err)
	}
	dir, _ := platform.ToolVersionDir("bun", "1.2.3")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("version dir exists after a failed install: %v", err)
	}
}

// GitHub returns at most 100 releases per page. Reading only the first left
// "bun@0.8" unresolvable once bun had more than 100 releases.
func TestBunReleases_FollowsPages(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	// Page 1 is full of 1.x releases, page 2 holds the 0.x line.
	var page1 bytes.Buffer
	page1.WriteByte('[')
	for i := range bunReleasesPerPage {
		if i > 0 {
			page1.WriteByte(',')
		}
		fmt.Fprintf(&page1, `{"tag_name":"bun-v1.%d.0"}`, bunReleasesPerPage-i)
	}
	page1.WriteByte(']')
	pages := map[string]string{"1": page1.String(), "2": `[{"tag_name":"bun-v0.8.1"},{"tag_name":"bun-v0.8.0"}]`}

	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		requests = append(requests, page)
		body, ok := pages[page]
		if !ok {
			body = "[]"
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_BUN_RELEASES", srv.URL+"/releases")

	got, err := resolveBunVersion(version.Version{Major: 0, Minor: 8, Raw: "0.8"})
	if err != nil || got != "0.8.1" {
		t.Fatalf("resolveBunVersion(0.8) = %q, %v; want 0.8.1", got, err)
	}

	// A match on the first page needs no second request.
	requests = nil
	if got, err := resolveBunVersion(version.Version{Latest: true, Raw: "latest"}); err != nil || got != "1.100.0" {
		t.Fatalf("resolveBunVersion(latest) = %q, %v; want 1.100.0", got, err)
	}
	if len(requests) != 1 {
		t.Errorf("latest fetched pages %v, want only page 1", requests)
	}

	all, err := ListBunReleases()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != bunReleasesPerPage+2 {
		t.Errorf("ListBunReleases() returned %d versions, want %d", len(all), bunReleasesPerPage+2)
	}
}
