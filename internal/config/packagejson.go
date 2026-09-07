package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PackageJSON represents the relevant fields from a package.json file.
type PackageJSON struct {
	Driftr         DriftrConfig `json:"driftr"`
	PackageManager string       `json:"packageManager"`
	Engines        Engines      `json:"engines"`
}

// Engines mirrors the standard "engines" object.
type Engines struct {
	Node string `json:"node"`
}

// EnginesNode returns the semver range from "engines".node, or "" when absent.
func (p *PackageJSON) EnginesNode() string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Engines.Node)
}

// PackageManagerTool parses the standard "packageManager" field (e.g.
// "pnpm@9.15.0" or "yarn@1.22.22+sha512.abc..."), used by corepack and npm.
// Returns ("", "") if the field is absent or malformed.
func (p *PackageJSON) PackageManagerTool() (tool, ver string) {
	if p == nil || p.PackageManager == "" {
		return "", ""
	}
	name, rest, ok := strings.Cut(p.PackageManager, "@")
	if !ok || name == "" || rest == "" {
		return "", ""
	}
	ver, _, _ = strings.Cut(rest, "+") // strip the optional integrity hash suffix
	if ver == "" {
		return "", ""
	}
	return name, ver
}

// DriftrConfig holds the Driftr tool pinning from package.json.
// Serialized as a flat JSON map: {"node": "22.14.0", "pnpm": "9.15.0"}
type DriftrConfig map[string]string

// GetTool returns the pinned version for a tool from package.json.
func (dc DriftrConfig) GetTool(tool string) string {
	return dc[tool]
}

// hasVersions returns true if at least one tool has a non-empty version.
func (dc DriftrConfig) hasVersions() bool {
	for _, v := range dc {
		if v != "" {
			return true
		}
	}
	return false
}

// SetTool sets the pinned version for a tool.
func (dc *DriftrConfig) SetTool(tool, version string) {
	if *dc == nil {
		*dc = make(DriftrConfig)
	}
	(*dc)[tool] = version
}

// LoadPackageJSON reads driftr tool versions from package.json in the given directory.
// Returns (nil, nil) if the file doesn't exist or has no driftr key with tool versions.
func LoadPackageJSON(dir string) (*PackageJSON, error) {
	path := filepath.Join(dir, "package.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// Return nil if no tool versions are configured.
	if !pkg.Driftr.hasVersions() && pkg.PackageManager == "" && pkg.EnginesNode() == "" {
		return nil, nil
	}

	return &pkg, nil
}

// SavePackageJSON writes a tool version into the driftr key in package.json.
// Returns an error if package.json does not exist in the directory.
func SavePackageJSON(dir string, version string) error {
	return SavePackageJSONTool(dir, "node", version)
}

// SavePackageJSONTool writes a specific tool version into the driftr key in package.json.
func SavePackageJSONTool(dir, tool, version string) error {
	path := filepath.Join(dir, "package.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no package.json found in %s. Run `npm init` first", dir)
		}
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Load existing driftr config to preserve other tools. A parse failure
	// must abort: writing with an empty config would drop other pinned tools.
	var existing PackageJSON
	if err := json.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("failed to parse %s: %w. Fix the file and retry", path, err)
	}
	existing.Driftr.SetTool(tool, version)

	driftrValue, err := json.Marshal(existing.Driftr)
	if err != nil {
		return err
	}

	out, err := patchTopLevelKey(data, "driftr", driftrValue)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", path, err)
	}

	return os.WriteFile(path, out, 0o644)
}

// RemoveDriftrFromPackageJSON removes the driftr key from package.json.
// Returns nil if package.json doesn't exist or has no driftr key.
func RemoveDriftrFromPackageJSON(dir string) error {
	path := filepath.Join(dir, "package.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Quick check: if "driftr" doesn't appear at all, nothing to do.
	if !bytes.Contains(data, []byte(`"driftr"`)) {
		return nil
	}

	out, err := patchTopLevelKey(data, "driftr", nil)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", path, err)
	}

	return os.WriteFile(path, out, 0o644)
}

// patchTopLevelKey modifies a top-level key in a JSON object, preserving key order.
// If value is non-nil, the key is set (inserted at end if new, replaced in-place if existing).
// If value is nil, the key is removed.
func patchTopLevelKey(data []byte, key string, value json.RawMessage) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if t != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}

	type kvPair struct {
		key string
		raw json.RawMessage
	}

	var pairs []kvPair
	found := false

	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read key: %w", err)
		}
		k, ok := t.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %T", t)
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("failed to read value for %q: %w", k, err)
		}

		if k == key {
			found = true
			if value != nil {
				pairs = append(pairs, kvPair{k, value})
			}
			continue
		}

		pairs = append(pairs, kvPair{k, raw})
	}

	if !found && value != nil {
		pairs = append(pairs, kvPair{key, value})
	}

	indent := detectIndent(data)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, p := range pairs {
		buf.WriteByte('\n')
		buf.WriteString(indent)
		keyBytes, _ := json.Marshal(p.key)
		buf.Write(keyBytes)
		buf.WriteString(": ")
		buf.Write(p.raw)
		if i < len(pairs)-1 {
			buf.WriteByte(',')
		}
	}
	if len(pairs) > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteByte('}')
	buf.WriteByte('\n')

	return buf.Bytes(), nil
}

// detectIndent returns the whitespace used for indentation in a JSON file.
// It looks at the whitespace after the first newline.
func detectIndent(data []byte) string {
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t') {
				j++
			}
			if j > i+1 {
				return string(data[i+1 : j])
			}
			break
		}
	}
	return "  "
}
