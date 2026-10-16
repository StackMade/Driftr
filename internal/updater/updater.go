package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/version"
)

const (
	repo                    = "stackmade/driftr"
	defaultAPIBaseURL       = "https://api.github.com/repos/" + repo
	defaultReleaseBaseURL   = "https://github.com/" + repo + "/releases/download"
	maxUpdaterDownloadBytes = 50 * 1024 * 1024 // 50 MB
)

// apiBase returns the release index base URL. DRIFTR_UPDATE_API overrides the
// default — for mirrors and hermetic tests.
func apiBase() string {
	if m := os.Getenv("DRIFTR_UPDATE_API"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultAPIBaseURL
}

// releaseBase returns the release asset base URL, to which "/v<version>" is
// appended. DRIFTR_UPDATE_MIRROR overrides the default — for mirrors and
// hermetic tests.
func releaseBase() string {
	if m := os.Getenv("DRIFTR_UPDATE_MIRROR"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultReleaseBaseURL
}

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
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

// githubRelease represents the relevant fields from the GitHub releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// Update checks for a newer version and replaces the current binary.
// Returns the new version string, or empty string if already up to date.
func Update(currentVersion string, verbose bool) (string, error) {
	// Only a release build can be compared with a release: replacing a dev
	// build would silently swap it for whatever was last published.
	current, ok := releaseVersion(currentVersion)
	if !ok {
		return "", fmt.Errorf("driftr %s is not a release build, so self-update cannot tell whether a release is newer. Install a release with `curl -fsSL https://raw.githubusercontent.com/StackMade/driftr/main/install.sh | sh`", currentVersion)
	}

	fmt.Println("Checking for updates...")

	latest, err := fetchLatestVersion()
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}

	if verbose {
		fmt.Printf("  Current: %s, Latest: %s\n", currentVersion, latest)
	}

	newest, ok := releaseVersion(latest)
	if !ok {
		return "", fmt.Errorf("latest release tag %q is not a version", latest)
	}
	// Equal or older: never downgrade a build that is ahead of the release.
	if !isNewer(newest, current) {
		return "", nil
	}

	fmt.Printf("Updating driftr v%s → v%s...\n", currentVersion, latest)

	archiveName := assetName(latest)
	baseURL := fmt.Sprintf("%s/v%s", releaseBase(), latest)

	tmpDir, err := os.MkdirTemp("", "driftr-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download archive.
	archivePath := filepath.Join(tmpDir, archiveName)
	fmt.Printf("Downloading %s...\n", archiveName)
	if err := downloadFile(baseURL+"/"+archiveName, archivePath); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Download and verify checksum.
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(baseURL+"/checksums.txt", checksumsPath); err != nil {
		return "", fmt.Errorf("failed to download checksums: %w", err)
	}

	fmt.Println("Verifying checksum...")
	if err := verifyChecksum(archivePath, checksumsPath); err != nil {
		return "", err
	}

	// Extract the driftr binary from the archive.
	newBinary := filepath.Join(tmpDir, "driftr")
	if err := extractBinary(archivePath, newBinary); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	// Replace the current binary.
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve executable path: %w", err)
	}

	if err := replaceBinary(newBinary, execPath); err != nil {
		return "", fmt.Errorf("failed to replace binary: %w", err)
	}

	return latest, nil
}

// assetName returns the name of the release archive for this platform.
func assetName(version string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("driftr_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

func fetchLatestVersion() (string, error) {
	resp, err := httpClient.Get(apiBase() + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// downloadIdleTimeout aborts a release download that receives no data for
// this long. httpClient's Timeout would count the body read too and cut off a
// slow but steady connection.
var downloadIdleTimeout = 60 * time.Second

// idleReader pushes the idle deadline back on every read that returns data.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(downloadIdleTimeout)
	}
	return n, err
}

func downloadFile(url, dest string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idle := time.AfterFunc(downloadIdleTimeout, cancel)
	defer idle.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := *httpClient
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	limited := io.LimitReader(&idleReader{r: resp.Body, timer: idle}, maxUpdaterDownloadBytes)
	if ioutil.IsTerminal(os.Stderr) {
		pw := &ioutil.ProgressWriter{Dest: f, Total: resp.ContentLength}
		_, err = io.Copy(pw, limited)
		pw.Finish()
	} else {
		_, err = io.Copy(f, limited)
	}
	// Close errors matter here: a failed flush means a truncated download.
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func verifyChecksum(archivePath, checksumsPath string) error {
	archiveName := filepath.Base(archivePath)

	// Read checksums file.
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("failed to read checksums: %w", err)
	}

	var expected string
	for _, line := range strings.Split(string(data), "\n") {
		// Format: "<hash>  <filename>"
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[1]) == archiveName {
			expected = strings.TrimSpace(parts[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum found for %s", archiveName)
	}

	// Compute actual checksum.
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func extractBinary(archivePath, destPath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipBinary(archivePath, destPath)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if filepath.Base(hdr.Name) == "driftr" && hdr.Typeflag == tar.TypeReg {
			return writeBinary(destPath, tr)
		}
	}
	return fmt.Errorf("driftr binary not found in archive")
}

// extractZipBinary copies driftr.exe out of a Windows release zip.
func extractZipBinary(archivePath, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if path.Base(f.Name) != "driftr.exe" || !f.Mode().IsRegular() {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		return writeBinary(destPath, src)
	}
	return fmt.Errorf("driftr binary not found in archive")
}

func writeBinary(destPath string, r io.Reader) error {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	// A failed close means a truncated binary on disk.
	return out.Close()
}

// replaceBinary puts the binary at newPath in place of oldPath. Windows will
// not overwrite a running .exe but lets it be renamed, so there the old one
// is moved to oldPath.old first; the next update deletes that copy.
func replaceBinary(newPath, oldPath string) error {
	if runtime.GOOS != "windows" {
		return swapIn(newPath, oldPath)
	}
	aside := oldPath + ".old"
	os.Remove(aside)
	if err := os.Rename(oldPath, aside); err != nil {
		return fmt.Errorf("cannot move the running binary aside: %w", err)
	}
	if err := swapIn(newPath, oldPath); err != nil {
		if restoreErr := os.Rename(aside, oldPath); restoreErr != nil {
			return fmt.Errorf("%w; restoring %s from %s also failed: %w", err, oldPath, aside, restoreErr)
		}
		return err
	}
	return nil
}

func swapIn(newPath, oldPath string) error {
	if err := os.Rename(newPath, oldPath); err != nil {
		tmpPath := oldPath + ".driftr-update"
		if err2 := ioutil.CopyFile(newPath, tmpPath); err2 != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename failed (%w); fallback copy also failed: %w", err, err2)
		}
		if err2 := os.Rename(tmpPath, oldPath); err2 != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename failed (%w); atomic replace also failed: %w", err, err2)
		}
		return nil
	}
	return nil
}

// releaseVersion parses an exact X.Y.Z version; ok is false for anything else,
// such as "dev" or a snapshot build.
func releaseVersion(s string) (version.Version, bool) {
	v, err := version.Parse(s)
	if err != nil || v.Latest || v.LTS || v.IsPartial() {
		return version.Version{}, false
	}
	return v, true
}

// isNewer reports whether a is a strictly higher version than b.
func isNewer(a, b version.Version) bool {
	if a.Major != b.Major {
		return a.Major > b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	return a.Patch > b.Patch
}
