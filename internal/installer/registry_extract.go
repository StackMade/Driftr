package installer

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractRegistryPackage extracts an npm registry tarball to the destination directory.
// npm tarballs contain a top-level "package/" prefix which is stripped during extraction.
// binaryPath is used to detect a concurrent install race on rename failure.
func ExtractRegistryPackage(archivePath, destDir, binaryPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("failed to create tools dir: %w", err)
	}
	// MkdirTemp guarantees a unique name, so concurrent installs of the same
	// version cannot clobber each other's work dir.
	tmpDir, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create destination dir: %w", err)
	}
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to set destination dir permissions: %w", err)
	}

	// Use os.Root to sandbox all file operations within tmpDir.
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to open root dir: %w", err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			root.Close()
			os.RemoveAll(tmpDir)
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Strip the "package/" prefix that all npm tarballs use.
		name := hdr.Name
		if _, after, ok := strings.Cut(name, "/"); ok {
			name = after
		}
		if name == "" {
			continue
		}

		if err := extractToRoot(root, name, hdr, tr); err != nil {
			root.Close()
			os.RemoveAll(tmpDir)
			return err
		}
	}

	root.Close()

	if err := checkSymlinks(tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		// Another process may have won the race — check if binary exists
		if _, statErr := os.Stat(binaryPath); statErr == nil {
			os.RemoveAll(tmpDir)
			return nil
		}
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to finalize install: %w", err)
	}

	return nil
}
