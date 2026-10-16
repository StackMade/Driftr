//go:build !windows

package pathsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/testutil"
)

func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.SetHome(t, dir)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return dir
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		sh   string
		want Shell
	}{
		{"/bin/zsh", ShellZsh},
		{"/usr/bin/bash", ShellBash},
		{"/opt/homebrew/bin/fish", ShellFish},
		{"/bin/dash", ShellUnknown},
		{"", ShellUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.sh, func(t *testing.T) {
			t.Setenv("SHELL", tc.sh)
			if got := DetectShell(); got != tc.want {
				t.Errorf("DetectShell(%q) = %v, want %v", tc.sh, got, tc.want)
			}
		})
	}
}

func TestTargetProfile_Zsh(t *testing.T) {
	home := setHome(t)
	got, err := TargetProfile(ShellZsh)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".zshenv")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

func TestTargetProfile_ZshHonorsZDOTDIR(t *testing.T) {
	setHome(t)
	custom := t.TempDir()
	t.Setenv("ZDOTDIR", custom)
	got, err := TargetProfile(ShellZsh)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(custom, ".zshenv")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

func TestTargetProfile_Bash(t *testing.T) {
	home := setHome(t)
	got, err := TargetProfile(ShellBash)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".bash_profile")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

func TestTargetProfile_Fish(t *testing.T) {
	home := setHome(t)
	got, err := TargetProfile(ShellFish)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "fish", "conf.d", "driftr.fish")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

func TestTargetProfile_FishHonorsXDG(t *testing.T) {
	setHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := TargetProfile(ShellFish)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "fish", "conf.d", "driftr.fish")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

func TestDetect_EmptyEnv(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	binDir := filepath.Join(home, ".driftr", "bin")
	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Shell != ShellZsh {
		t.Errorf("shell = %v, want zsh", r.Shell)
	}
	if r.InTarget {
		t.Errorf("InTarget = true, want false (.zshenv does not exist)")
	}
	if len(r.StaleFiles) != 0 {
		t.Errorf("StaleFiles = %v, want empty", r.StaleFiles)
	}
	if r.InProcessPATH {
		t.Errorf("InProcessPATH = true, want false")
	}
	if !r.NeedsFix() {
		t.Errorf("NeedsFix = false, want true")
	}
}

func TestDetect_StaleInZshrc(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")
	zshrc := filepath.Join(home, ".zshrc")
	writeFile(t, zshrc, "# legacy\nexport PATH=\""+binDir+":$PATH\"\n")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if r.InTarget {
		t.Errorf("InTarget = true, want false")
	}
	if len(r.StaleFiles) != 1 || r.StaleFiles[0] != zshrc {
		t.Errorf("StaleFiles = %v, want [%s]", r.StaleFiles, zshrc)
	}
	if !r.NeedsFix() {
		t.Errorf("NeedsFix = false, want true")
	}
}

func TestDetect_AlreadyInTarget(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")
	zshenv := filepath.Join(home, ".zshenv")
	writeFile(t, zshenv, "export PATH=\""+binDir+":$PATH\"\n")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.InTarget {
		t.Errorf("InTarget = false, want true")
	}
	if r.NeedsFix() {
		t.Errorf("NeedsFix = true, want false")
	}
}

func TestApply_CreatesZshenv(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")
	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}

	wrote, file, err := Apply(r)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("Apply did not write")
	}
	want := filepath.Join(home, ".zshenv")
	if file != want {
		t.Errorf("wrote to %q, want %q", file, want)
	}
	content := readFile(t, file)
	if !strings.Contains(content, "export PATH=\""+binDir+":$PATH\"") {
		t.Errorf("content missing export line:\n%s", content)
	}
	if !strings.Contains(content, "# Driftr") {
		t.Errorf("content missing # Driftr marker:\n%s", content)
	}
}

func TestApply_NoOpWhenInTarget(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")
	zshenv := filepath.Join(home, ".zshenv")
	original := "export PATH=\"" + binDir + ":$PATH\"\n"
	writeFile(t, zshenv, original)

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	wrote, file, err := Apply(r)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Errorf("Apply wrote when target already had entry")
	}
	if file != "" {
		t.Errorf("file = %q, want empty", file)
	}
	if got := readFile(t, zshenv); got != original {
		t.Errorf("file was modified:\n%s", got)
	}
}

func TestApply_AppendsWithLeadingNewline(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")
	zshenv := filepath.Join(home, ".zshenv")
	writeFile(t, zshenv, "export FOO=bar\n")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Apply(r); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, zshenv)
	wantPrefix := "export FOO=bar\n\n# Driftr\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("content does not preserve prior line with newline separator:\n%s", got)
	}
}

func TestApply_Fish(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/opt/homebrew/bin/fish")

	binDir := filepath.Join(home, ".driftr", "bin")
	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	wrote, file, err := Apply(r)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("Apply did not write")
	}
	want := filepath.Join(home, ".config", "fish", "conf.d", "driftr.fish")
	if file != want {
		t.Errorf("wrote to %q, want %q", file, want)
	}
	content := readFile(t, file)
	if !strings.Contains(content, "set -gx PATH '"+binDir+"' $PATH") {
		t.Errorf("content missing fish syntax:\n%s", content)
	}
}

func TestDetect_HomeRelativeForms(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := filepath.Join(home, ".driftr", "bin")

	// Each rc file uses a different form of the home-relative path.
	writeFile(t, filepath.Join(home, ".zshenv"), "export PATH=\"$HOME/.driftr/bin:$PATH\"\n")
	writeFile(t, filepath.Join(home, ".zshrc"), "export PATH=\"~/.driftr/bin:$PATH\"\n")
	writeFile(t, filepath.Join(home, ".zprofile"), "export PATH=\"${HOME}/.driftr/bin:$PATH\"\n")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.InTarget {
		t.Errorf("InTarget = false, want true ($HOME form in .zshenv)")
	}
	// Both .zshrc (~) and .zprofile (${HOME}) should register as stale.
	if len(r.StaleFiles) != 2 {
		t.Errorf("StaleFiles = %v, want 2 entries (~ and ${HOME} forms)", r.StaleFiles)
	}
}

func TestDetect_InProcessPATH(t *testing.T) {
	home := setHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", binDir+":/usr/bin")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.InProcessPATH {
		t.Errorf("InProcessPATH = false, want true")
	}
}

func TestGuardProfile(t *testing.T) {
	home := setHome(t)
	cases := map[Shell]string{
		ShellZsh:     filepath.Join(home, ".zshrc"),
		ShellBash:    filepath.Join(home, ".bashrc"),
		ShellFish:    filepath.Join(home, ".config", "fish", "config.fish"),
		ShellUnknown: "",
	}
	for shell, want := range cases {
		got, err := GuardProfile(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if got != want {
			t.Errorf("GuardProfile(%s) = %q, want %q", shell, got, want)
		}
	}
}

func TestApplyGuard_AppendsToInteractiveRc(t *testing.T) {
	home := setHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")
	zshrc := filepath.Join(home, ".zshrc")
	writeFile(t, zshrc, "eval \"$(/opt/homebrew/bin/brew shellenv)\"\n")

	wrote, file, err := ApplyGuard(ShellZsh, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote || file != zshrc {
		t.Fatalf("ApplyGuard wrote=%v file=%q, want true %q", wrote, file, zshrc)
	}

	content := readFile(t, zshrc)
	// Guard must come AFTER the brew line to win precedence.
	brewIdx := strings.Index(content, "brew shellenv")
	guardIdx := strings.Index(content, "export PATH=\""+binDir+":$PATH\"")
	if brewIdx < 0 || guardIdx < 0 || guardIdx < brewIdx {
		t.Errorf("guard must be appended after existing content:\n%s", content)
	}
	if !strings.Contains(content, "precedence guard") {
		t.Errorf("missing guard marker:\n%s", content)
	}
}

func TestApplyGuard_Idempotent(t *testing.T) {
	home := setHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")
	zshrc := filepath.Join(home, ".zshrc")
	writeFile(t, zshrc, "export PATH=\""+binDir+":$PATH\"\n")

	wrote, _, err := ApplyGuard(ShellZsh, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("ApplyGuard wrote when binDir already exported in guard file")
	}
}

func TestApplyGuard_UnknownShellNoOp(t *testing.T) {
	home := setHome(t)
	binDir := filepath.Join(home, ".driftr", "bin")

	wrote, file, err := ApplyGuard(ShellUnknown, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote || file != "" {
		t.Errorf("ApplyGuard(unknown) wrote=%v file=%q, want no-op", wrote, file)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A disabled line or a longer dir that starts with binDir does not put driftr on PATH.
func TestDetect_IgnoresCommentsAndLongerPaths(t *testing.T) {
	for _, content := range []string{
		"# export PATH=\"$HOME/.driftr/bin:$PATH\"  (disabled)\n",
		"  #export PATH=\"$HOME/.driftr/bin:$PATH\"\n",
		"export PATH=\"$HOME/.driftr/bin-legacy:$PATH\"\n",
		"export PATH=\"$HOME/.driftr/binaries:$HOME/.driftr/bin.old:$PATH\"\n",
	} {
		home := setHome(t)
		t.Setenv("SHELL", "/bin/zsh")
		writeFile(t, filepath.Join(home, ".zshenv"), content)

		r, err := Detect(filepath.Join(home, ".driftr", "bin"))
		if err != nil {
			t.Fatal(err)
		}
		if r.InTarget {
			t.Errorf("InTarget = true for %q", content)
		}
	}
}

func TestNamesDir(t *testing.T) {
	const dir = "$HOME/.driftr/bin"
	for line, want := range map[string]bool{
		`export PATH="$HOME/.driftr/bin:$PATH"`:                       true,
		`export PATH=$HOME/.driftr/bin`:                               true,
		"set -gx PATH $HOME/.driftr/bin $PATH":                        true,
		"set -gx PATH '$HOME/.driftr/bin' $PATH":                      true,
		`PATH="${PATH}:$HOME/.driftr/bin"; export PATH`:               true,
		`export PATH="$HOME/.driftr/bin-old:$HOME/.driftr/bin:$PATH"`: true,
		`export PATH="$HOME/.driftr/bin-old:$PATH"`:                   false,
		`export PATH="$HOME/.driftr/binx:$PATH"`:                      false,
	} {
		if got := namesDir(line, dir); got != want {
			t.Errorf("namesDir(%q) = %v, want %v", line, got, want)
		}
	}
}

// fish splits a bare word on spaces, so an unquoted "/Users/John Doe" became two PATH entries.
func TestApply_FishQuotesBinDir(t *testing.T) {
	for _, dirName := range []string{"John Doe", `O'Brien`, `back\slash`} {
		home := filepath.Join(t.TempDir(), dirName)
		testutil.SetHome(t, home)
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("SHELL", "/usr/bin/fish")
		binDir := filepath.Join(home, ".driftr", "bin")

		r, err := Detect(binDir)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := Apply(r); err != nil {
			t.Fatal(err)
		}
		content := readFile(t, r.Target)
		want := "set -gx PATH '" + fishEscaper.Replace(binDir) + "' $PATH"
		if !strings.Contains(content, want) {
			t.Errorf("%s: want %q in:\n%s", dirName, want, content)
		}

		// The line just written counts as configured, so a rerun is a no-op.
		r, err = Detect(binDir)
		if err != nil {
			t.Fatal(err)
		}
		if !r.InTarget {
			t.Errorf("%s: quoted fish line not detected", dirName)
		}
	}
}

// Lines written by older driftr versions, and by install.sh, are unquoted.
func TestDetect_FishUnquotedLegacyLine(t *testing.T) {
	home := setHome(t)
	t.Setenv("SHELL", "/usr/bin/fish")
	binDir := filepath.Join(home, ".driftr", "bin")
	writeFile(t, filepath.Join(home, ".config", "fish", "conf.d", "driftr.fish"),
		"# Driftr\nset -gx PATH "+binDir+" $PATH\n")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.InTarget {
		t.Error("unquoted fish line not detected")
	}
}

func TestRootOwnedRcFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not bind root")
	}
	home := setHome(t)
	t.Setenv("SHELL", "/bin/zsh")
	binDir := filepath.Join(home, ".driftr", "bin")
	zshenv := filepath.Join(home, ".zshenv")
	zshrc := filepath.Join(home, ".zshrc")
	writeFile(t, zshenv, "export EDITOR=vim\n")
	writeFile(t, zshrc, "")
	os.Chmod(zshenv, 0o444)
	os.Chmod(zshrc, 0o000)
	t.Cleanup(func() { os.Chmod(zshrc, 0o644) })

	if _, err := Detect(binDir); err == nil {
		t.Error("Detect ignored an unreadable .zshrc")
	}
	if _, _, err := ApplyGuard(ShellZsh, binDir); err == nil {
		t.Error("ApplyGuard ignored an unreadable .zshrc")
	}

	os.Chmod(zshrc, 0o644)
	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote, _, err := Apply(r); err == nil || wrote {
		t.Errorf("Apply to read-only .zshenv: wrote=%v err=%v", wrote, err)
	}
	if got := readFile(t, zshenv); got != "export EDITOR=vim\n" {
		t.Errorf(".zshenv changed: %q", got)
	}
}

func TestApply_NoModifyPath(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	t.Setenv("DRIFTR_NO_MODIFY_PATH", "1")
	binDir := filepath.Join(home, ".driftr", "bin")

	r, err := Detect(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote, _, err := Apply(r); wrote || err == nil {
		t.Errorf("Apply() = %v, %v; want a refusal", wrote, err)
	}
	if wrote, _, err := ApplyGuard(ShellZsh, binDir); wrote || err == nil {
		t.Errorf("ApplyGuard() = %v, %v; want a refusal", wrote, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshenv")); !os.IsNotExist(err) {
		t.Errorf(".zshenv written anyway: %v", err)
	}
}
