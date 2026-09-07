package resolver

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/version"
)

// RequireInstalled verifies a version string parses correctly and the version is installed.
// For partial versions (e.g. "24", "24.14") and "latest", it finds the best matching
// installed version. Returns the normalized version string and binary path, or an actionable error.
func RequireInstalled(versionSpec string) (string, string, error) {
	return RequireToolInstalled("node", versionSpec)
}

// RequireToolInstalled verifies a tool version is installed, with partial version resolution.
func RequireToolInstalled(tool, versionSpec string) (string, string, error) {
	v, err := version.Parse(versionSpec)
	if err != nil {
		return "", "", fmt.Errorf("invalid version: %w", err)
	}

	if v.Latest || v.IsPartial() {
		return resolveInstalledPartial(tool, v)
	}

	versionStr := v.String()
	binPath, err := requireToolBinaryExists(tool, versionStr, "")
	if err != nil {
		return "", "", err
	}
	return versionStr, binPath, nil
}

// resolveInstalledPartial finds the latest installed version matching a partial spec.
func resolveInstalledPartial(tool string, v version.Version) (string, string, error) {
	best, err := newestInstalled(tool, v.Matches)
	if err != nil {
		return "", "", err
	}
	if best == "" {
		if v.Latest {
			return "", "", fmt.Errorf("no %s versions installed. Run `driftr install %s@<version>`", tool, tool)
		}
		return "", "", fmt.Errorf("no installed %s version matches %s. Run `driftr install %s@%s`", tool, v.Raw, tool, v.Raw)
	}

	binPath, err := requireToolBinaryExists(tool, best, "")
	if err != nil {
		return "", "", err
	}
	return best, binPath, nil
}

// newestInstalled returns the highest installed version of tool accepted by
// match, or "" when nothing matches.
func newestInstalled(tool string, match func(version.Version) bool) (string, error) {
	installed, err := ListToolVersions(tool)
	if err != nil {
		return "", err
	}

	var matches []version.Version
	for _, verStr := range installed {
		iv, err := version.Parse(verStr)
		if err != nil {
			continue
		}
		if match(iv) {
			matches = append(matches, iv)
		}
	}

	if len(matches) == 0 {
		return "", nil
	}

	// Sort descending to pick the latest.
	slices.SortFunc(matches, func(a, b version.Version) int {
		if c := cmp.Compare(b.Major, a.Major); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Minor, a.Minor); c != 0 {
			return c
		}
		return cmp.Compare(b.Patch, a.Patch)
	})

	return matches[0].String(), nil
}

// ListToolVersions returns all installed version strings for a tool.
func ListToolVersions(tool string) ([]string, error) {
	return platform.ListToolVersions(tool)
}

// NotInstalledError is returned when a resolved tool version is not installed locally.
type NotInstalledError struct {
	Tool    string
	Version string
	Context string // e.g. "pinned in /home/user/project" or "global default"
}

func (e *NotInstalledError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("%s %s (%s) is not installed. Run `driftr install %s@%s`", e.Tool, e.Version, e.Context, e.Tool, e.Version)
	}
	return fmt.Sprintf("%s %s is not installed. Run `driftr install %s@%s`", e.Tool, e.Version, e.Tool, e.Version)
}

// requireToolBinaryExists checks that the binary for the given tool and version is installed.
func requireToolBinaryExists(tool, ver, context string) (string, error) {
	binPath, err := platform.ToolBinary(tool, ver)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(binPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &NotInstalledError{Tool: tool, Version: ver, Context: context}
		}
		return "", fmt.Errorf("failed to check %s %s binary: %w", tool, ver, err)
	}
	return binPath, nil
}

// Source describes where a version resolution came from.
type Source int

const (
	SourceExplicit       Source = iota
	SourceProject               // .driftr.toml
	SourcePackageJSON           // package.json driftr key
	SourceNvmrc                 // .nvmrc
	SourceNodeVersion           // .node-version
	SourcePackageManager        // package.json "packageManager" field
	SourceGlobal
)

func (s Source) String() string {
	switch s {
	case SourceExplicit:
		return "explicit override"
	case SourceProject:
		return "project config"
	case SourcePackageJSON:
		return "package.json (driftr)"
	case SourceNvmrc:
		return ".nvmrc"
	case SourceNodeVersion:
		return ".node-version"
	case SourcePackageManager:
		return "package.json (packageManager)"
	case SourceGlobal:
		return "global default"
	default:
		return "unknown"
	}
}

// Resolution holds the result of resolving a tool version.
type Resolution struct {
	Tool       string
	Version    string
	BinaryPath string
	Source     Source
	ProjectDir string // set when Source == SourceProject
}

// ResolveNode determines which Node.js version to use.
func ResolveNode(explicit string) (*Resolution, error) {
	return ResolveTool("node", explicit, false)
}

// ResolveNodeVerbose determines which Node.js version to use, with optional tracing.
func ResolveNodeVerbose(explicit string, verbose bool) (*Resolution, error) {
	return ResolveTool("node", explicit, verbose)
}

// ResolveTool determines which version of a tool to use.
// Resolution order: explicit > project config > global default.
func ResolveTool(tool, explicit string, verbose bool) (*Resolution, error) {
	if verbose {
		fmt.Printf("  [resolve] Starting %s version resolution\n", tool)
	}

	if explicit != "" {
		if verbose {
			fmt.Printf("  [resolve] Step 1: Explicit override provided: %s\n", explicit)
		}
		return resolveExplicit(tool, explicit)
	}
	if verbose {
		fmt.Println("  [resolve] Step 1: No explicit override")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}

	if verbose {
		fmt.Printf("  [resolve] Step 2: Searching for project config from %s\n", cwd)
	}

	res, err := resolveFromProject(tool, cwd, verbose)
	if err != nil {
		return nil, err
	}
	if res != nil {
		if verbose {
			fmt.Printf("  [resolve] Resolved: %s from %s (%s)\n", res.Version, res.Source, res.ProjectDir)
		}
		return res, nil
	}

	if verbose {
		fmt.Println("  [resolve] Step 3: No project config found, checking global default")
	}

	res, err = resolveFromGlobal(tool)
	if err != nil {
		if verbose {
			fmt.Printf("  [resolve] Global default failed: %v\n", err)
		}
		return nil, err
	}

	if verbose {
		fmt.Printf("  [resolve] Resolved: %s from %s\n", res.Version, res.Source)
	}

	return res, nil
}

func resolveExplicit(tool, ver string) (*Resolution, error) {
	binPath, err := requireToolBinaryExists(tool, ver, "")
	if err != nil {
		return nil, err
	}
	return &Resolution{
		Tool:       tool,
		Version:    ver,
		BinaryPath: binPath,
		Source:     SourceExplicit,
	}, nil
}

const maxResolveDepth = 20

func resolveFromProject(tool, dir string, verbose bool) (*Resolution, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	current := absDir
	depth := 0
	for {
		if depth >= maxResolveDepth {
			if verbose {
				fmt.Printf("  [resolve]   Reached max depth (%d), stopping search\n", maxResolveDepth)
			}
			break
		}
		// Check .driftr.toml first.
		cfgPath := filepath.Join(current, config.ProjectConfigFile)
		if verbose {
			fmt.Printf("  [resolve]   Checking: %s\n", cfgPath)
		}

		cfg, err := config.LoadProject(current)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			if ver := cfg.Tools.GetTool(tool); ver != "" {
				return resolveProjectVersion(tool, ver, current, SourceProject)
			}
		}

		// Check package.json driftr key.
		pkgPath := filepath.Join(current, "package.json")
		if verbose {
			fmt.Printf("  [resolve]   Checking: %s (driftr)\n", pkgPath)
		}

		pkg, err := config.LoadPackageJSON(current)
		if err != nil {
			return nil, err
		}
		if pkg != nil {
			if ver := pkg.Driftr.GetTool(tool); ver != "" {
				return resolveProjectVersion(tool, ver, current, SourcePackageJSON)
			}
			// Standard "packageManager" field (corepack/npm convention), e.g.
			// "pnpm@9.15.0". Only meaningful for tools with their own
			// independently pinned version — node's own version isn't
			// expressed this way, and npm/npx always follow node.
			if tool == "pnpm" || tool == "yarn" {
				if pmTool, pmVer := pkg.PackageManagerTool(); pmTool == tool && pmVer != "" {
					return resolveProjectVersion(tool, pmVer, current, SourcePackageManager)
				}
			}
		}

		// Check .nvmrc and .node-version (node only).
		if tool == "node" {
			nvmrcPath := filepath.Join(current, ".nvmrc")
			if verbose {
				fmt.Printf("  [resolve]   Checking: %s\n", nvmrcPath)
			}
			ver, err := config.LoadNvmrc(current)
			if err != nil {
				return nil, err
			}
			if ver != "" {
				res, err := resolveVersionFilePin(tool, ver, current, SourceNvmrc)
				if err != nil {
					return nil, err
				}
				if res != nil {
					return res, nil
				}
			}

			nodeVersionPath := filepath.Join(current, ".node-version")
			if verbose {
				fmt.Printf("  [resolve]   Checking: %s\n", nodeVersionPath)
			}
			ver, err = config.LoadNodeVersion(current)
			if err != nil {
				return nil, err
			}
			if ver != "" {
				res, err := resolveVersionFilePin(tool, ver, current, SourceNodeVersion)
				if err != nil {
					return nil, err
				}
				if res != nil {
					return res, nil
				}
			}
		}

		depth++
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return nil, nil
}

// resolveVersionFilePin resolves a value read from .nvmrc or .node-version,
// which may be an LTS alias ("lts", "lts/*", "lts/iron") rather than a version
// number. Returns (nil, nil) when the alias cannot be resolved at all, so the
// caller keeps walking the config chain.
func resolveVersionFilePin(tool, ver, dir string, source Source) (*Resolution, error) {
	v, err := version.Parse(ver)
	if err != nil || !v.LTS {
		return resolveProjectVersion(tool, ver, dir, source)
	}

	// LTS aliases resolve against installed versions only — the shim hot path
	// must never hit the network.
	match := func(iv version.Version) bool {
		// Node.js LTS lines are the even majors from v4 onwards.
		return iv.Major >= 4 && iv.Major%2 == 0
	}
	alias := "lts"
	if v.LTSCodename != "" {
		major, ok := version.LTSCodenameMajor(v.LTSCodename)
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: %s in %s pins unknown LTS alias %q; ignoring\n", source, dir, ver)
			return nil, nil
		}
		alias = "lts/" + v.LTSCodename
		match = func(iv version.Version) bool { return iv.Major == major }
	}

	best, err := newestInstalled(tool, match)
	if err != nil {
		return nil, err
	}
	if best == "" {
		return nil, &NotInstalledError{Tool: tool, Version: alias, Context: "pinned in " + dir}
	}

	return resolveProjectVersion(tool, best, dir, source)
}

func resolveProjectVersion(tool, ver, dir string, source Source) (*Resolution, error) {
	binPath, err := requireToolBinaryExists(tool, ver, "pinned in "+dir)
	if err != nil {
		return nil, err
	}
	return &Resolution{
		Tool:       tool,
		Version:    ver,
		BinaryPath: binPath,
		Source:     source,
		ProjectDir: dir,
	}, nil
}

func resolveFromGlobal(tool string) (*Resolution, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, err
	}

	ver := cfg.Default.GetTool(tool)
	if ver == "" {
		return nil, fmt.Errorf("no %s version configured. Run `driftr install %s@<version>` and `driftr default %s@<version>`", tool, tool, tool)
	}

	binPath, err := requireToolBinaryExists(tool, ver, "global default")
	if err != nil {
		return nil, err
	}
	return &Resolution{
		Tool:       tool,
		Version:    ver,
		BinaryPath: binPath,
		Source:     SourceGlobal,
	}, nil
}

// toolParent maps tools to the parent tool whose version controls resolution.
// Tools not listed here resolve independently.
var toolParent = map[string]string{
	"npm": "node",
	"npx": "node",
}

// ResolvedBinary holds the result of resolving a tool binary.
type ResolvedBinary struct {
	ToolPath string // path to the tool binary
	NodePath string // path to node binary, set when the tool needs node to execute
}

// ResolveBinary resolves the full path to a tool binary.
// For bundled tools (npm, npx), resolves via the parent tool (node).
// For standalone tools (node, pnpm, yarn), resolves via their own version.
func ResolveBinary(tool string, explicit string) (string, error) {
	rb, err := ResolveBinaryFull(tool, explicit)
	if err != nil {
		return "", err
	}
	return rb.ToolPath, nil
}

// ResolveBinaryFull resolves a tool binary with dual resolution.
// For tools that need Node.js (e.g. yarn), it also resolves the Node binary path.
func ResolveBinaryFull(tool string, explicit string) (*ResolvedBinary, error) {
	resolveTool := tool
	if parent, ok := toolParent[tool]; ok {
		resolveTool = parent
	}

	// Standalone tools that need Node.js (yarn, pnpm) resolve their own
	// version independently of `explicit` — explicit always pins Node.js,
	// per the --node flag it comes from.
	entry, hasEntry := platform.LookupTool(tool)
	standaloneNeedsNode := hasEntry && entry.NeedsNode && resolveTool != "node"

	toolExplicit := explicit
	if standaloneNeedsNode {
		toolExplicit = ""
	}

	res, err := ResolveTool(resolveTool, toolExplicit, false)
	if err != nil {
		return nil, err
	}

	toolPath, err := platform.ToolBinary(tool, res.Version)
	if err != nil {
		return nil, err
	}

	rb := &ResolvedBinary{ToolPath: toolPath}

	if standaloneNeedsNode {
		nodeRes, err := ResolveTool("node", explicit, false)
		if err != nil {
			return nil, fmt.Errorf("%s requires Node.js: %w", tool, err)
		}
		nodePath, err := platform.ToolBinary("node", nodeRes.Version)
		if err != nil {
			return nil, err
		}
		rb.NodePath = nodePath
	}

	return rb, nil
}
