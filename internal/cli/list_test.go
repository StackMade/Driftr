package cli

import (
	"strings"
	"testing"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/testutil"
)

func TestListCmd_UnknownToolFails(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	err := runCmd(t, "list", "foo")
	if err == nil || !strings.Contains(err.Error(), "unknown tool: foo") {
		t.Errorf("expected an unknown-tool error, got: %v", err)
	}
}

func TestListCmd_NpxListsNpm(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "list", "npx") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No npm versions installed") {
		t.Errorf("output = %q, want npx to list npm", out)
	}
}

func TestListCmd_RemoteOnlyFlagsNeedRemote(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	for _, flag := range []string{"--pre", "--limit=5"} {
		err := runCmd(t, "list", "pnpm", flag)
		if err == nil || !strings.Contains(err.Error(), "--remote") {
			t.Errorf("list pnpm %s: error = %v, want one naming --remote", flag, err)
		}
	}
}

// Installed versions come back in directory order, which put 9.0.0 last and
// 22.14.0 before 22.9.0. A partial default still gets its marker.
func TestListCmd_NumericOrderAndPartialDefault(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())
	for _, v := range []string{"9.0.0", "22.14.0", "22.9.0"} {
		fakeNodeInstall(t, v)
	}
	cfg := &config.GlobalConfig{}
	cfg.Default.SetTool("node", "22")
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	var err error
	out := captureStdout(t, func() { err = runCmd(t, "list", "node") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	i9, i229, i2214 := strings.Index(out, " 9.0.0"), strings.Index(out, "22.9.0"), strings.Index(out, "22.14.0")
	if i9 < 0 || i9 >= i229 || i229 >= i2214 {
		t.Errorf("versions not in numeric order:\n%s", out)
	}
	if !strings.Contains(out, "* 22.14.0") {
		t.Errorf("default 22 should mark 22.14.0:\n%s", out)
	}
}
