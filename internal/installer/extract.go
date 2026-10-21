package installer

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/v2/internal/platform"
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

// checkSymlinks rejects any symlink under dir whose target leaves dir once
// the links already on disk are followed. symlinkEscapes sees one entry at a
// time and resolves ".." lexically, so a chain such as b -> .. followed by
// b/c -> ../.. passes it while c points two levels above dir. Run it on the
// finished tree, before the rename makes the version visible.
func checkSymlinks(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if resolvesOutside(dir, filepath.ToSlash(rel)) {
			return fmt.Errorf("archive entry %s is a symlink pointing outside the install directory. Run `driftr cache clean` and retry", rel)
		}
		return nil
	})
}

// resolvesOutside follows rel inside dir one component at a time, reading
// every symlink it meets, and reports whether the walk climbs above dir.
// Components that do not exist are taken lexically.
func resolvesOutside(dir, rel string) bool {
	parts := strings.Split(rel, "/")
	var cur []string
	for hops := 0; len(parts) > 0; {
		p := parts[0]
		parts = parts[1:]
		switch p {
		case "", ".":
			continue
		case "..":
			if len(cur) == 0 {
				return true
			}
			cur = cur[:len(cur)-1]
			continue
		}
		cur = append(cur, p)
		full := filepath.Join(dir, filepath.FromSlash(path.Join(cur...)))
		fi, err := os.Lstat(full)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		if hops++; hops > 255 {
			return true // a loop: treat as unsafe rather than spin
		}
		target, err := os.Readlink(full)
		if err != nil || target == "" || path.IsAbs(target) {
			return true
		}
		cur = cur[:len(cur)-1]
		parts = append(strings.Split(target, "/"), parts...)
	}
	return false
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
			return fmt.Errorf("archive entry %s is a symlink pointing outside the install directory (%s). Run `driftr cache clean` and retry", relPath, hdr.Linkname)
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

	// The archive contains "node-v<version>-<os>-<arch>/" as prefix.
	prefix := fmt.Sprintf("node-v%s-%s-%s/", version, platform.OS(), nodeArch(version))

	if platform.ArchiveExt() == "zip" {
		if verbose {
			fmt.Printf("  Extracting to: %s\n", destDir)
		}
		// The Windows archive keeps node.exe and npm at its top level, not in
		// bin/. Unpacking it into bin/ keeps one layout for every platform,
		// and npm, whose global prefix is node.exe's dir, puts package bins
		// there too.
		return extractZipToBin(archivePath, destDir, prefix, platform.ExeName("node"), cleanup)
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
		return fmt.Errorf("archive has unexpected layout: no entries under %q. Run `driftr cache clean` and retry", prefix)
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

	if err := checkSymlinks(tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}

	return finalizeInstall(tmpDir, destDir, nodeBin, cleanup)
}

// finalizeInstall moves a verified work dir to destDir and removes it if the
// move fails. destDir may exist already: another install of the version won
// the race, and its binary is there; or a removal or install stopped halfway
// and left a dir without one, which is replaced instead of blocking every
// later install of that version.
//
// An interrupt removes the work dir under cleanup's lock, which is held here:
// otherwise that removal could run on through a dir just renamed into place
// and leave an empty version dir.
func finalizeInstall(tmpDir, destDir, binPath string, cleanup *installCleanup) error {
	if cleanup != nil {
		cleanup.mu.Lock()
		defer cleanup.mu.Unlock()
		if cleanup.ran {
			return errors.New("install interrupted")
		}
		cleanup.tmpDir = ""
	}
	defer os.RemoveAll(tmpDir) // a no-op once the rename succeeded
	err := platform.Rename(tmpDir, destDir)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(binPath); statErr == nil {
		return nil
	}
	if _, statErr := os.Stat(destDir); statErr == nil {
		if rmErr := platform.RemoveVersionDir(destDir); rmErr != nil {
			return fmt.Errorf("failed to finalize install: %s is incomplete and could not be removed: %w", destDir, rmErr)
		}
		if err = platform.Rename(tmpDir, destDir); err == nil {
			return nil
		}
		if _, statErr := os.Stat(binPath); statErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to finalize install: %w", err)
}
