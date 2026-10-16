package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/shim"
	"github.com/stackmade/driftr/internal/testutil"
)

func TestBinDirOnPath(t *testing.T) {
	binDir := "/home/user/.driftr/bin"

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact entry", "/usr/bin:" + binDir, true},
		{"only entry", binDir, true},
		{"unnormalised entry", "/usr/bin:/home/user/.driftr/bin/", true},
		{"absent", "/usr/bin:/usr/local/bin", false},
		{"empty PATH", "", false},
		{"prefix of another dir", "/home/user/.driftr/binaries", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", tt.path)
			if got := binDirOnPath(binDir); got != tt.want {
				t.Errorf("binDirOnPath(%q) with PATH=%q = %v, want %v", binDir, tt.path, got, tt.want)
			}
		})
	}
}

func TestCheckPath_CountsMissingBinDirAsAnIssue(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")

	t.Setenv("PATH", "/usr/bin")
	var issues int
	out := captureStdout(t, func() { issues = checkPath(binDir) })
	if issues != 1 {
		t.Errorf("checkPath() = %d, want 1 when the shim dir is not on PATH", issues)
	}
	if !strings.Contains(out, "not on PATH") {
		t.Errorf("output does not explain the problem: %q", out)
	}

	t.Setenv("PATH", "/usr/bin:"+binDir)
	out = captureStdout(t, func() { issues = checkPath(binDir) })
	if issues != 0 {
		t.Errorf("checkPath() = %d, want 0 when the shim dir is on PATH", issues)
	}
	if !strings.Contains(out, binDir) {
		t.Errorf("output does not name the shim dir: %q", out)
	}
}

func TestBrokenShims_MissingAndNotExecutable(t *testing.T) {
	binDir := t.TempDir()
	tools := shim.ShimTools()
	if len(tools) < 2 {
		t.Skipf("need at least two shim tools, got %v", tools)
	}

	if err := os.WriteFile(filepath.Join(binDir, tools[0]), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, tools[1]), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	broken := brokenShims(binDir)
	if len(broken) != len(tools)-1 {
		t.Fatalf("brokenShims() = %v, want every tool but %q", broken, tools[0])
	}
	for _, b := range broken {
		if b == tools[0] {
			t.Errorf("%q is a working shim but was reported broken", b)
		}
	}
	if !slices.Contains(broken, tools[1]) {
		t.Errorf("non-executable shim %q was not reported broken", tools[1])
	}
}

func TestCheckShims_FixRegeneratesEveryShim(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	binDir := filepath.Join(home, ".driftr", "bin")

	var issues int
	captureStdout(t, func() { issues = checkShims(binDir, false) })
	if issues != len(shim.ShimTools()) {
		t.Fatalf("checkShims(fix=false) = %d, want %d broken shims", issues, len(shim.ShimTools()))
	}

	captureStdout(t, func() { issues = checkShims(binDir, true) })
	if issues != 0 {
		t.Fatalf("checkShims(fix=true) = %d, want 0 after regeneration", issues)
	}
	for _, tool := range shim.ShimTools() {
		info, err := os.Stat(filepath.Join(binDir, tool))
		if err != nil {
			t.Errorf("shim %s was not regenerated: %v", tool, err)
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("shim %s is not executable: mode %v", tool, info.Mode())
		}
	}
}

func TestCheckGlobalDefault(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.GlobalConfig
		cfgErr     error
		want       int
		wantOutput string
	}{
		{
			name:       "unreadable config",
			cfgErr:     os.ErrPermission,
			want:       1,
			wantOutput: "Cannot read global config",
		},
		{
			name:       "no default set",
			cfg:        &config.GlobalConfig{},
			want:       1,
			wantOutput: "No global default node version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int
			out := captureStdout(t, func() { got = checkGlobalDefault(tt.cfg, tt.cfgErr) })
			if got != tt.want {
				t.Errorf("checkGlobalDefault() = %d, want %d", got, tt.want)
			}
			if !strings.Contains(out, tt.wantOutput) {
				t.Errorf("output %q does not contain %q", out, tt.wantOutput)
			}
		})
	}
}

// A default uninstalled behind driftr's back is the failure mode this check exists for.
func TestCheckDefaultsInstalled_ReportsMissingInstall(t *testing.T) {
	testutil.SetHome(t, t.TempDir())

	cfg := &config.GlobalConfig{}
	cfg.Default.SetTool("node", "22.14.0")

	var issues int
	out := captureStdout(t, func() { issues = checkDefaultsInstalled(cfg) })
	if issues != 1 {
		t.Fatalf("checkDefaultsInstalled() = %d, want 1", issues)
	}
	if !strings.Contains(out, "driftr install node@22.14.0") {
		t.Errorf("output does not suggest the fix: %q", out)
	}

	fakeNodeInstall(t, "22.14.0")
	out = captureStdout(t, func() { issues = checkDefaultsInstalled(cfg) })
	if issues != 0 {
		t.Errorf("checkDefaultsInstalled() = %d after install, want 0", issues)
	}
	if !strings.Contains(out, "All default versions are installed") {
		t.Errorf("unexpected output once installed: %q", out)
	}
}

// pnpm and yarn are JS scripts: without node they cannot run at all.
func TestCheckNeedsNode(t *testing.T) {
	tests := []struct {
		name     string
		versions map[string][]string
		want     int
	}{
		{"node present", map[string][]string{"node": {"22.14.0"}, "pnpm": {"9.15.0"}}, 0},
		{"pnpm without node", map[string][]string{"pnpm": {"9.15.0"}}, 1},
		{"pnpm and yarn without node", map[string][]string{"pnpm": {"9.15.0"}, "yarn": {"1.22.22"}}, 2},
		{"nothing installed", map[string][]string{}, 0},
		{"bun alone needs no node", map[string][]string{"bun": {"1.2.3"}}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int
			captureStdout(t, func() { got = checkNeedsNode(tt.versions) })
			if got != tt.want {
				t.Errorf("checkNeedsNode(%v) = %d, want %d", tt.versions, got, tt.want)
			}
		})
	}
}

func zshHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	return home
}

func TestCheckShellRCPlacement_FixWritesZshenv(t *testing.T) {
	home := zshHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")

	var issues int
	out := captureStdout(t, func() { issues = checkShellRCPlacement(binDir, false) })
	if issues != 1 || !strings.Contains(out, "not configured in any shell rc file") {
		t.Fatalf("issues = %d, out = %q", issues, out)
	}

	out = captureStdout(t, func() { issues = checkShellRCPlacement(binDir, true) })
	if issues != 0 || !strings.Contains(out, "fixed") {
		t.Fatalf("fix: issues = %d, out = %q", issues, out)
	}
	if data := readFile(t, filepath.Join(home, ".zshenv")); !strings.Contains(data, binDir) {
		t.Fatalf(".zshenv lacks %s: %q", binDir, data)
	}

	out = captureStdout(t, func() { issues = checkShellRCPlacement(binDir, false) })
	if issues != 0 || !strings.Contains(out, "universal shell coverage") {
		t.Errorf("after fix: issues = %d, out = %q", issues, out)
	}
}

func TestCheckShellRCPlacement_FlagsInteractiveOnlyEntry(t *testing.T) {
	home := zshHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")
	writeFile(t, filepath.Join(home, ".zshrc"), "export PATH=\"$HOME/.driftr/bin:$PATH\"\n")

	var issues int
	out := captureStdout(t, func() { issues = checkShellRCPlacement(binDir, false) })
	if issues != 1 {
		t.Errorf("issues = %d, want 1", issues)
	}
	if !strings.Contains(out, "interactive rc file(s): "+filepath.Join(home, ".zshrc")) {
		t.Errorf("output does not name .zshrc: %q", out)
	}
}

func TestCheckShimVersion(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)

	var issues int
	out := captureStdout(t, func() { issues = checkShimVersion(false) })
	if issues != 1 || !strings.Contains(out, "an older driftr") {
		t.Fatalf("no stamp: issues = %d, out = %q", issues, out)
	}

	if _, err := shim.GenerateShims("0.9.0"); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() { issues = checkShimVersion(false) })
	if issues != 1 || !strings.Contains(out, "written by driftr 0.9.0") {
		t.Fatalf("old stamp: issues = %d, out = %q", issues, out)
	}

	captureStdout(t, func() { issues = checkShimVersion(true) })
	if issues != 0 || shim.Stamp() != Version {
		t.Fatalf("fix: issues = %d, stamp = %q, want %q", issues, shim.Stamp(), Version)
	}
	out = captureStdout(t, func() { issues = checkShimVersion(false) })
	if issues != 0 || !strings.Contains(out, "match") {
		t.Errorf("after fix: issues = %d, out = %q", issues, out)
	}
}

func TestCheckShimBinaryPath(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	binDir := filepath.Join(home, ".driftr", "bin")

	if got := checkShimBinaryPath(binDir, false); got != 0 {
		t.Fatalf("missing shim = %d, want 0 (checkShims reports it)", got)
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, "node"), "#!/bin/sh\nexec '/opt/old/driftr' shim node \"$@\"\n")
	var issues int
	out := captureStdout(t, func() { issues = checkShimBinaryPath(binDir, false) })
	if issues != 1 || !strings.Contains(out, "Shims point to /opt/old/driftr") {
		t.Fatalf("stale shim: issues = %d, out = %q", issues, out)
	}

	captureStdout(t, func() { issues = checkShimBinaryPath(binDir, true) })
	if issues != 0 {
		t.Fatalf("fix: issues = %d", issues)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	got, _ := filepath.EvalSymlinks(shim.BakedPath(filepath.Join(binDir, "node")))
	if got != exe {
		t.Errorf("shim points to %q, want %q", got, exe)
	}
	out = captureStdout(t, func() { issues = checkShimBinaryPath(binDir, false) })
	if issues != 0 || !strings.Contains(out, "current driftr binary") {
		t.Errorf("after fix: issues = %d, out = %q", issues, out)
	}
}

func TestCheckConflictingManagers(t *testing.T) {
	binDir := t.TempDir()
	otherDir := t.TempDir()
	nvmDir := t.TempDir()
	writeExe(t, otherDir, "fnm")
	writeExe(t, binDir, "volta")
	writeFile(t, filepath.Join(nvmDir, "nvm.sh"), "")

	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", "")
	var issues int
	out := captureStdout(t, func() { issues = checkConflictingManagers(binDir) })
	if issues != 0 || !strings.Contains(out, "No conflicting") {
		t.Fatalf("clean env: issues = %d, out = %q", issues, out)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+otherDir)
	t.Setenv("NVM_DIR", nvmDir)
	out = captureStdout(t, func() { issues = checkConflictingManagers(binDir) })
	if issues != 2 {
		t.Errorf("issues = %d, want 2 (nvm and fnm; volta in binDir is ours)", issues)
	}
	for _, want := range []string{"nvm detected", "fnm detected at " + filepath.Join(otherDir, "fnm")} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q: %q", want, out)
		}
	}
	if strings.Contains(out, "volta") {
		t.Errorf("volta inside the shim dir was reported: %q", out)
	}
}
