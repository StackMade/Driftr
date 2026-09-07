package installer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ExtractZipToBin unpacks the files stored under prefix in a zip archive into
// destDir/bin, stripping the prefix. A release archive that keeps its binary at
// "bun-linux-x64/bun" therefore installs it as "<destDir>/bin/bun".
//
// binName must be present once the archive is unpacked; if it is not, the
// half-written directory is removed rather than left looking like a valid
// install. Extraction happens in a temp dir under destDir's parent, so two
// concurrent installs of the same version cannot interleave.
func ExtractZipToBin(archivePath, destDir, prefix, binName string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer zr.Close()

	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return fmt.Errorf("failed to create tools dir: %w", err)
	}
	// MkdirTemp guarantees a unique name, so concurrent installs of the same
	// version cannot clobber each other's work dir.
	tmpDir, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create version dir: %w", err)
	}
	// After a successful rename tmpDir is gone and this is a no-op.
	defer os.RemoveAll(tmpDir)

	if err := os.Chmod(tmpDir, 0o755); err != nil {
		return fmt.Errorf("failed to set version dir permissions: %w", err)
	}

	// Use os.Root to sandbox all file operations within tmpDir. The kernel
	// enforces that no extracted path can escape this directory.
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to open root dir: %w", err)
	}
	defer root.Close()

	if err := root.Mkdir("bin", 0o755); err != nil {
		return fmt.Errorf("failed to create bin dir: %w", err)
	}

	matched := false
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		// Directories are created on demand from the file paths; anything that
		// is not a regular file (symlink, device, socket) has no legitimate
		// place in a release archive and is skipped.
		if !f.Mode().IsRegular() {
			continue
		}

		rel := path.Clean(strings.TrimPrefix(f.Name, prefix))
		if rel == "" || rel == "." {
			continue
		}
		if path.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("archive entry %q escapes the extraction directory", f.Name)
		}

		if err := extractZipEntry(root, path.Join("bin", rel), f); err != nil {
			return err
		}
		matched = true
	}

	// A valid archive always contains the platform-prefixed directory. No
	// matches means a wrong or corrupted archive, not a partial install.
	if !matched {
		return fmt.Errorf("archive has unexpected layout: no entries under %q. Run 'driftr cache clean' and retry", prefix)
	}

	binRel := path.Join("bin", binName)
	if _, err := root.Stat(binRel); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("extraction completed but %s binary not found in the archive", binName)
		}
		return fmt.Errorf("failed to verify extracted %s binary: %w", binName, err)
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		// Another process may have won the race — check if the binary is there.
		if _, statErr := os.Stat(filepath.Join(destDir, "bin", binName)); statErr == nil {
			return nil
		}
		return fmt.Errorf("failed to finalize install: %w", err)
	}

	return nil
}

// extractZipEntry writes one zip file entry into an os.Root-sandboxed directory.
func extractZipEntry(root *os.Root, relPath string, f *zip.File) error {
	if dir := path.Dir(relPath); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", relPath, err)
		}
	}

	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to read %s from archive: %w", f.Name, err)
	}
	defer src.Close()

	out, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", relPath, err)
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return fmt.Errorf("failed to write file %s: %w", relPath, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close file %s: %w", relPath, err)
	}
	return nil
}
