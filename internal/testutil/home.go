// Package testutil holds helpers shared by the unit tests. Nothing in the
// shipped binary imports it.
package testutil

import "testing"

// SetHome points the home directory at dir for the rest of the test, and
// returns dir. HOME is what Unix reads; Windows reads USERPROFILE.
func SetHome(t testing.TB, dir string) string {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}
