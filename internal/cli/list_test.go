package cli

import (
	"strings"
	"testing"
)

func TestListCmd_UnknownToolFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runCmd(t, "list", "foo")
	if err == nil || !strings.Contains(err.Error(), "unknown tool: foo") {
		t.Errorf("expected an unknown-tool error, got: %v", err)
	}
}

func TestListCmd_NpxListsNpm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())

	for _, flag := range []string{"--pre", "--limit=5"} {
		err := runCmd(t, "list", "pnpm", flag)
		if err == nil || !strings.Contains(err.Error(), "--remote") {
			t.Errorf("list pnpm %s: error = %v, want one naming --remote", flag, err)
		}
	}
}
