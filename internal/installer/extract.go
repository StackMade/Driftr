package installer

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/internal/platform"
)

// symlinkEscapes reports whether a symlink created at linkPath and pointing at
// target would resolve outside the extraction root.
//
// os.Root sandboxes where a link is created and refuses to follow one that
// leaves the root, so an escaping link cannot write outside during extraction.
// It outlives extraction, though: once the version dir is in place, anything
// reading it without os.Root — npm, a postinstall script, a user's own tooling
// — follows the link wherever it points. Both paths use slashes because they
// come from tar headers.
func symlinkEscapes(linkPath, target string) bool {
	if target == "" || path.IsAbs(target) {
		return true
	}
	resolved := path.Join(path.Dir(linkPath), target)
	return resolved == ".." || strings.HasPrefix(resolved, "../")
}

// extractToRoot extracts a tar entry into an os.Root-sandboxed directory.
// The Root enforces that no path can escape destDir, replacing manual prefix checks.
func extractToRoot(root *os.Root, relPath string, hdr *tar.Header, tr *tar.Reader) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := root.MkdirAll(relPath, hdr.FileInfo().Mode().Perm()); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", relPath, err)
		}

	case tar.TypeReg:
		if dir := filepath.Dir(relPath); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create parent dir for %s: %w", relPath, err)
			}
		}
		outFile, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", relPath, err)
		}
		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return fmt.Errorf("failed to write file %s: %w", relPath, err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("failed to close file %s: %w", relPath, err)
		}

	case tar.TypeSymlink:
		if symlinkEscapes(relPath, hdr.Linkname) {
			return fmt.Errorf("archive entry %s is a symlink pointing outside the install directory (%s). Run 'driftr cache clean' and retry", relPath, hdr.Linkname)
		}
		if dir := filepath.Dir(relPath); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create parent dir for %s: %w", relPath, err)
			}
		}
		if err := root.Remove(relPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove existing path %s before creating symlink: %w", relPath, err)
		}
		if err := root.Symlink(hdr.Linkname, relPath); err != nil {
			return fmt.Errorf("failed to create symlink %s: %w", relPath, err)
		}
	}
	return nil
}

// Extract unpacks the downloaded archive into the tools directory.
// The Node.js archive contains a top-level directory like "node-v24.0.0-darwin-arm64/".
// We extract its contents into ~/.driftr/tools/node/<version>/.
func Extract(archivePath, version string, verbose bool, cleanup *installCleanup) error {
	destDir, err := platform.NodeVersionDir(version)
	if err != nil {
		return err
	}

	// If already extracted, skip.
	nodeBin, err := platform.NodeBinary(version)
	if err != nil {
		return err
	}
	if _, err := os.Stat(nodeBin); err == nil {
		if verbose {
			fmt.Printf("  Already extracted: %s\n", destDir)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("failed to create tools dir: %w", err)
	}
	// MkdirTemp guarantees a unique name, so concurrent installs of the same
	// version cannot clobber each other's work dir.
	tmpDir, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create version dir: %w", err)
	}
	// A signal can arrive at any point below, and the name is random, so the
	// interrupt handler has to be told where the work dir is.
	cleanup.setTmpDir(tmpDir)
	defer cleanup.clearTmpDir()
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to set version dir permissions: %w", err)
	}

	if verbose {
		fmt.Printf("  Extracting to: %s\n", destDir)
	}

	// Use os.Root to sandbox all file operations within tmpDir.
	// The kernel enforces that no extracted path can escape this directory.
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to open root dir: %w", err)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		root.Close()
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		root.Close()
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// The archive contains "node-v<version>-<os>-<arch>/" as prefix.
	prefix := fmt.Sprintf("node-v%s-%s-%s/", version, platform.OS(), platform.Arch())
	matched := false

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			root.Close()
			os.RemoveAll(tmpDir)
			return fmt.Errorf("failed to read archive: %w", err)
		}

		name := hdr.Name
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		matched = true

		// Strip the archive prefix to get the relative path.
		relPath := path.Clean(strings.TrimPrefix(name, prefix))
		if relPath == "" || relPath == "." {
			continue
		}

		if err := extractToRoot(root, relPath, hdr, tr); err != nil {
			root.Close()
			os.RemoveAll(tmpDir)
			return err
		}
	}

	// A valid Node.js archive always contains the version-prefixed directory.
	// No matches means a wrong or corrupted archive, not a partial install.
	if !matched {
		root.Close()
		os.RemoveAll(tmpDir)
		return fmt.Errorf("archive has unexpected layout: no entries under %q. Run 'driftr cache clean' and retry", prefix)
	}

	// Verify the node binary exists after extraction.
	if _, err := root.Stat(filepath.Join("bin", "node")); err != nil {
		root.Close()
		os.RemoveAll(tmpDir)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("extraction completed but node binary not found at %s", nodeBin)
		}
		return fmt.Errorf("failed to verify extracted node binary at %s: %w", nodeBin, err)
	}

	root.Close()

	if err := os.Rename(tmpDir, destDir); err != nil {
		// Another process may have won the race — check if binary exists
		if _, statErr := os.Stat(nodeBin); statErr == nil {
			os.RemoveAll(tmpDir)
			return nil
		}
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to finalize install: %w", err)
	}

	return nil
}
