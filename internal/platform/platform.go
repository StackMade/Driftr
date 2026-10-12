package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DriftrHome returns the root directory for Driftr storage (~/.driftr).
func DriftrHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".driftr"), nil
}

// BinDir returns the shim directory (~/.driftr/bin).
func BinDir() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "bin"), nil
}

// ToolsDir returns the tools root (~/.driftr/tools).
func ToolsDir() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "tools"), nil
}

// ToolVersionDir returns the directory for a specific tool and version.
func ToolVersionDir(tool, version string) (string, error) {
	tools, err := ToolsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(tools, tool, version), nil
}

// NodeVersionDir returns the directory for a specific Node version.
func NodeVersionDir(version string) (string, error) {
	return ToolVersionDir("node", version)
}

// NodeBinary returns the path to the node binary for a given version.
func NodeBinary(version string) (string, error) {
	return ToolBinary("node", version)
}

// BundledNpmBinary returns the npm or npx that ships inside a Node.js
// version, which is what the npm and npx shims run when nothing pins npm.
func BundledNpmBinary(tool, nodeVersion string) (string, error) {
	dir, err := NodeVersionDir(nodeVersion)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin", tool), nil
}

// CacheDir returns the cache directory (~/.driftr/cache).
func CacheDir() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "cache"), nil
}

// StoresDir returns the shared package-store root (~/.driftr/stores).
func StoresDir() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "stores"), nil
}

// PnpmStoreDir returns the pnpm content-addressable store directory
// (~/.driftr/stores/pnpm) that driftr configures pnpm to use.
func PnpmStoreDir() (string, error) {
	stores, err := StoresDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stores, "pnpm"), nil
}

// GlobalConfigPath returns the path to the global config file.
func GlobalConfigPath() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config", "config.toml"), nil
}

// ProjectsRegistryPath returns the path to the list of known project
// directories (~/.driftr/config/projects), one absolute path per line.
func ProjectsRegistryPath() (string, error) {
	home, err := DriftrHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config", "projects"), nil
}

// EnsureDirs creates all required Driftr directories.
func EnsureDirs() error {
	return EnsureToolDirs("node")
}

// EnsureToolDirs creates all required Driftr directories including tool-specific ones.
func EnsureToolDirs(tools ...string) error {
	home, err := DriftrHome()
	if err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(home, "bin"),
		filepath.Join(home, "config"),
		filepath.Join(home, "cache"),
	}

	for _, tool := range tools {
		dirs = append(dirs, filepath.Join(home, "tools", tool))
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	return nil
}

// Arch returns the architecture string used by Node.js distribution.
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}

// OS returns the OS string used by Node.js distribution.
func OS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

// ToolEntry describes how to find and execute a tool binary.
type ToolEntry struct {
	Parent    string // which tool's version dir contains this binary
	Binary    string // binary name under bin/
	NeedsNode bool   // true if the binary is a JS script requiring node to execute
}

// toolBinaryMap maps tool names to their parent tool and binary info.
var toolBinaryMap = map[string]ToolEntry{
	"node": {Parent: "node", Binary: "node"},
	// npm installed by driftr from the registry. When nothing pins npm, the
	// npm and npx shims run the copy bundled with node instead.
	"npm":  {Parent: "npm", Binary: "npm-cli.js", NeedsNode: true},
	"npx":  {Parent: "npm", Binary: "npx-cli.js", NeedsNode: true},
	"pnpm": {Parent: "pnpm", Binary: "pnpm.cjs", NeedsNode: true},
	"pnpx": {Parent: "pnpm", Binary: "pnpx.cjs", NeedsNode: true},
	"yarn": {Parent: "yarn", Binary: "yarn.js", NeedsNode: true},
	// bun is a native binary, so it runs without node.
	"bun": {Parent: "bun", Binary: "bun"},
}

// LookupTool returns the ToolEntry for a tool, or false if unknown.
func LookupTool(tool string) (ToolEntry, bool) {
	entry, ok := toolBinaryMap[tool]
	return entry, ok
}

// ToolBinary returns the binary path for a given tool and version.
// For tools that ship inside another (npx, pnpx), the version is the parent's.
func ToolBinary(tool, version string) (string, error) {
	entry, ok := toolBinaryMap[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tool)
	}
	dir, err := ToolVersionDir(entry.Parent, version)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin", entry.Binary), nil
}

// ListToolVersions returns all installed version strings for a tool.
func ListToolVersions(tool string) ([]string, error) {
	toolsDir, err := ToolsDir()
	if err != nil {
		return nil, err
	}

	toolDir := filepath.Join(toolsDir, tool)
	entries, err := os.ReadDir(toolDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s versions: %w", tool, err)
	}

	var versions []string
	for _, e := range entries {
		// Skip in-progress or stale extraction work dirs (<version>.tmp-XXXX).
		if e.IsDir() && !strings.Contains(e.Name(), ".tmp-") {
			versions = append(versions, e.Name())
		}
	}
	return versions, nil
}

// NodePackageBins maps each executable that a global npm install put in a
// Node.js version's bin dir (e.g. tsc after `npm install -g typescript`) to
// the node versions that have it. Driftr's own tools are left out, since they
// already have shims, and so are names unsafe to write into a shim script.
func NodePackageBins() (map[string][]string, error) {
	versions, err := ListToolVersions("node")
	if err != nil {
		return nil, err
	}

	bins := map[string][]string{}
	for _, v := range versions {
		dir, err := ToolVersionDir("node", v)
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(filepath.Join(dir, "bin"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to read node %s bin dir: %w", v, err)
		}
		for _, e := range entries {
			name := e.Name()
			if _, core := toolBinaryMap[name]; core || e.IsDir() || !IsSafeBinName(name) {
				continue
			}
			bins[name] = append(bins[name], v)
		}
	}
	return bins, nil
}

// FindOutsideDriftr returns the first executable called name on PATH that is
// not under ~/.driftr, or "". It is what the shell would run if driftr's shim
// dir were not on PATH, so it searches PATH the way the shell does: an empty
// or relative entry is taken relative to the current directory.
func FindOutsideDriftr(name string) string {
	home, err := DriftrHome()
	if err != nil {
		return ""
	}
	realHome := home
	if r, err := filepath.EvalSymlinks(home); err == nil {
		realHome = r
	}
	dirs := filepath.SplitList(os.Getenv("PATH"))
	if path, set := os.LookupEnv("PATH"); set && path == "" {
		dirs = []string{""} // a set but empty PATH is one empty entry: the current directory
	}
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		candidate := filepath.Join(abs, name)
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		// Driftr's own files do not count, under any name a symlink gives
		// them: the shim dir, and node bin dirs a package bin puts on PATH for
		// the commands it runs. Counting a symlinked alias of the shim dir
		// would make the shim exec itself forever.
		if within(abs, home) {
			continue
		}
		if real, err := filepath.EvalSymlinks(candidate); err != nil || within(real, realHome) {
			continue
		}
		return candidate
	}
	return ""
}

func within(path, dir string) bool {
	path = filepath.Clean(path)
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// IsSafeBinName reports whether name can become a shim file name and a shell
// word without quoting. npm package authors choose bin names, so anything
// outside this set is skipped rather than escaped.
func IsSafeBinName(name string) bool {
	if name == "" || name[0] == '.' || name[0] == '-' {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-', r == '+', r == '@':
		default:
			return false
		}
	}
	return true
}

// ArchiveExt returns the archive extension for the current platform.
func ArchiveExt() string {
	return "tar.gz"
}
