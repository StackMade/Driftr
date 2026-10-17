package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/resolver"
	"github.com/stackmade/driftr/v2/internal/version"
)

// runPinBump moves the current project's pins to newer upstream releases, in
// the file that holds each winning pin. tools narrows it; empty means every
// pinned tool. Nothing is installed.
func runPinBump(w io.Writer, tools []string, allowMajor, dryRun bool) error {
	for _, t := range tools {
		if !slices.Contains(versionedTools, t) {
			return fmt.Errorf("unknown tool %q (known: %s). --bump takes tool names, e.g. `driftr pin --bump node`", t, strings.Join(versionedTools, ", "))
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine current directory: %w", err)
	}
	all, err := resolver.ProjectPins(cwd)
	if err != nil {
		return err
	}

	var pins []resolver.Pin
	for _, p := range all {
		if len(tools) == 0 || slices.Contains(tools, p.Tool) {
			pins = append(pins, p)
		}
	}
	for _, t := range tools {
		if !slices.ContainsFunc(pins, func(p resolver.Pin) bool { return p.Tool == t }) {
			fmt.Fprintf(w, "%s: not pinned in this project, skipped\n", t)
		}
	}
	if len(pins) == 0 {
		fmt.Fprintln(w, "Nothing to bump — no pins found in this project.")
		fmt.Fprintln(w, "Run `driftr pin node@<version>` to pin one.")
		return nil
	}

	changed, failed := 0, 0
	for _, p := range pins {
		file := pinFile(p)
		shown := file
		if rel, err := filepath.Rel(cwd, file); err == nil {
			shown = rel
		}

		versions, _, err := upstreamReleases(p.Tool, false)
		if err != nil {
			fmt.Fprintln(w, ioutil.Warn(fmt.Sprintf("%s: cannot check upstream: %v", p.Tool, err)))
			failed++
			continue
		}

		switch p.Source {
		case resolver.SourceEnginesNode:
			// A range owned by the package author, not a pin Driftr may move.
			newest := "unknown"
			if v, ok := newestRelease(versions, -1); ok {
				newest = v.String()
			}
			fmt.Fprintf(w, "%s: engines.node in %s is a range, not rewritten. Newest release is %s; raise the range by hand if the project supports it.\n",
				p.Tool, shown, newest)
			continue
		case resolver.SourcePackageManager:
			// Corepack format, which may carry an integrity hash Driftr cannot compute.
			to, note := bumpTarget(p.Version, versions, allowMajor)
			if to == "" {
				fmt.Fprintf(w, "%s: %s from packageManager in %s, %s\n", p.Tool, p.Version, shown, note)
				continue
			}
			fmt.Fprintf(w, "%s: packageManager in %s is not rewritten (corepack format, may carry a hash). Newer: %s@%s. Run `corepack use %s@%s` or edit it by hand.\n",
				p.Tool, shown, p.Tool, to, p.Tool, to)
			continue
		}

		to, note := bumpTarget(p.Version, versions, allowMajor)
		if to == "" {
			fmt.Fprintf(w, "%s: %s in %s, %s\n", p.Tool, p.Version, shown, note)
			continue
		}
		if !dryRun {
			if err := writeBump(p, to); err != nil {
				return fmt.Errorf("cannot bump %s in %s: %w", p.Tool, file, err)
			}
		}
		fmt.Fprintf(w, "%s: %s -> %s in %s\n", p.Tool, p.Version, ioutil.Bold(to), shown)
		changed++
	}
	if changed > 0 && !dryRun {
		recordProject(all)
	}

	switch {
	case changed > 0 && dryRun:
		fmt.Fprintf(w, "Dry run: %d pin(s) would change, nothing written.\n", changed)
	case changed > 0:
		fmt.Fprintln(w, ioutil.Success(fmt.Sprintf("Bumped %d pin(s). Run `driftr install` to install the new versions.", changed)))
	case failed == 0:
		fmt.Fprintln(w, "Nothing to bump.")
	}
	if failed > 0 {
		return fmt.Errorf("could not check %d tool(s) upstream. Check your network and rerun `driftr pin --bump`", failed)
	}
	return nil
}

// bumpTarget returns the version a pin moves to, or "" plus the reason it
// stays put. versions are upstream releases in any order.
//
// The pin keeps its precision: "22.14.0" moves to the newest 22.x.y, "22.14"
// to the newest 22.x, and "22" stays "22" because it already floats over the
// whole line. allowMajor lifts the same-major limit, so "22" can become "24".
// Pre-releases are never a target, a pin never moves backwards, and aliases
// such as "lts/iron" or "latest" float on their own and are left alone. A
// leading "v" survives.
func bumpTarget(pin string, versions []string, allowMajor bool) (string, string) {
	cv, err := version.Parse(pin)
	if err != nil {
		return "", "cannot parse the pin, not bumped"
	}
	if cv.Latest || cv.LTS {
		return "", "an alias that already floats, not bumped"
	}

	major := cv.Major
	if allowMajor {
		major = -1
	}
	best, ok := newestRelease(versions, major)
	if !ok {
		if allowMajor {
			return "", "no stable release upstream"
		}
		return "", fmt.Sprintf("no %d.x release upstream; `--major` moves it to the newest line", cv.Major)
	}

	precision := min(strings.Count(pin, ".")+1, 3)
	target := version.Version{Major: best.Major}
	to := strconv.Itoa(best.Major)
	if precision >= 2 {
		target.Minor = best.Minor
		to = best.MajorMinor()
	}
	if precision == 3 {
		target.Patch = best.Patch
		to = best.String()
	}

	if compareVersions(target, cv) <= 0 {
		return "", "up to date"
	}
	if strings.HasPrefix(pin, "v") {
		to = "v" + to
	}
	return to, ""
}

// newestRelease returns the highest stable release in versions, limited to
// one major line unless major is -1. Pre-releases ("10.0.0-rc.1") are skipped.
func newestRelease(versions []string, major int) (version.Version, bool) {
	var best version.Version
	found := false
	for _, v := range versions {
		if strings.Contains(v, "-") {
			continue
		}
		pv, err := version.Parse(v)
		if err != nil || pv.Latest || pv.LTS {
			continue
		}
		if major >= 0 && pv.Major != major {
			continue
		}
		if !found || compareVersions(pv, best) > 0 {
			best, found = pv, true
		}
	}
	return best, found
}

// pinFile is the file that holds a pin.
func pinFile(p resolver.Pin) string {
	switch p.Source {
	case resolver.SourceProject:
		return filepath.Join(p.Dir, config.ProjectConfigFile)
	case resolver.SourceNvmrc:
		return filepath.Join(p.Dir, ".nvmrc")
	case resolver.SourceNodeVersion:
		return filepath.Join(p.Dir, ".node-version")
	default:
		return filepath.Join(p.Dir, "package.json")
	}
}

// writeBump rewrites one pin in place with the writer that owns its file.
func writeBump(p resolver.Pin, to string) error {
	switch p.Source {
	case resolver.SourceProject:
		cfg, err := config.LoadProject(p.Dir)
		if err != nil {
			return err
		}
		if cfg == nil {
			return fmt.Errorf("%s disappeared", config.ProjectConfigFile)
		}
		cfg.Tools.SetTool(p.Tool, to)
		return config.SaveProject(p.Dir, cfg)
	case resolver.SourcePackageJSON:
		return config.SavePackageJSONTool(p.Dir, p.Tool, to)
	case resolver.SourceNvmrc, resolver.SourceNodeVersion:
		return config.SaveVersionFile(pinFile(p), to)
	}
	return fmt.Errorf("a pin from %s is not rewritten by Driftr", p.Source)
}
