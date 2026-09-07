package installer

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ShasumsURL returns the URL for the SHASUMS256.txt file for a given Node.js version.
func ShasumsURL(version string) string {
	return fmt.Sprintf("%s/v%s/SHASUMS256.txt", nodeDistBase(), version)
}

// FetchExpectedChecksum downloads SHASUMS256.txt and extracts the hash for the given filename.
func FetchExpectedChecksum(version, filename string) (string, error) {
	return fetchChecksumLine(ShasumsURL(version), filename)
}

// fetchChecksumLine downloads a SHASUMS256.txt-style listing and returns the
// hash recorded for filename.
func fetchChecksumLine(url, filename string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch checksums: HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "<hash>  <filename>"
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[1]) == filename {
			return strings.TrimSpace(parts[0]), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading checksums: %w", err)
	}

	return "", fmt.Errorf("checksum not found for %s in SHASUMS256.txt", filename)
}

// ComputeFileChecksum returns the SHA256 hex digest of a file.
func ComputeFileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChecksum downloads the expected checksum and compares it against the local file.
func VerifyChecksum(archivePath, version string, verbose bool) error {
	filename := ArchiveFilename(version)

	if verbose {
		fmt.Printf("  Fetching checksums from: %s\n", ShasumsURL(version))
	}

	expected, err := FetchExpectedChecksum(version, filename)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("  Expected SHA256: %s\n", expected)
	}

	actual, err := ComputeFileChecksum(archivePath)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("  Computed SHA256: %s\n", actual)
	}

	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("checksum mismatch for %s\n  expected: %s\n  got:      %s\nThe download may be corrupted. Delete the cached file and try again:\n  rm %s",
			filename, expected, actual, archivePath)
	}

	if verbose {
		fmt.Printf("  Checksum verified OK\n")
	}

	return nil
}
