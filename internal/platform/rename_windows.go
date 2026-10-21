package platform

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Rename is os.Rename, retried for about two seconds while Windows reports
// the file or a file under the directory in use. Antivirus and the search
// indexer open freshly written files, node.exe above all, for a moment, and a
// rename in that window fails with access denied. A program that runs from
// the directory holds it for longer, and the error is returned then.
func Rename(oldpath, newpath string) error {
	var err error
	for delay := 10 * time.Millisecond; ; delay *= 2 {
		if err = os.Rename(oldpath, newpath); err == nil || delay > time.Second {
			return err
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(delay)
	}
}
