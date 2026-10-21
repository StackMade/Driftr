package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/v2/internal/platform"
	"github.com/stackmade/driftr/v2/internal/testutil"
	"github.com/stackmade/driftr/v2/internal/version"
)

func nodeIndexServer(t *testing.T, releases []map[string]any) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_NODE_MIRROR", srv.URL)
}

// A nil version list makes the registry fail with a 500, to exercise the per-tool failure path.
func npmRegistryServer(t *testing.T, versions []string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if versions == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload := map[string]any{"name": "pnpm", "versions": map[string]any{}}
		vers := payload["versions"].(map[string]any)
		for _, v := range versions {
			vers[v] = map[string]any{"version": v}
		}
		json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_NPM_REGISTRY", srv.URL)
}

// bun ships binaries on GitHub releases rather than on npm.
func bunReleasesServer(t *testing.T, versions []string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		releases := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			releases = append(releases, map[string]any{"tag_name": "bun-v" + v})
		}
		json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DRIFTR_BUN_RELEASES", srv.URL)
}

func projectWith(t *testing.T, toml string) {
	t.Helper()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".driftr.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
}

func outdatedOutput(t *testing.T, only string, pre, exitCode bool) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := runOutdated(&buf, only, pre, exitCode)
	return buf.String(), err
}

func rowFor(t *testing.T, out, tool string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, tool+" ") {
			return line
		}
	}
	t.Fatalf("no row for %q in output:\n%s", tool, out)
	return ""
}

func TestOutdated_BehindOnPatch(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	projectWith(t, "[tools]\nnode = \"22.14.0\"\n")
	nodeIndexServer(t, []map[string]any{
		{"version": "v22.21.0", "lts": "Jod"},
		{"version": "v22.14.0", "lts": "Jod"},
	})

	out, err := outdatedOutput(t, "node", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	row := rowFor(t, out, "node")
	if !strings.Contains(row, "behind: 22.21.0") {
		t.Errorf("expected behind status, got: %q", row)
	}
}

func TestOutdated_BehindOnMajorButCurrentInLine(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "22.21.0")
	projectWith(t, "[tools]\nnode = \"22.21.0\"\n")
	nodeIndexServer(t, []map[string]any{
		{"version": "v24.1.0", "lts": false},
		{"version": "v22.21.0", "lts": "Jod"},
	})

	out, err := outdatedOutput(t, "node", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	row := rowFor(t, out, "node")
	if !strings.Contains(row, "newest in 22.x") || !strings.Contains(row, "24.1.0") {
		t.Errorf("expected newest-in-line status, got: %q", row)
	}
	// The newest LTS is reported even when it is not the newest release.
	if !strings.Contains(row, "22.21.0") {
		t.Errorf("expected LTS column, got: %q", row)
	}
}

func TestOutdated_UpToDate(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "24.1.0")
	projectWith(t, "[tools]\nnode = \"24.1.0\"\n")
	nodeIndexServer(t, []map[string]any{
		{"version": "v24.1.0", "lts": false},
		{"version": "v22.21.0", "lts": "Jod"},
	})

	out, err := outdatedOutput(t, "node", false, true)
	if err != nil {
		t.Fatalf("--exit-code must stay zero when everything is current: %v", err)
	}
	if row := rowFor(t, out, "node"); !strings.Contains(row, "up to date") {
		t.Errorf("expected up-to-date status, got: %q", row)
	}
}

func TestOutdated_OneToolFailsOthersSucceed(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "24.1.0")
	projectWith(t, "[tools]\nnode = \"24.1.0\"\npnpm = \"9.0.0\"\n")
	nodeIndexServer(t, []map[string]any{{"version": "v24.1.0", "lts": false}})
	npmRegistryServer(t, nil) // registry is down

	out, err := outdatedOutput(t, "", false, false)
	if err != nil {
		t.Fatalf("a single failing tool must not fail the run: %v", err)
	}
	if row := rowFor(t, out, "node"); !strings.Contains(row, "up to date") {
		t.Errorf("node row = %q, want up to date", row)
	}
	if row := rowFor(t, out, "pnpm"); !strings.Contains(row, "unknown") {
		t.Errorf("pnpm row = %q, want unknown", row)
	}
}

func TestOutdated_ExitCodeWhenBehind(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "22.14.0")
	projectWith(t, "[tools]\nnode = \"22.14.0\"\n")
	nodeIndexServer(t, []map[string]any{{"version": "v22.21.0", "lts": "Jod"}})

	if _, err := outdatedOutput(t, "node", false, true); err == nil {
		t.Error("--exit-code must fail when a tool is behind")
	}
}

func TestOutdated_NothingInstalled(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())

	out, err := outdatedOutput(t, "", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "driftr install") {
		t.Errorf("expected a pointer at `driftr install`, got: %q", out)
	}
}

func TestOutdated_BunUsesGitHubReleases(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	bin, err := platform.ToolBinary("bun", "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	projectWith(t, "[tools]\nbun = \"1.2.0\"\n")
	bunReleasesServer(t, []string{"1.4.2", "1.2.0"})

	out, err := outdatedOutput(t, "bun", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	row := rowFor(t, out, "bun")
	if !strings.Contains(row, "1.4.2") {
		t.Errorf("expected the newest GitHub release, got: %q", row)
	}
}

func TestOutdated_UnknownTool(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())

	if _, err := outdatedOutput(t, "ruby", false, false); err == nil {
		t.Error("expected an unknown-tool error")
	}
}

func TestOutdated_UnpinnedComparesNewestInstalled(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	fakeNodeInstall(t, "20.9.0")
	fakeNodeInstall(t, "20.11.0")
	t.Chdir(t.TempDir())
	nodeIndexServer(t, []map[string]any{
		{"version": "v20.12.0", "lts": "Iron"},
		{"version": "v20.11.0", "lts": "Iron"},
	})

	out, err := outdatedOutput(t, "node", false, false)
	if err != nil {
		t.Fatal(err)
	}
	row := rowFor(t, out, "node")
	for _, want := range []string{"20.11.0", "installed", "behind: 20.12.0"} {
		if !strings.Contains(row, want) {
			t.Errorf("row lacks %q: %q", want, row)
		}
	}
}

// With --pre the newest upstream release can be a pre-release; a stable
// version below it is still behind, so --exit-code fails.
func TestOutdatedStatus_PreReleaseLatest(t *testing.T) {
	cv, err := version.Parse("9.1.0")
	state, _ := outdatedStatus("9.1.0", cv, err, "10.0.0-rc.1", "9.15.0")
	if state == stateUnknown || state == stateCurrent {
		t.Errorf("state = %d, want behind", state)
	}
}
