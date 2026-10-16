package installer

import (
	"testing"

	"github.com/stackmade/driftr/internal/testutil"
)

func TestInstallYarn_RejectsLTS(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	if _, err := InstallYarn("lts", false); err == nil {
		t.Fatal("InstallYarn(lts) expected error, got nil")
	}
}
