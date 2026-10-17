package shim

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
)

// shimTools lists the tools for which shims are created.
var shimTools = []string{"node", "npm", "npx", "pnpm", "pnpx", "yarn", "bun"}

// ShimTools returns the list of tools that have shims.
func ShimTools() []string {
	return append([]string(nil), shimTools...)
}

// stampFile records which driftr version last generated the shims. Its name
// starts with a dot, so it can never collide with a package bin.
const stampFile = ".driftr-version"

// GenerateShims creates shim shell scripts in ~/.driftr/bin/.
// Each shim invokes `driftr shim <tool>` to resolve and exec the real binary.
// Besides driftr's own tools, every bin a global npm install left in a node
// version gets a shim, and package shims whose bin is gone are removed. It
// returns one note per package bin it left without a shim, and why, and
// stamps the shims with version so RefreshIfStale can tell they are current.
func GenerateShims(version string) ([]string, error) {
	binDir, err := platform.BinDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create bin dir: %w", err)
	}

	driftrBin, err := stableExecutable()
	if err != nil {
		return nil, fmt.Errorf("cannot determine driftr executable path: %w", err)
	}
	previous := BakedPath(Path(binDir, "node"))
	owned := shimOwner(binDir)

	for _, tool := range shimTools {
		if err := writeShim(binDir, tool, driftrBin); err != nil {
			return nil, fmt.Errorf("failed to create shim for %s: %w", tool, err)
		}
	}

	packageBins, err := platform.NodePackageBins()
	if err != nil {
		return nil, err
	}

	var skipped []string
	for name := range packageBins {
		// A file the user keeps here, such as the driftr binary itself, is
		// never overwritten by a package that happens to share its name.
		path := Path(binDir, name)
		if _, err := os.Lstat(path); err == nil && !owned(path, name) {
			skipped = append(skipped, fmt.Sprintf("%s: %s already exists and is not a driftr shim", name, path))
			delete(packageBins, name)
			continue
		}
		if err := writeShim(binDir, name, driftrBin); err != nil {
			return nil, fmt.Errorf("failed to create shim for %s: %w", name, err)
		}
	}
	slices.Sort(skipped)

	if err := removeStaleShims(binDir, packageBins, owned); err != nil {
		return nil, err
	}
	if err := recordShims(binDir, slices.Collect(maps.Keys(packageBins))); err != nil {
		return nil, err
	}
	if err := ioutil.WriteFileAtomic(filepath.Join(binDir, stampFile), []byte(version+"\n"), 0o644); err != nil {
		return nil, err
	}
	if previous != driftrBin {
		releaseHomebrewKeg(previous)
	}
	return skipped, nil
}

// releaseHomebrewKeg lets Homebrew clean up the keg old shims pointed at.
// Up to 1.2.1, shims on Linux baked the versioned Cellar path; the formula's
// post_install marks older kegs with .keepme so `brew upgrade` keeps them and
// those shims keep working. Once the shims point elsewhere, the mark goes.
// Only a .keepme in <prefix>/Cellar/driftr/<version> is ever removed.
func releaseHomebrewKeg(oldDriftr string) {
	keg := filepath.Dir(filepath.Dir(oldDriftr))
	if filepath.Base(filepath.Dir(keg)) != "driftr" || filepath.Base(filepath.Dir(filepath.Dir(keg))) != "Cellar" {
		return
	}
	_ = os.Remove(filepath.Join(keg, ".keepme")) // best effort: a kept keg only costs disk
}

// Stamp returns the driftr version that last generated the shims, or "" when
// they predate the stamp or were never generated.
func Stamp() string {
	binDir, err := platform.BinDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(binDir, stampFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// RefreshIfStale regenerates the shims when a different driftr version wrote
// them, which is how an upgrade by any route (Homebrew, self-update, the
// install script) brings new shims without anyone running setup. It reads one
// small file when nothing changed, and does nothing before setup has created
// the shim dir.
func RefreshIfStale(version string) error {
	binDir, err := platform.BinDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(binDir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if Stamp() == version {
		return nil
	}
	_, err = GenerateShims(version)
	return err
}

// removeStaleShims deletes package shims whose bin no node version provides
// any more, e.g. after `npm uninstall -g` or `driftr uninstall node@<ver>`.
// Only files that are a driftr shim for their own name are touched, so the
// driftr binary or anything else a user keeps in the bin dir stays put.
func removeStaleShims(binDir string, packageBins map[string][]string, owned func(path, tool string) bool) error {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("failed to read bin dir: %w", err)
	}
	for _, e := range entries {
		name := shimTool(e.Name())
		if _, core := platform.LookupTool(name); name == "" || core || packageBins[name] != nil || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(binDir, e.Name())
		if !owned(path, name) {
			continue
		}
		// A concurrent refresh may have removed it first.
		if err := removeShim(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove stale shim %s: %w", name, err)
		}
	}
	return nil
}

// stableExecutable returns the path driftr was invoked by, with symlinks left
// in place. Package managers like Homebrew expose a stable symlink (e.g.
// /opt/homebrew/bin/driftr) that points at a version-pinned target
// (.../Cellar/driftr/0.11.1/bin/driftr). Baking the version-pinned path into
// every shim leaves them dangling ("not found") once `brew upgrade` deletes
// that directory. os.Executable() alone is not enough: on Linux it reads
// /proc/self/exe, which has the symlinks resolved, so Linuxbrew installs lost
// their shims on every upgrade. The invoked path is used only when it is the
// same file as the running binary; otherwise os.Executable() is. An absolute
// path is also more robust than relying on PATH, since shims run in
// non-interactive shells where the driftr install dir may not be on PATH.
func stableExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if invoked, ok := invokedPath(); ok && sameFile(invoked, exe) {
		return invoked, nil
	}
	return exe, nil
}

// invokedPath turns os.Args[0] into an absolute path without resolving
// symlinks, looking a bare name up on PATH the way the shell did.
func invokedPath() (string, bool) {
	invoked := os.Args[0]
	if !strings.Contains(invoked, string(filepath.Separator)) {
		found, err := exec.LookPath(invoked)
		if err != nil {
			return "", false
		}
		invoked = found
	}
	abs, err := filepath.Abs(invoked)
	return abs, err == nil
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

// Path returns the path of tool's shim in binDir.
func Path(binDir, tool string) string {
	return filepath.Join(binDir, shimFile(tool))
}

// ShimDir returns the path to the shim directory for display purposes.
func ShimDir() (string, error) {
	return platform.BinDir()
}
