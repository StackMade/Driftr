package ioutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies src to dst, creating dst with mode 0o755.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("copy failed: %w; close failed: %w", err, cerr)
		}
		return err
	}
	// A failed close means a truncated copy.
	return out.Close()
}

// WriteFileAtomic replaces path in one rename, so a reader never sees a
// half-written file. Shims are rewritten while other processes run them,
// possibly by several driftr processes refreshing at once after an upgrade.
// The mode is set explicitly, so a shim that lost its executable bit gets it
// back.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
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
