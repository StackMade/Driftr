package installer

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/version"
)

// bun ships as a native binary in a zip on GitHub Releases, not as an npm
// package. Two endpoints are involved: the API lists the releases, the
// download host serves the assets and their SHASUMS256.txt.
const (
	defaultBunReleasesURL  = "https://api.github.com/repos/oven-sh/bun/releases"
	defaultBunDownloadBase = "https://github.com/oven-sh/bun/releases/download"
)

// bunReleasesURL returns the release index URL. DRIFTR_BUN_RELEASES overrides
// the default — for mirrors and hermetic tests.
func bunReleasesURL() string {
	if m := os.Getenv("DRIFTR_BUN_RELEASES"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultBunReleasesURL
}

// bunDownloadBase returns the release asset base URL. DRIFTR_BUN_MIRROR
// overrides the default — for mirrors and hermetic tests.
func bunDownloadBase() string {
	if m := os.Getenv("DRIFTR_BUN_MIRROR"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultBunDownloadBase
}

const maxBunDownloadBytes = 200 * 1024 * 1024 // 200 MB

// bunRelease is one entry of the GitHub releases index.
type bunRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// BunAssetName returns the release asset for the current platform, e.g.
// "bun-darwin-aarch64.zip". bun names architectures differently from Node.js,
// so this does not go through platform.Arch().
func BunAssetName() (string, error) {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "aarch64"
	default:
		return "", fmt.Errorf("bun has no release build for architecture %s", runtime.GOARCH)
	}

	switch runtime.GOOS {
	case "darwin", "linux":
	default:
		return "", fmt.Errorf("bun has no release build for %s", runtime.GOOS)
	}

	return fmt.Sprintf("bun-%s-%s.zip", runtime.GOOS, arch), nil
}

// ListBunReleases returns released bun versions, newest first. Drafts and
// pre-releases are skipped, as are tags that are not plain versions (canary
// builds and the like).
func ListBunReleases() ([]string, error) {
	resp, err := httpClient.Get(bunReleasesURL() + "?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bun release list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch bun release list: HTTP %d", resp.StatusCode)
	}

	var releases []bunRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse bun release list: %w", err)
	}

	var versions []string
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		ver, ok := strings.CutPrefix(rel.TagName, "bun-v")
		if !ok {
			continue
		}
		v, err := version.Parse(ver)
		if err != nil || v.IsPartial() {
			continue
		}
		versions = append(versions, v.String())
	}
	return versions, nil
}

// resolveBunVersion picks the newest released version matching a partial spec
// or "latest". The release list is newest-first, so the first match wins.
func resolveBunVersion(v version.Version) (string, error) {
	versions, err := ListBunReleases()
	if err != nil {
		return "", err
	}

	for _, verStr := range versions {
		rv, err := version.Parse(verStr)
		if err != nil {
			continue
		}
		if v.Latest || v.Matches(rv) {
			return rv.String(), nil
		}
	}

	if v.Latest {
		return "", errors.New("no bun releases found")
	}
	return "", fmt.Errorf("no bun release found matching %s", v.Raw)
}

// InstallBun downloads and installs a bun version from GitHub Releases.
// The asset is verified against the SHASUMS256.txt published with the release;
// a release without one is refused rather than installed unverified.
func InstallBun(versionStr string, verbose bool) (string, error) {
	if err := platform.EnsureToolDirs("bun"); err != nil {
		return "", err
	}

	v, err := version.Parse(versionStr)
	if err != nil {
		return "", fmt.Errorf("invalid version: %w", err)
	}
	if v.LTS {
		return "", fmt.Errorf("lts is only supported for node (driftr install node@lts); bun has no LTS concept, use a version like bun@1")
	}

	asset, err := BunAssetName()
	if err != nil {
		return "", err
	}

	resolvedVersion := v.String()
	if v.Latest || v.IsPartial() {
		resolved, err := resolveBunVersion(v)
		if err != nil {
			return "", err
		}
		resolvedVersion = resolved
		if verbose {
			fmt.Printf("  Resolved %s to %s\n", versionStr, resolvedVersion)
		}
	}

	// Check if already installed.
	binPath, err := platform.ToolBinary("bun", resolvedVersion)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(binPath); err == nil {
		if verbose {
			fmt.Printf("  bun %s is already installed\n", resolvedVersion)
		}
		return resolvedVersion, nil
	}

	tag := "bun-v" + resolvedVersion
	assetURL := fmt.Sprintf("%s/%s/%s", bunDownloadBase(), tag, asset)
	cacheName := fmt.Sprintf("bun-v%s-%s", resolvedVersion, asset)

	archivePath, err := fetchToCache(assetURL, cacheName, maxBunDownloadBytes, verbose, nil)
	if errors.Is(err, errNotFound) {
		return "", fmt.Errorf("bun %s has no %s build (%s). Run `driftr list --remote bun` to see available versions", resolvedVersion, asset, assetURL)
	}
	if err != nil {
		return "", err
	}

	if err := verifyBunChecksum(archivePath, tag, asset, verbose); err != nil {
		return "", removeCorruptArchive(archivePath, err)
	}

	versionDir, err := platform.ToolVersionDir("bun", resolvedVersion)
	if err != nil {
		return "", err
	}
	if verbose {
		fmt.Printf("  Extracting to: %s\n", versionDir)
	}
	// The archive holds a single top-level directory named after the asset,
	// e.g. "bun-linux-x64/bun".
	prefix := strings.TrimSuffix(asset, ".zip") + "/"
	if err := ExtractZipToBin(archivePath, versionDir, prefix, "bun"); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	// Ensure the binary is executable.
	if err := os.Chmod(binPath, 0o755); err != nil {
		return "", err
	}

	return resolvedVersion, nil
}

// verifyBunChecksum checks the downloaded asset against the SHASUMS256.txt
// published alongside the release.
func verifyBunChecksum(archivePath, tag, asset string, verbose bool) error {
	url := fmt.Sprintf("%s/%s/SHASUMS256.txt", bunDownloadBase(), tag)
	if verbose {
		fmt.Printf("  Fetching checksums from: %s\n", url)
	}

	expected, err := fetchChecksumLine(url, asset)
	if err != nil {
		return fmt.Errorf("cannot verify bun %s: %w; refusing to install unverified package", tag, err)
	}

	actual, err := ComputeFileChecksum(archivePath)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("checksum mismatch for %s\n  expected: %s\n  got:      %s\nThe download may be corrupted. Delete the cached file and try again:\n  rm %s",
			asset, expected, actual, archivePath)
	}

	if verbose {
		fmt.Println("  Checksum verified OK")
	}
	return nil
}
