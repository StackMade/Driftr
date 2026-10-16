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
var installableTools = []string{"node", "npm", "pnpm", "yarn", "bun"}

// ProjectPins collects the tool versions pinned by the project containing dir.
// It walks up the directory tree exactly like resolveFromProject, but does not
// require the versions to be installed — that is the point: the result feeds
// the installer.
//
// Per tool the first source found wins; the walk continues for tools that are
// still unpinned, so a repo may pin node in .nvmrc and pnpm in packageManager
// at different levels. engines.node is the exception, as in the resolver: the
// nearest one wins only when no explicit node pin exists anywhere up the tree.
func ProjectPins(dir string) ([]Pin, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	found := make(map[string]Pin, len(installableTools))
	decided := 0 // tools pinned by something a farther directory cannot override
	current := absDir
	for depth := 0; depth < maxResolveDepth && decided < len(installableTools); depth++ {
		cfg, err := config.LoadProject(current)
		if err != nil {
			return nil, err
		}
		pkg, err := config.LoadPackageJSON(current)
		if err != nil {
			return nil, err
		}

		for _, tool := range installableTools {
			prev, seen := found[tool]
			if seen && prev.Source != SourceEnginesNode {
				continue
			}
			ver, source, ok, err := pinAt(tool, current, cfg, pkg)
			if err != nil {
				return nil, err
			}
			if !ok || (seen && source == SourceEnginesNode) {
				continue // the nearest engines.node stays the fallback
			}
			found[tool] = Pin{Tool: tool, Version: ver, Source: source, Dir: current}
			if source != SourceEnginesNode {
				decided++
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
		p, ok := found[tool]
		if !ok {
			continue
		}
		if p.Source == SourceEnginesNode {
			if p, err = enginesPin(p); err != nil {
				return nil, err
			}
		}
		pins = append(pins, p)
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

// ProjectSources lists every source that pins a tool for the project
// containing dir, across the same directory walk the resolver makes. Per tool
// the pins come in resolution order, so the first one is the one that wins;
// the rest are what the resolver never gets to. engines.node is returned as
// its range text, not converted to a version, after every explicit node pin.
func ProjectSources(dir string) ([]Pin, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var pins, engines []Pin
	current := absDir
	for depth := 0; depth < maxResolveDepth; depth++ {
		cfg, err := config.LoadProject(current)
		if err != nil {
			return nil, err
		}
		pkg, err := config.LoadPackageJSON(current)
		if err != nil {
			return nil, err
		}
		for _, tool := range installableTools {
			found, err := pinsAt(tool, current, cfg, pkg, false)
			if err != nil {
				return nil, err
			}
			for _, p := range found {
				if p.Source == SourceEnginesNode {
					engines = append(engines, p)
				} else {
					pins = append(pins, p)
				}
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	// engines.node only counts when no explicit pin exists anywhere up the
	// tree, so it ranks behind every other node source.
	return append(pins, engines...), nil
}

// pinAt returns the version pinned for a tool in a single directory: the first
// source in resolution order. engines.node comes back as its range text.
func pinAt(tool, dir string, cfg *config.ProjectConfig, pkg *config.PackageJSON) (string, Source, bool, error) {
	pins, err := pinsAt(tool, dir, cfg, pkg, true)
	if err != nil || len(pins) == 0 {
		return "", 0, false, err
	}
	return pins[0].Version, pins[0].Source, true, nil
}

// enginesPin turns an engines.node pin into a version to install: the
// lower-bound major, the oldest release the project accepts. Anything newer
// that is already installed still satisfies the range.
func enginesPin(p Pin) (Pin, error) {
	rng, err := version.ParseRange(p.Version)
	if err != nil {
		return Pin{}, fmt.Errorf("cannot use engines.node from %s: %w", filepath.Join(p.Dir, "package.json"), err)
	}
	p.Version = "lts"
	if major, ok := rng.LowerBoundMajor(); ok {
		p.Version = strconv.Itoa(major)
	}
	return p, nil
}

// pinsAt returns every source pinning a tool in a single directory, in
// resolution order. Adding a source means adding one block here.
//
// With firstOnly it stops at the first pin, without reading the files after
// it, so an unreadable .nvmrc behind a valid .driftr.toml fails nothing that
// only needs the winner.
func pinsAt(tool, dir string, cfg *config.ProjectConfig, pkg *config.PackageJSON, firstOnly bool) ([]Pin, error) {
	var pins []Pin
	// add records a pin and reports whether the caller should stop here.
	add := func(ver string, source Source) bool {
		pins = append(pins, Pin{Tool: tool, Version: ver, Source: source, Dir: dir})
		return firstOnly
	}

	if cfg != nil {
		if ver := cfg.Tools.GetTool(tool); ver != "" {
			if add(ver, SourceProject) {
				return pins, nil
			}
		}
	}

	if pkg != nil {
		if ver := pkg.Driftr.GetTool(tool); ver != "" {
			if add(ver, SourcePackageJSON) {
				return pins, nil
			}
		}
		if usesPackageManagerField(tool) {
			if pmTool, pmVer := pkg.PackageManagerTool(); pmTool == tool && pmVer != "" {
				if add(pmVer, SourcePackageManager) {
					return pins, nil
				}
			}
		}
	}

	if tool == "node" {
		// LTS aliases come back verbatim (e.g. "lts/*", "lts/iron"); the
		// installer resolves them against the release index. An unreadable or
		// empty file yields "" and is skipped.
		ver, err := config.LoadNvmrc(dir)
		if err != nil {
			return nil, err
		}
		if ver != "" {
			if add(ver, SourceNvmrc) {
				return pins, nil
			}
		}
		ver, err = config.LoadNodeVersion(dir)
		if err != nil {
			return nil, err
		}
		if ver != "" {
			if add(ver, SourceNodeVersion) {
				return pins, nil
			}
		}
		if pkg != nil {
			if rangeText := pkg.EnginesNode(); rangeText != "" {
				if add(rangeText, SourceEnginesNode) {
					return pins, nil
				}
			}
		}
	}

	return pins, nil
}
