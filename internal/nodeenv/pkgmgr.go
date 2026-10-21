package nodeenv

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PackageManager identifies the Node.js package manager a project uses.
type PackageManager string

const (
	PnpmManager PackageManager = "pnpm"
	NpmManager  PackageManager = "npm"
	YarnManager PackageManager = "yarn"
	NoManager   PackageManager = "none"
)

// DetectPackageManager determines the package manager for the project rooted at
// dir. Precedence:
//  1. package.json "packageManager" field (e.g. "pnpm@9.1.0") — authoritative,
//     this is the corepack-standard signal.
//  2. Lockfile presence (pnpm-lock.yaml / yarn.lock / package-lock.json).
//  3. NoManager when nothing indicates a manager.
//
// A directory with neither defers to its parents: a workspace member has no
// lockfile of its own and is installed by the workspace root's manager.
func DetectPackageManager(dir string) PackageManager {
	for {
		if pm := managerIn(dir); pm != NoManager {
			return pm
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return NoManager
		}
		dir = parent
	}
}

// managerIn applies the precedence above to one directory.
func managerIn(dir string) PackageManager {
	if pm := managerFromPackageJSON(filepath.Join(dir, "package.json")); pm != NoManager {
		return pm
	}

	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return PnpmManager
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return YarnManager
	case fileExists(filepath.Join(dir, "package-lock.json")):
		return NpmManager
	}

	return NoManager
}

// managerFromPackageJSON reads the "packageManager" field. Returns NoManager if
// the file is missing, unparsable, or the field is absent/unrecognized — these
// are not errors, they just mean "fall through to lockfile detection".
func managerFromPackageJSON(path string) PackageManager {
	data, err := os.ReadFile(path)
	if err != nil {
		return NoManager
	}

	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &pkg); err != nil {
		return NoManager
	}

	// Field looks like "pnpm@9.1.0"; only the name before "@" matters.
	name := pkg.PackageManager
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name = name[:i]
	}
	switch PackageManager(name) {
	case PnpmManager:
		return PnpmManager
	case YarnManager:
		return YarnManager
	case NpmManager:
		return NpmManager
	default:
		return NoManager
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
