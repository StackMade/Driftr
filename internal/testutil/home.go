// Package testutil holds helpers shared by the unit tests. Nothing in the
// shipped binary imports it.
package testutil

import (
	"runtime"
	"testing"
)

// SetHome points the home directory at dir for the rest of the test, and
// returns dir. HOME is what Unix reads; Windows reads USERPROFILE.
func SetHome(t testing.TB, dir string) string {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

// SkipOnWindows skips a test of behavior that Windows does not have, such as
// shell rc files, execute bits or symlinks an unprivileged user can create.
func SkipOnWindows(t testing.TB, why string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("not on Windows: " + why)
	}
}
