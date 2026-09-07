package resolver

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/version"
)

// Pin is a tool version pinned by project configuration.
type Pin struct {
	Tool    string
	Version string
	Source  Source
	Dir     string // directory the pin was found in
}

// installableTools are the tools `driftr install` with no argument acts on,
// in the order they are reported.
var installableTools = []string{"node", "pnpm", "yarn", "bun"}

// ProjectPins collects the tool versions pinned by the project containing dir.
// It walks up the directory tree exactly like resolveFromProject, but does not
// require the versions to be installed — that is the point: the result feeds
// the installer.
//
// Per tool the first source found wins; the walk continues for tools that are
// still unpinned, so a repo may pin node in .nvmrc and pnpm in packageManager
// at different levels.
func ProjectPins(dir string) ([]Pin, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	found := make(map[string]Pin, len(installableTools))
	current := absDir
	for depth := 0; depth < maxResolveDepth && len(found) < len(installableTools); depth++ {
		cfg, err := config.LoadProject(current)
		if err != nil {
			return nil, err
		}
		pkg, err := config.LoadPackageJSON(current)
		if err != nil {
			return nil, err
		}

		for _, tool := range installableTools {
			if _, ok := found[tool]; ok {
				continue
			}
			ver, source, ok, err := pinAt(tool, current, cfg, pkg)
			if err != nil {
				return nil, err
			}
			if ok {
				found[tool] = Pin{Tool: tool, Version: ver, Source: source, Dir: current}
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	pins := make([]Pin, 0, len(found))
	for _, tool := range installableTools {
		if p, ok := found[tool]; ok {
			pins = append(pins, p)
		}
	}
	return pins, nil
}

// ResolvePin returns the installed version a pin selects, using the same
// matching the resolver applies on the shim path: exact versions, partial specs
// like "24", and LTS aliases. ok is false when the spec is unresolvable or no
// installed version satisfies it, which means the pin protects nothing.
func ResolvePin(tool, spec string) (string, bool, error) {
	v, err := version.Parse(spec)
	if err != nil {
		return "", false, err
	}
	match, known := installedMatcher(v)
	if !known {
		return "", false, nil
	}
	best, found, err := newestInstalledMatching(tool, match)
	if err != nil || !found {
		return "", false, err
	}
	return best.String(), true, nil
}

// pinAt returns the version pinned for a tool in a single directory, checking
// each source in resolution order. Adding a source means adding one block here.
func pinAt(tool, dir string, cfg *config.ProjectConfig, pkg *config.PackageJSON) (string, Source, bool, error) {
	if cfg != nil {
		if ver := cfg.Tools.GetTool(tool); ver != "" {
			return ver, SourceProject, true, nil
		}
	}

	if pkg != nil {
		if ver := pkg.Driftr.GetTool(tool); ver != "" {
			return ver, SourcePackageJSON, true, nil
		}
		if usesPackageManagerField(tool) {
			if pmTool, pmVer := pkg.PackageManagerTool(); pmTool == tool && pmVer != "" {
				return pmVer, SourcePackageManager, true, nil
			}
		}
	}

	if tool == "node" {
		// LTS aliases come back verbatim (e.g. "lts/*", "lts/iron"); the
		// installer resolves them against the release index. An unreadable or
		// empty file yields "" and is skipped.
		ver, err := config.LoadNvmrc(dir)
		if err != nil {
			return "", 0, false, err
		}
		if ver != "" {
			return ver, SourceNvmrc, true, nil
		}
		ver, err = config.LoadNodeVersion(dir)
		if err != nil {
			return "", 0, false, err
		}
		if ver != "" {
			return ver, SourceNodeVersion, true, nil
		}

		// engines.node holds a range, not a version. Install the lower-bound
		// major, the oldest release the project accepts. Anything newer that
		// is already installed still satisfies the range.
		if pkg != nil {
			if rangeText := pkg.EnginesNode(); rangeText != "" {
				rng, err := version.ParseRange(rangeText)
				if err != nil {
					return "", 0, false, fmt.Errorf("cannot use engines.node from %s: %w", filepath.Join(dir, "package.json"), err)
				}
				if major, ok := rng.LowerBoundMajor(); ok {
					return strconv.Itoa(major), SourceEnginesNode, true, nil
				}
				return "lts", SourceEnginesNode, true, nil
			}
		}
	}

	return "", 0, false, nil
}
