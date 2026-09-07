package shim

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/stackmade/driftr/internal/platform"
)

// shimTools lists the tools for which shims are created.
var shimTools = []string{"node", "npm", "npx", "pnpm", "pnpx", "yarn", "bun"}

// ShimTools returns the list of tools that have shims.
func ShimTools() []string {
	return append([]string(nil), shimTools...)
}

// GenerateShims creates shim shell scripts in ~/.driftr/bin/.
// Each shim invokes `driftr shim <tool>` to resolve and exec the real binary.
func GenerateShims() error {
	binDir, err := platform.BinDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("failed to create bin dir: %w", err)
	}

	driftrBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine driftr executable path: %w", err)
	}
	// Intentionally do NOT resolve symlinks here. Package managers like
	// Homebrew expose a stable symlink (e.g. /opt/homebrew/bin/driftr) that
	// points at a version-pinned target (.../Cellar/driftr/0.11.1/bin/driftr).
	// Resolving the symlink bakes the version-pinned path into every shim, so
	// the next `brew upgrade` deletes that directory and leaves the shims
	// dangling ("No such file or directory"). os.Executable() already returns
	// the path the process was invoked with — the stable symlink — which
	// survives upgrades. An absolute path is also more robust than relying on
	// PATH, since shims run in non-interactive shells where the driftr install
	// dir may not be on PATH.

	for _, tool := range shimTools {
		if err := writeShim(binDir, tool, driftrBin); err != nil {
			return fmt.Errorf("failed to create shim for %s: %w", tool, err)
		}
	}

	return nil
}

func writeShim(binDir, tool, driftrBin string) error {
	shimPath := filepath.Join(binDir, tool)

	content := fmt.Sprintf(`#!/bin/sh
exec "%s" shim %s "$@"
`, driftrBin, tool)

	return os.WriteFile(shimPath, []byte(content), 0o755)
}

// ShimDir returns the path to the shim directory for display purposes.
func ShimDir() (string, error) {
	return platform.BinDir()
}
