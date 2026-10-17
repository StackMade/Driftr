package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/resolver"
	"github.com/stackmade/driftr/v2/internal/testutil"
)

func TestBumpTarget(t *testing.T) {
	// Deliberately not sorted: the rule must not depend on upstream order.
	upstream := []string{"22.14.0", "24.2.0", "22.21.1", "25.0.0-rc.1", "24.10.3", "22.9.0", "20.19.4"}

	tests := []struct {
		name       string
		pin        string
		allowMajor bool
		want       string
		wantNote   string
	}{
		{"full pin moves within its major", "22.14.0", false, "22.21.1", ""},
		{"full pin with --major moves to newest stable", "22.14.0", true, "24.10.3", ""},
		{"major.minor keeps its precision", "22.14", false, "22.21", ""},
		{"major.minor with --major", "22.14", true, "24.10", ""},
		{"major-only already floats", "22", false, "", "up to date"},
		{"major-only with --major", "22", true, "24", ""},
		{"pre-releases are never a target", "24.10.3", true, "", "up to date"},
		{"never moves backwards", "26.0.0", true, "", "up to date"},
		{"newest in line is up to date", "22.21.1", false, "", "up to date"},
		{"leading v survives", "v22.14.0", false, "v22.21.1", ""},
		{"no release on the line", "18.0.0", false, "", "no 18.x release upstream"},
		{"no release on the line, --major", "18.0.0", true, "24.10.3", ""},
		{"lts alias is left alone", "lts/iron", true, "", "alias"},
		{"lts star alias is left alone", "lts/*", false, "", "alias"},
		{"latest is left alone", "latest", true, "", "alias"},
		{"garbage is reported", "banana", false, "", "cannot parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, note := bumpTarget(tt.pin, upstream, tt.allowMajor)
			if got != tt.want {
				t.Errorf("bumpTarget(%q, major=%v) = %q, want %q (note %q)", tt.pin, tt.allowMajor, got, tt.want, note)
			}
			if tt.wantNote != "" && !strings.Contains(note, tt.wantNote) {
				t.Errorf("note = %q, want it to contain %q", note, tt.wantNote)
			}
		})
	}
}

func TestBumpTarget_OnlyPrereleasesOnLine(t *testing.T) {
	got, note := bumpTarget("10.0.0", []string{"10.1.0-beta.2", "9.15.0"}, false)
	if got != "" {
		t.Errorf("got %q, want no bump when the line only has pre-releases (note %q)", got, note)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWriteBump_DriftrToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, config.ProjectConfigFile), "[tools]\nnode = \"22.1.0\"\npnpm = \"9.0.0\"\n")

	if err := writeBump(resolver.Pin{Tool: "node", Source: resolver.SourceProject, Dir: dir}, "22.21.1"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Tools.GetTool("node"); got != "22.21.1" {
		t.Errorf("node = %q, want 22.21.1", got)
	}
	if got := cfg.Tools.GetTool("pnpm"); got != "9.0.0" {
		t.Errorf("pnpm = %q, want the other pin untouched", got)
	}
}

func TestWriteBump_PackageJSONKeepsOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	writeFile(t, path, "{\n  \"name\": \"app\",\n  \"driftr\": {\"node\": \"22.1.0\"},\n  \"engines\": {\"node\": \">=18\"},\n  \"scripts\": {\"test\": \"x\"}\n}\n")

	if err := writeBump(resolver.Pin{Tool: "node", Source: resolver.SourcePackageJSON, Dir: dir}, "22.21.1"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	want := "{\n  \"name\": \"app\",\n  \"driftr\": {\"node\":\"22.21.1\"},\n  \"engines\": {\"node\": \">=18\"},\n  \"scripts\": {\"test\": \"x\"}\n}\n"
	if got != want {
		t.Errorf("package.json =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteBump_VersionFiles(t *testing.T) {
	for _, tc := range []struct {
		source resolver.Source
		name   string
	}{
		{resolver.SourceNvmrc, ".nvmrc"},
		{resolver.SourceNodeVersion, ".node-version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.name)
			writeFile(t, path, "v22.1.0\n")
			if err := writeBump(resolver.Pin{Tool: "node", Source: tc.source, Dir: dir}, "22.21.1"); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != "v22.21.1\n" {
				t.Errorf("%s = %q, want %q", tc.name, got, "v22.21.1\n")
			}
		})
	}
}

func TestWriteBump_RefusesReadOnlySources(t *testing.T) {
	for _, src := range []resolver.Source{resolver.SourceEnginesNode, resolver.SourcePackageManager} {
		if err := writeBump(resolver.Pin{Tool: "node", Source: src, Dir: t.TempDir()}, "24.0.0"); err == nil {
			t.Errorf("writeBump from %s succeeded, want a refusal", src)
		}
	}
}

func bumpOutput(t *testing.T, tools []string, allowMajor, dryRun bool) string {
	t.Helper()
	var buf bytes.Buffer
	if err := runPinBump(&buf, tools, allowMajor, dryRun); err != nil {
		t.Fatalf("runPinBump: %v\n%s", err, buf.String())
	}
	return buf.String()
}

func TestRunPinBump_RewritesAndDryRun(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	nodeIndexServer(t, []map[string]any{
		{"version": "v24.2.0", "lts": false},
		{"version": "v22.21.1", "lts": "Jod"},
		{"version": "v22.1.0", "lts": "Jod"},
	})
	dir := t.TempDir()
	t.Chdir(dir)
	nvmrc := filepath.Join(dir, ".nvmrc")
	writeFile(t, nvmrc, "v22.1.0\n")

	out := bumpOutput(t, nil, false, true)
	if got := readFile(t, nvmrc); got != "v22.1.0\n" {
		t.Errorf("--dry-run wrote .nvmrc: %q", got)
	}
	if !strings.Contains(out, "22.1.0 -> 22.21.1") || !strings.Contains(out, "nothing written") {
		t.Errorf("dry-run output = %q", out)
	}

	out = bumpOutput(t, nil, false, false)
	if got := readFile(t, nvmrc); got != "v22.21.1\n" {
		t.Errorf(".nvmrc = %q, want v22.21.1", got)
	}
	if !strings.Contains(out, "driftr install") {
		t.Errorf("output = %q, want an install hint", out)
	}

	out = bumpOutput(t, nil, false, false)
	if !strings.Contains(out, "Nothing to bump") {
		t.Errorf("second run output = %q, want nothing to bump", out)
	}

	bumpOutput(t, []string{"node"}, true, false)
	if got := readFile(t, nvmrc); got != "v24.2.0\n" {
		t.Errorf(".nvmrc after --major = %q, want v24.2.0", got)
	}
}

func TestRunPinBump_ReportsReadOnlySources(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	nodeIndexServer(t, []map[string]any{{"version": "v24.2.0", "lts": false}})
	npmRegistryServer(t, []string{"9.0.0", "9.15.0", "10.0.0"})
	dir := t.TempDir()
	t.Chdir(dir)
	pkg := "{\n  \"name\": \"app\",\n  \"packageManager\": \"pnpm@9.0.0+sha512.abc\",\n  \"engines\": {\"node\": \">=18\"}\n}\n"
	pkgPath := filepath.Join(dir, "package.json")
	writeFile(t, pkgPath, pkg)

	out := bumpOutput(t, nil, false, false)
	if got := readFile(t, pkgPath); got != pkg {
		t.Errorf("package.json was rewritten:\n%s", got)
	}
	if !strings.Contains(out, "engines.node") || !strings.Contains(out, "24.2.0") {
		t.Errorf("output = %q, want the engines.node report with the newest release", out)
	}
	if !strings.Contains(out, "corepack use pnpm@9.15.0") {
		t.Errorf("output = %q, want a corepack hint for pnpm@9.15.0", out)
	}
}

func TestRunPinBump_NoPins(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())
	out := bumpOutput(t, []string{"yarn"}, false, false)
	if !strings.Contains(out, "yarn: not pinned") || !strings.Contains(out, "Nothing to bump") {
		t.Errorf("output = %q", out)
	}
}

func TestPinCmd_BumpFlagRules(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"pin", "--bump", "node@22"}, "--bump takes tool names"},
		{[]string{"pin", "--bump", "--migrate"}, "cannot be combined"},
		{[]string{"pin", "--major", "node@22"}, "only apply to --bump"},
		{[]string{"pin", "--dry-run", "node@22"}, "only apply to --bump"},
		{[]string{"pin", "--bump", "ruby"}, `unknown tool "ruby"`},
	}
	for _, tt := range tests {
		err := runCmd(t, tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: err = %v, want it to contain %q", tt.args, err, tt.want)
		}
	}
}
