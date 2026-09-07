package installer

import (
	"cmp"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/stackmade/driftr/internal/version"
)

const defaultRegistryBaseURL = "https://registry.npmjs.org"

// registryBase returns the npm registry base URL. DRIFTR_NPM_REGISTRY
// overrides the default — for corporate mirrors and hermetic tests.
func registryBase() string {
	if m := os.Getenv("DRIFTR_NPM_REGISTRY"); m != "" {
		return strings.TrimRight(m, "/")
	}
	return defaultRegistryBaseURL
}

const maxRegistryDownloadBytes = 50 * 1024 * 1024 // 50 MB

// registryPackage represents the top-level npm registry response for a package.
type registryPackage struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]registryVersion `json:"versions"`
}

// registryVersion represents a single version from the npm registry.
type registryVersion struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Dist    registryDist      `json:"dist"`
	Bin     map[string]string `json:"bin,omitempty"`
}

// registryDist holds the distribution metadata for an npm package version.
type registryDist struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"` // SRI format: "sha512-<base64>"
	Shasum    string `json:"shasum"`    // sha1 hex (fallback)
}

// FetchRegistryVersion fetches metadata for a specific package version from the npm registry.
func FetchRegistryVersion(pkg, ver string) (*registryVersion, error) {
	url := fmt.Sprintf("%s/%s/%s", registryBase(), pkg, ver)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s@%s from registry: %w", pkg, ver, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s@%s not found in npm registry", pkg, ver)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm registry returned status %d for %s@%s", resp.StatusCode, pkg, ver)
	}

	var rv registryVersion
	if err := json.NewDecoder(resp.Body).Decode(&rv); err != nil {
		return nil, fmt.Errorf("failed to parse registry response for %s@%s: %w", pkg, ver, err)
	}
	return &rv, nil
}

// ResolveRegistryLatest finds the latest version of an npm package matching a partial version spec.
// For "latest" or when no constraint is given, returns the dist-tag "latest".
func ResolveRegistryLatest(pkg string, v version.Version) (string, error) {
	url := fmt.Sprintf("%s/%s", registryBase(), pkg)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s from registry: %w", pkg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned status %d for %s", resp.StatusCode, pkg)
	}

	var rp registryPackage
	if err := json.NewDecoder(resp.Body).Decode(&rp); err != nil {
		return "", fmt.Errorf("failed to parse registry response for %s: %w", pkg, err)
	}

	// For "latest", use the dist-tag.
	if v.Latest {
		latest, ok := rp.DistTags["latest"]
		if !ok {
			return "", fmt.Errorf("no 'latest' dist-tag for %s", pkg)
		}
		return latest, nil
	}

	// Find the highest version matching the partial spec.
	// Collect all matching versions, then pick the highest.
	var best *version.Version
	for verStr := range rp.Versions {
		rv, err := version.Parse(verStr)
		if err != nil {
			continue
		}
		if !v.Matches(rv) {
			continue
		}
		if best == nil || versionCompare(rv, *best) > 0 {
			rv := rv // copy
			best = &rv
		}
	}

	if best == nil {
		return "", fmt.Errorf("no %s version found matching %s", pkg, v.Raw)
	}
	return best.String(), nil
}

// ListRemoteVersions returns available versions of an npm package sorted newest-first.
// Pre-release versions (those containing "-" in the version string) are excluded unless
// includePre is true. Versions that cannot be parsed as semver are silently skipped.
func ListRemoteVersions(pkg string, includePre bool) ([]string, error) {
	url := fmt.Sprintf("%s/%s", registryBase(), pkg)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s from registry: %w", pkg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm registry returned status %d for %s", resp.StatusCode, pkg)
	}

	var rp registryPackage
	if err := json.NewDecoder(resp.Body).Decode(&rp); err != nil {
		return nil, fmt.Errorf("failed to parse registry response for %s: %w", pkg, err)
	}

	type entry struct {
		ver version.Version
		raw string
	}
	var entries []entry

	for verStr := range rp.Versions {
		isPre := strings.Contains(verStr, "-")
		if !includePre && isPre {
			continue
		}
		// Strip pre-release suffix before parsing so version.Parse succeeds
		// (e.g. "9.2.0-rc.0" → parse "9.2.0" for sorting, keep original as raw).
		parseStr := verStr
		if isPre {
			parseStr = verStr[:strings.Index(verStr, "-")]
		}
		v, err := version.Parse(parseStr)
		if err != nil {
			continue
		}
		entries = append(entries, entry{ver: v, raw: verStr})
	}

	sort.Slice(entries, func(i, j int) bool {
		return versionCompare(entries[i].ver, entries[j].ver) > 0
	})

	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.raw
	}
	return result, nil
}

// versionCompare returns a positive value if a > b, negative if a < b, zero if equal.
func versionCompare(a, b version.Version) int {
	if c := cmp.Compare(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Minor, b.Minor); c != 0 {
		return c
	}
	return cmp.Compare(a.Patch, b.Patch)
}

// validateTarballURL ensures a registry-supplied tarball URL points back at
// the configured npm registry. The URL comes from registry metadata, so a
// poisoned response could otherwise send the download to an arbitrary host or
// downgrade it to plaintext HTTP (CheckRedirect only guards redirects, not the
// initial request). With the default registry this enforces HTTPS +
// registry.npmjs.org; a DRIFTR_NPM_REGISTRY override must match itself exactly.
func validateTarballURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid tarball URL in registry metadata: %w", err)
	}
	registry, err := url.Parse(registryBase())
	if err != nil {
		return fmt.Errorf("invalid registry base URL: %w", err)
	}
	if u.Scheme != registry.Scheme {
		return fmt.Errorf("refusing tarball URL with scheme %q (registry uses %q): %q", u.Scheme, registry.Scheme, rawURL)
	}
	// DNS hostnames are case-insensitive.
	if !strings.EqualFold(u.Host, registry.Host) {
		return fmt.Errorf("refusing tarball URL from unexpected host %q (expected %q): %q", u.Host, registry.Host, rawURL)
	}
	return nil
}

// DownloadRegistryPackage downloads an npm package tarball to the cache directory.
// Returns the path to the downloaded file and the registry version metadata.
func DownloadRegistryPackage(pkg, ver string, verbose bool) (string, *registryVersion, error) {
	rv, err := FetchRegistryVersion(pkg, ver)
	if err != nil {
		return "", nil, err
	}

	tarballURL := rv.Dist.Tarball
	if err := validateTarballURL(tarballURL); err != nil {
		return "", nil, err
	}

	filename := fmt.Sprintf("%s-%s.tgz", pkg, ver)
	destPath, err := fetchToCache(tarballURL, filename, maxRegistryDownloadBytes, verbose, nil)
	if err != nil {
		return "", nil, err
	}
	return destPath, rv, nil
}

// VerifyIntegrity verifies a file against an SRI integrity string (e.g. "sha512-<base64>").
func VerifyIntegrity(filePath, integrity string) error {
	algo, expectedHash, err := parseSRI(integrity)
	if err != nil {
		return err
	}

	if algo != "sha512" {
		return fmt.Errorf("unsupported integrity algorithm: %s (expected sha512)", algo)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for integrity check: %w", err)
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	actual := h.Sum(nil)
	if subtle.ConstantTimeCompare(actual, expectedHash) != 1 {
		return fmt.Errorf("integrity mismatch: expected sha512-%s, got sha512-%s",
			base64.StdEncoding.EncodeToString(expectedHash),
			base64.StdEncoding.EncodeToString(actual))
	}

	return nil
}

// parseSRI parses an SRI integrity string like "sha512-<base64>" into algorithm and raw hash bytes.
func parseSRI(sri string) (string, []byte, error) {
	parts := strings.SplitN(sri, "-", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid SRI format: %q", sri)
	}

	algo := parts[0]
	hashBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("invalid base64 in SRI: %w", err)
	}

	return algo, hashBytes, nil
}
