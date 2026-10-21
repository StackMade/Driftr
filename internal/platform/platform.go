package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HomeDir returns the user's home directory: the parent of ~/.driftr and of
// the shell rc files driftr edits.
func HomeDir() (string, error) {
	// On Windows this is USERPROFILE, never HOME: Git Bash and Cygwin may set
	// HOME to another directory, and cmd and PowerShell would then see a
	// different ~/.driftr than the shells that have it.
	return os.UserHomeDir()
}

// DriftrHome returns the root directory for Driftr storage (~/.driftr).
func DriftrHome() (string, error) {
	home, err := HomeDir()
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
	// version comes from pins in files driftr did not write: it must name one
	// directory under tools/<tool>, never climb out of it.
	if version == "" || version == "." || version == ".." || filepath.Base(version) != version {
		return "", fmt.Errorf("invalid %s version %q", tool, version)
	}
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
	if runtime.GOOS == "windows" {
		// Windows has npm.cmd, which only cmd.exe can run, so the shims run
		// the script it wraps under node instead.
		return filepath.Join(dir, "bin", "node_modules", "npm", "bin", tool+"-cli.js"), nil
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
	case "windows":
		return "win"
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
	binary := entry.Binary
	if !entry.NeedsNode {
		binary = ExeName(binary)
	}
	return filepath.Join(dir, "bin", binary), nil
}

// ExeName returns the file name of the native program name: name itself, or
// name.exe on Windows.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// IsExecutable reports whether info describes a file the shell can run.
// Windows has no execute bit: there, the file extension decides.
func IsExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return info.Mode().IsRegular()
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// SamePath reports whether a and b name the same path, ignoring case on
// Windows, whose file system does.
func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
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

// RemoveVersionDir deletes an installed version directory. It first moves the
// directory under a ".tmp-" name, which ListToolVersions skips, so a removal
// that stops halfway never leaves a version that looks installed. On Windows
// the move fails while a program in the directory runs, and then nothing at
// all is removed.
func RemoveVersionDir(dir string) error {
	trash, err := os.MkdirTemp(filepath.Dir(dir), filepath.Base(dir)+".tmp-rm-")
	if err != nil {
		return err
	}
	if err := Rename(dir, filepath.Join(trash, filepath.Base(dir))); err != nil {
		_ = os.Remove(trash)
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%w. Close the programs running from %s and retry", err, dir)
		}
		return err
	}
	if err := os.RemoveAll(trash); err != nil {
		return fmt.Errorf("%w. Manual cleanup: %s", err, RemoveCommand(trash))
	}
	return nil
}

// RemoveCommand returns the shell command that deletes path, for error
// messages: PowerShell's on Windows, where rm -rf does not exist.
func RemoveCommand(path string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Remove-Item -Recurse -Force %q", path)
	}
	return fmt.Sprintf("rm -rf %q", path)
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
			name, ok := packageBinName(e.Name())
			if _, core := toolBinaryMap[name]; !ok || core || e.IsDir() || !IsSafeBinName(name) {
				continue
			}
			bins[name] = append(bins[name], v)
		}
	}
	return bins, nil
}

// packageBinName returns the command a file in a node bin dir provides. On
// Windows, npm writes each bin as name.cmd next to a sh script and a
// PowerShell one, so only the .cmd counts.
func packageBinName(file string) (string, bool) {
	if runtime.GOOS != "windows" {
		return file, true
	}
	ext := filepath.Ext(file)
	if !strings.EqualFold(ext, ".cmd") {
		return "", false
	}
	return strings.TrimSuffix(file, ext), true
}

// PackageBinFile returns the file a global npm install writes for the
// command bin in a node bin dir.
func PackageBinFile(bin string) string {
	if runtime.GOOS == "windows" {
		return bin + ".cmd"
	}
	return bin
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
		// Driftr's own files do not count, under any name a symlink gives
		// them: the shim dir, and node bin dirs a package bin puts on PATH for
		// the commands it runs. Counting a symlinked alias of the shim dir
		// would make the shim exec itself forever.
		if within(abs, home) {
			continue
		}
		for _, file := range commandFiles(name) {
			candidate := filepath.Join(abs, file)
			info, err := os.Stat(candidate)
			if err != nil || !IsExecutable(info) {
				continue
			}
			if real, err := filepath.EvalSymlinks(candidate); err != nil || within(real, realHome) {
				continue
			}
			return candidate
		}
	}
	return ""
}

// commandFiles returns the file names the shell tries for the command name,
// in its order: name itself, or on Windows name plus each PATHEXT extension.
func commandFiles(name string) []string {
	if runtime.GOOS != "windows" {
		return []string{name}
	}
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	var files []string
	for _, ext := range filepath.SplitList(pathext) {
		if ext != "" {
			files = append(files, name+strings.ToLower(ext))
		}
	}
	return files
}

func within(path, dir string) bool {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path, dir = strings.ToLower(path), strings.ToLower(dir)
	}
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
	if runtime.GOOS == "windows" {
		return "zip"
	}
	return "tar.gz"
}
