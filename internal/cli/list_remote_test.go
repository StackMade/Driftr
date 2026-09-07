package cli

import (
	"strings"
	"testing"
)

// listRemoteOutput runs a remote listing and returns what it printed.
func listRemoteOutput(t *testing.T, tool string, pre bool, limit int) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = listRemote(tool, pre, limit) })
	return out, err
}

func TestListRemote_NodeMarksInstalledAndLTS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	nodeIndexServer(t, []map[string]any{
		{"version": "v24.0.0", "lts": false},
		{"version": "v22.14.0", "lts": "Jod"},
		{"version": "v20.11.0", "lts": "Iron"},
	})
	fakeNodeInstall(t, "22.14.0")

	out, err := listRemoteOutput(t, "node", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Available node versions (all 3):") {
		t.Errorf("header missing or wrong: %q", out)
	}
	if !strings.Contains(out, "22.14.0 (LTS: Jod)") {
		t.Errorf("LTS codename not shown: %q", out)
	}
	if strings.Contains(out, "24.0.0 (LTS") {
		t.Errorf("non-LTS release was labelled LTS: %q", out)
	}
	// The installed version carries the ● marker; the others do not.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "22.14.0") && !strings.Contains(line, "●") {
			t.Errorf("installed version not marked: %q", line)
		}
		if strings.Contains(line, "24.0.0") && strings.Contains(line, "●") {
			t.Errorf("uninstalled version marked as installed: %q", line)
		}
	}
	if !strings.Contains(out, "● = installed") {
		t.Errorf("legend missing: %q", out)
	}
}

func TestListRemote_LimitTruncates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	nodeIndexServer(t, []map[string]any{
		{"version": "v24.0.0", "lts": false},
		{"version": "v22.14.0", "lts": "Jod"},
		{"version": "v20.11.0", "lts": "Iron"},
	})

	out, err := listRemoteOutput(t, "node", false, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "(latest 2 of 3)") {
		t.Errorf("header does not report the truncation: %q", out)
	}
	if strings.Contains(out, "20.11.0") {
		t.Errorf("version beyond the limit was printed: %q", out)
	}
}

func TestListRemote_NpmPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	npmRegistryServer(t, []string{"9.15.0", "9.14.0", "8.15.0"})

	out, err := listRemoteOutput(t, "pnpm", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Available pnpm versions (all 3):") {
		t.Errorf("unexpected output: %q", out)
	}
	// Newest first.
	if strings.Index(out, "9.15.0") > strings.Index(out, "8.15.0") {
		t.Errorf("versions are not newest-first: %q", out)
	}
}

func TestListRemote_Bun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	bunReleasesServer(t, []string{"1.2.3", "1.2.2"})

	out, err := listRemoteOutput(t, "bun", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Available bun versions (all 2):") || !strings.Contains(out, "1.2.3") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestListRemote_UnsupportedTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	_, err := listRemoteOutput(t, "npm", false, 0)
	if err == nil || !strings.Contains(err.Error(), "remote listing not supported") {
		t.Fatalf("expected an unsupported-tool error, got: %v", err)
	}
}

// An unreachable release source must fail loudly, not print an empty list.
func TestListRemote_UpstreamFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	npmRegistryServer(t, nil) // answers 500

	_, err := listRemoteOutput(t, "pnpm", false, 0)
	if err == nil || !strings.Contains(err.Error(), "failed to fetch pnpm versions") {
		t.Fatalf("expected a fetch failure, got: %v", err)
	}
}

func TestLtsCodename(t *testing.T) {
	tests := []struct {
		name string
		lts  any
		want string
	}{
		{"codename", "Jod", "Jod"},
		{"not lts", false, ""},
		{"true without codename", true, ""},
		{"missing field", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ltsCodename(tt.lts); got != tt.want {
				t.Errorf("ltsCodename(%v) = %q, want %q", tt.lts, got, tt.want)
			}
		})
	}
}
