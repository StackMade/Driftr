//go:build !windows

package platform

import "os"

// Rename is os.Rename. On Windows it also waits out a scanner holding a file.
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
