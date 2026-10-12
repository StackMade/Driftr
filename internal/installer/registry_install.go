package installer

import (
	"fmt"
	"os"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/version"
)

// installRegistryTool downloads and installs a version of a tool published as
// an npm package of the same name (npm, pnpm, yarn), using the registry
// tarball with SHA-512 SRI verification. example is a version spec named in
// the error that rejects "lts".
func installRegistryTool(tool, example, versionStr string, verbose bool) (string, error) {
	if err := platform.EnsureToolDirs(tool); err != nil {
		return "", err
	}

	v, err := version.Parse(versionStr)
	if err != nil {
		return "", fmt.Errorf("invalid version: %w", err)
	}
	if v.LTS {
		return "", fmt.Errorf("lts is only supported for node (driftr install node@lts); %s has no LTS concept, use a version like %s", tool, example)
	}

	// Resolve partial versions via npm registry.
	resolvedVersion := v.String()
	if v.Latest || v.IsPartial() {
		resolved, err := ResolveRegistryLatest(tool, v)
		if err != nil {
			return "", err
		}
		resolvedVersion = resolved
		if verbose {
			fmt.Printf("  Resolved %s to %s\n", versionStr, resolvedVersion)
		}
	}

	// Check if already installed.
	binPath, err := platform.ToolBinary(tool, resolvedVersion)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(binPath); err == nil {
		if verbose {
			fmt.Printf("  %s %s is already installed\n", tool, resolvedVersion)
		}
		return resolvedVersion, nil
	}

	// Download from npm registry.
	archivePath, rv, err := DownloadRegistryPackage(tool, resolvedVersion, verbose)
	if err != nil {
		return "", err
	}

	// Verify integrity.
	if rv.Dist.Integrity == "" {
		err := fmt.Errorf("registry metadata for %s %s is missing integrity data; refusing to install unverified package", tool, resolvedVersion)
		return "", removeCorruptArchive(archivePath, err)
	}
	if verbose {
		fmt.Println("  Verifying integrity...")
	}
	if err := VerifyIntegrity(archivePath, rv.Dist.Integrity); err != nil {
		return "", removeCorruptArchive(archivePath, err)
	}

	// Extract to version directory.
	versionDir, err := platform.ToolVersionDir(tool, resolvedVersion)
	if err != nil {
		return "", err
	}

	if verbose {
		fmt.Printf("  Extracting to: %s\n", versionDir)
	}
	if err := ExtractRegistryPackage(archivePath, versionDir, binPath); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	// Ensure the binary is executable.
	if err := os.Chmod(binPath, 0o755); err != nil {
		return "", err
	}

	return resolvedVersion, nil
}
