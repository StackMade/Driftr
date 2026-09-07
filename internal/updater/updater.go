package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/stackmade/driftr/internal/ioutil"
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
	fmt.Println("Checking for updates...")

	latest, err := fetchLatestVersion()
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}

	if verbose {
		fmt.Printf("  Current: %s, Latest: %s\n", currentVersion, latest)
	}

	if latest == currentVersion {
		return "", nil
	}

	fmt.Printf("Updating driftr v%s → v%s...\n", currentVersion, latest)

	archiveName := fmt.Sprintf("driftr_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
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

func downloadFile(url, dest string) error {
	resp, err := httpClient.Get(url)
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

	limited := io.LimitReader(resp.Body, maxUpdaterDownloadBytes)
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
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			// A failed close means a truncated binary on disk.
			if err := out.Close(); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("driftr binary not found in archive")
}

func replaceBinary(newPath, oldPath string) error {
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
