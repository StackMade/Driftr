package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// LoadNvmrc reads the .nvmrc file from the given directory.
// Returns the version string, or ("", nil) if the file doesn't exist or
// contains an unsupported format (e.g. LTS aliases).
func LoadNvmrc(dir string) (string, error) {
	path := filepath.Join(dir, ".nvmrc")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	return parseVersionFile(string(data))
}

// parseVersionFile extracts a version string from a single-line version file.
// LTS aliases ("lts", "lts/*", "lts/hydrogen") are returned verbatim for the
// resolver to interpret.
func parseVersionFile(content string) (string, error) {
	// Take the first non-empty, non-comment line.
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// LTS aliases pass through untouched — no "v" prefix to strip.
		if strings.HasPrefix(strings.ToLower(line), "lts") {
			return line, nil
		}

		// Strip optional "v" prefix.
		line = strings.TrimPrefix(line, "v")

		return line, nil
	}

	return "", nil
}
