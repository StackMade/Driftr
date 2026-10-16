package installer

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stackmade/driftr/internal/platform"
)

func swapHTTPClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Transport: &hostRewriteTransport{target: srv.URL}}
	t.Cleanup(func() { httpClient = orig })
}

func TestDownload_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	content := []byte("fake node archive")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	path, err := Download("99.0.0", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("downloaded content mismatch: got %q", got)
	}

	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != cacheDir {
		t.Errorf("archive not in cache dir: %s", path)
	}
}

func TestDownload_VersionNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	_, err := Download("99.0.0", false, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestDownload_ServerError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	_, err := Download("99.0.0", false, nil)
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected status-500 error, got: %v", err)
	}
}

func TestDownload_UsesCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cached := []byte("cached archive")
	cachedPath := filepath.Join(cacheDir, ArchiveFilename("99.0.0"))
	if err := os.WriteFile(cachedPath, cached, 0o644); err != nil {
		t.Fatal(err)
	}

	// A failing server proves the network is never touched on a cache hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("network request made despite cache hit")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	path, err := Download("99.0.0", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cachedPath {
		t.Errorf("expected cached path %s, got %s", cachedPath, path)
	}
}

func TestDownload_EmptyCachedFileRedownloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir, err := platform.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cachedPath := filepath.Join(cacheDir, ArchiveFilename("99.0.0"))
	if err := os.WriteFile(cachedPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	content := []byte("fresh archive")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()
	swapHTTPClient(t, srv)

	path, err := Download("99.0.0", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("zero-byte cache entry was not re-downloaded: got %q", got)
	}
}

func TestNodeDistBase_EnvOverride(t *testing.T) {
	t.Setenv("DRIFTR_NODE_MIRROR", "http://127.0.0.1:9123/")
	if got := nodeDistBase(); got != "http://127.0.0.1:9123" {
		t.Errorf("nodeDistBase() = %q, want trailing slash trimmed", got)
	}
	if got := DownloadURL("22.0.0"); !strings.HasPrefix(got, "http://127.0.0.1:9123/v22.0.0/") {
		t.Errorf("DownloadURL did not use mirror: %q", got)
	}
}

func slowServer(t *testing.T, n int, gap time.Duration, stall bool) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !stall {
			w.Header().Set("Content-Length", strconv.Itoa(n))
		}
		w.WriteHeader(http.StatusOK)
		for range n {
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(gap)
		}
		if stall {
			<-release
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first, unblocking srv.Close
	return srv
}

// The client Timeout used to count the body read and kill node downloads below ~450 KB/s.
func TestFetchToCache_SlowSteadyDownloadOutlivesClientTimeout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := slowServer(t, 40, 20*time.Millisecond, false)
	swapHTTPClient(t, srv)
	httpClient.Timeout = 300 * time.Millisecond

	if _, err := fetchToCache(srv.URL+"/a", "a.bin", 1<<20, false, nil); err != nil {
		t.Fatalf("steady download aborted: %v", err)
	}
}

func TestFetchToCache_StalledDownloadTimesOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := slowServer(t, 10, 0, true)
	swapHTTPClient(t, srv)
	orig := downloadIdleTimeout
	downloadIdleTimeout = 200 * time.Millisecond
	t.Cleanup(func() { downloadIdleTimeout = orig })

	_, err := fetchToCache(srv.URL+"/a", "a.bin", 1<<20, false, nil)
	if err == nil || !strings.Contains(err.Error(), "no data received") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}
}
