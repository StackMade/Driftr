package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
)

const defaultNodeDistBaseURL = "https://nodejs.org/dist"

// nodeDistBase returns the Node.js distribution base URL. DRIFTR_NODE_MIRROR
// overrides the default — for corporate mirrors and hermetic tests.
func nodeDistBase() string {
	if m := os.Getenv("DRIFTR_NODE_MIRROR"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultNodeDistBaseURL
}

const maxNodeDownloadBytes = 500 * 1024 * 1024 // 500 MB

// httpClient is the shared HTTP client for all installer network operations.
// Its Timeout bounds small metadata fetches. Archive downloads drop it, since
// it counts the body read too, and use downloadIdleTimeout instead.
var httpClient = &http.Client{
	Timeout: 120 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to non-HTTPS URL: %s", req.URL)
		}
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// downloadIdleTimeout aborts an archive download that receives no data for
// this long. A slow but steady download runs as long as it needs.
var downloadIdleTimeout = 60 * time.Second

// idleTimeoutReader pushes the idle deadline back on every read that
// returns data.
type idleTimeoutReader struct {
	r     io.Reader
	timer *time.Timer
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(downloadIdleTimeout)
	}
	return n, err
}

// DownloadURL returns the download URL for a given Node.js version.
func DownloadURL(version string) string {
	filename := ArchiveFilename(version)
	return fmt.Sprintf("%s/v%s/%s", nodeDistBase(), version, filename)
}

// ArchiveFilename returns the expected archive filename.
func ArchiveFilename(version string) string {
	return fmt.Sprintf("node-v%s-%s-%s.%s",
		version, platform.OS(), platform.Arch(), platform.ArchiveExt())
}

// removeCorruptArchive deletes a cached archive that failed verification.
// If removal fails, the corrupt archive would be reused on every subsequent
// attempt, so the failure is surfaced with a manual remediation hint.
func removeCorruptArchive(archivePath string, cause error) error {
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w (failed to remove corrupt archive; run `rm %q` and retry)", cause, archivePath)
	}
	return cause
}

// errNotFound reports that the download host answered 404. Callers wrap it
// with a message naming what was actually missing.
var errNotFound = errors.New("not found")

// Download fetches the Node.js archive to the cache directory.
// Returns the path to the downloaded file.
// If cleanup is non-nil, the temp file path is registered for signal-safe removal.
func Download(version string, verbose bool, cleanup *installCleanup) (string, error) {
	url := DownloadURL(version)
	path, err := fetchToCache(url, ArchiveFilename(version), maxNodeDownloadBytes, verbose, cleanup)
	if errors.Is(err, errNotFound) {
		return "", fmt.Errorf("node.js version %s not found at %s", version, url)
	}
	return path, err
}

// fetchToCache downloads url into the cache directory under filename and
// returns the resulting path. A non-empty file already cached under that name
// is reused without touching the network. If cleanup is non-nil, the temp file
// is registered for signal-safe removal.
func fetchToCache(url, filename string, maxBytes int64, verbose bool, cleanup *installCleanup) (string, error) {
	cacheDir, err := platform.CacheDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache dir: %w", err)
	}

	destPath := filepath.Join(cacheDir, filename)

	// Skip download if already cached.
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		if verbose {
			fmt.Printf("  Using cached archive: %s\n", destPath)
		}
		return destPath, nil
	}

	if verbose {
		fmt.Printf("  Downloading: %s\n", url)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idle := time.AfterFunc(downloadIdleTimeout, cancel)
	defer idle.Stop()
	stalled := func(err error) error {
		if ctx.Err() != nil {
			return fmt.Errorf("no data received for %s: %w", downloadIdleTimeout, err)
		}
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	client := *httpClient
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", stalled(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: %s", errNotFound, url)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(cacheDir, "driftr-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Register temp file for signal-safe cleanup.
	if cleanup != nil {
		cleanup.setTmpFile(tmpPath)
	}

	limited := io.LimitReader(&idleTimeoutReader{r: resp.Body, timer: idle}, maxBytes)
	if ioutil.IsTerminal(os.Stderr) {
		pw := &ioutil.ProgressWriter{Dest: tmpFile, Total: resp.ContentLength}
		_, err = io.Copy(pw, limited)
		pw.Finish()
	} else {
		_, err = io.Copy(tmpFile, limited)
	}
	// A failed close means a truncated file; it must not be renamed into the
	// cache, where it would be treated as a valid archive.
	if cerr := tmpFile.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmpPath)
		if cleanup != nil {
			cleanup.clearTmpFile()
		}
		return "", fmt.Errorf("download interrupted: %w", stalled(err))
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		if cleanup != nil {
			cleanup.clearTmpFile()
		}
		return "", fmt.Errorf("failed to save archive: %w", err)
	}

	// Temp file has been renamed to final path — no longer needs cleanup.
	if cleanup != nil {
		cleanup.clearTmpFile()
	}

	return destPath, nil
}
