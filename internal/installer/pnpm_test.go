package installer

import (
	"testing"

	"github.com/stackmade/driftr/v2/internal/testutil"
)

func TestInstallPnpm_RejectsLTS(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	if _, err := InstallPnpm("lts", false); err == nil {
		t.Fatal("InstallPnpm(lts) expected error, got nil")
	}
}
