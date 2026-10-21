package config

import (
	"errors"
	"fmt"
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
// resolver to interpret; nvm's "node" and "stable" come back as "latest".
func parseVersionFile(content string) (string, error) {
	// Take the first line that holds a version, not just a comment.
	for _, line := range strings.Split(content, "\n") {
		line = versionToken(line)
		if line == "" {
			continue
		}

		// nvm's "node" and "stable" mean the newest release: driftr's "latest".
		switch strings.ToLower(line) {
		case "node", "stable":
			return "latest", nil
		case "system":
			// nvm's "system" is the Node.js outside nvm: no pin for driftr.
			return "", nil
		}

		// LTS aliases pass through untouched — no "v" prefix to strip.
		if strings.HasPrefix(strings.ToLower(line), "lts") {
			return line, nil
		}

		// Strip optional "v" prefix. Trim again afterwards: "v 22.14.0" would
		// otherwise carry the space through, since the earlier TrimSpace ran
		// before the prefix came off.
		return strings.TrimSpace(strings.TrimPrefix(line, "v")), nil
	}

	return "", nil
}

// versionToken returns the version a line of a version file holds: the line
// without a UTF-8 byte order mark, a trailing "# comment" or surrounding
// space. "" means the line holds no version.
func versionToken(line string) string {
	line, _, _ = strings.Cut(strings.TrimPrefix(line, "\ufeff"), "#")
	return strings.TrimSpace(line)
}

// SaveVersionFile replaces the version in a single-line version file such as
// .nvmrc or .node-version. Only the version token changes: the line it sits
// on, comments, blank lines, line endings and a leading "v" stay as they were,
// so the diff a user commits is exactly the version.
func SaveVersionFile(path, ver string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	// Same line parseVersionFile reads: the first one holding a version.
	for i, line := range lines {
		token := versionToken(line)
		if token == "" {
			continue
		}
		start := strings.Index(line, token)
		prefix := ""
		if strings.HasPrefix(token, "v") {
			prefix = "v"
		}
		lines[i] = line[:start] + prefix + ver + line[start+len(token):]
		return replaceFile(path, []byte(strings.Join(lines, "\n")))
	}

	return fmt.Errorf("%s holds no version to replace", path)
}
