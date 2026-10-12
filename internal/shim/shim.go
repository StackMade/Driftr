package shim

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stackmade/driftr/internal/platform"
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
		path := filepath.Join(binDir, name)
		if _, err := os.Lstat(path); err == nil && !isShimFile(path, name) {
			skipped = append(skipped, fmt.Sprintf("%s: %s already exists and is not a driftr shim", name, path))
			delete(packageBins, name)
			continue
		}
		if err := writeShim(binDir, name, driftrBin); err != nil {
			return nil, fmt.Errorf("failed to create shim for %s: %w", name, err)
		}
	}
	slices.Sort(skipped)

	if err := removeStaleShims(binDir, packageBins); err != nil {
		return nil, err
	}
	return skipped, writeAtomic(filepath.Join(binDir, stampFile), []byte(version+"\n"), 0o644)
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
func removeStaleShims(binDir string, packageBins map[string][]string) error {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("failed to read bin dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if _, core := platform.LookupTool(name); core || packageBins[name] != nil || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(binDir, name)
		if !isShimFile(path, name) {
			continue
		}
		// A concurrent refresh may have removed it first.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove stale shim %s: %w", name, err)
		}
	}
	return nil
}

// isShimFile reports whether path holds the shim writeShim makes for tool.
// The size check keeps it from reading a binary such as driftr itself.
func isShimFile(path, tool string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && isShimFor(string(data), tool)
}

// isShimFor matches what writeShim produces for tool, with any driftr path:
// the path changes when driftr moves, and an old shim is still ours.
func isShimFor(content, tool string) bool {
	return strings.HasPrefix(content, "#!/bin/sh\nexec \"") &&
		strings.HasSuffix(content, "\" shim "+tool+" \"$@\"\n") &&
		strings.Count(content, "\n") == 2
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

func writeShim(binDir, tool, driftrBin string) error {
	content := fmt.Sprintf(`#!/bin/sh
exec "%s" shim %s "$@"
`, driftrBin, tool)
	return writeAtomic(filepath.Join(binDir, tool), []byte(content), 0o755)
}

// writeAtomic replaces path in one rename. Shims are rewritten while other
// processes run them, possibly by several driftr processes refreshing at once
// after an upgrade; writing in place could let a shell read a half-written
// shim. The mode is set explicitly, so a shim that lost its executable bit
// gets it back.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ShimDir returns the path to the shim directory for display purposes.
func ShimDir() (string, error) {
	return platform.BinDir()
}
