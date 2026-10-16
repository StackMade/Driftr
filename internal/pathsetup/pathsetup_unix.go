//go:build !windows

package pathsetup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/internal/platform"
)

// Detect inspects the user's shell configuration and reports the state.
func Detect(binDir string) (Result, error) {
	shell := DetectShell()
	target, err := TargetProfile(shell)
	if err != nil {
		return Result{}, err
	}

	needles := binDirNeedles(binDir)

	inTarget, err := fileMentionsAny(target, needles)
	if err != nil {
		return Result{}, err
	}

	stale, err := scanStale(shell, target, needles)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Shell:         shell,
		BinDir:        binDir,
		Target:        target,
		InTarget:      inTarget,
		StaleFiles:    stale,
		InProcessPATH: pathContains(os.Getenv("PATH"), binDir),
	}, nil
}

// binDirNeedles returns the substrings that may represent binDir in an rc
// file. Users (and the installer) often write the path home-relative using
// $HOME or ~, so we look for all three forms.
func binDirNeedles(binDir string) []string {
	needles := []string{binDir}
	// The quoted fish line escapes ' and \, so look for that form too.
	if escaped := fishEscaper.Replace(binDir); escaped != binDir {
		needles = append(needles, escaped)
	}
	home, err := platform.HomeDir()
	if err != nil || home == "" {
		return needles
	}
	cleanHome := filepath.Clean(home)
	cleanBin := filepath.Clean(binDir)
	if strings.HasPrefix(cleanBin, cleanHome+string(filepath.Separator)) {
		rel := strings.TrimPrefix(cleanBin, cleanHome)
		needles = append(needles, "$HOME"+rel, "~"+rel, "${HOME}"+rel)
	}
	return needles
}

// Apply writes the PATH export to the recommended target file if missing.
// Returns (true, target) when it wrote, (false, "") when no change was needed.
// Never removes entries from stale files — that's left to the user.
func Apply(r Result) (wrote bool, target string, err error) {
	if !r.NeedsFix() {
		return false, "", nil
	}

	if err = os.MkdirAll(filepath.Dir(r.Target), 0o755); err != nil {
		return false, "", fmt.Errorf("create profile dir: %w", err)
	}

	line := exportLine(r.Shell, r.BinDir)
	f, ferr := os.OpenFile(r.Target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if ferr != nil {
		return false, "", fmt.Errorf("open %s: %w", r.Target, ferr)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", r.Target, cerr)
			wrote = false
			target = ""
		}
	}()

	// Leading newline when file has content to avoid gluing to prior line.
	info, _ := f.Stat()
	prefix := "\n"
	if info != nil && info.Size() == 0 {
		prefix = ""
	}
	if _, werr := fmt.Fprintf(f, "%s# Driftr\n%s\n", prefix, line); werr != nil {
		return false, "", fmt.Errorf("write %s: %w", r.Target, werr)
	}

	return true, r.Target, nil
}

// GuardProfile returns the interactive rc file that is sourced *last* for the
// shell — the right place for a PATH "precedence guard" that must win over
// dirs prepended by other tools (Homebrew, Volta) during interactive startup.
//
// This differs from TargetProfile (the universally-sourced file): the target
// guarantees coverage in non-interactive shells, while the guard guarantees
// precedence in interactive ones. Returns "" for shells where no guard applies.
func GuardProfile(shell Shell) (string, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	switch shell {
	case ShellZsh:
		zdotdir := os.Getenv("ZDOTDIR")
		if zdotdir == "" {
			zdotdir = home
		}
		return filepath.Join(zdotdir, ".zshrc"), nil
	case ShellBash:
		return filepath.Join(home, ".bashrc"), nil
	case ShellFish:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "fish", "config.fish"), nil
	default:
		return "", nil
	}
}

// ApplyGuard appends a PATH precedence guard to the shell's interactive rc file
// (see GuardProfile). It is idempotent: if the file already exports binDir, it
// writes nothing. Returns (true, file) when it wrote, (false, "") otherwise.
//
// Use only when binDir is on PATH but shadowed by an earlier entry — appending
// the export last lets driftr's shims win. The universal config (Apply) is
// still required for non-interactive coverage.
func ApplyGuard(shell Shell, binDir string) (wrote bool, file string, err error) {
	guard, err := GuardProfile(shell)
	if err != nil {
		return false, "", err
	}
	if guard == "" {
		return false, "", nil
	}

	mentioned, err := fileMentionsAny(guard, binDirNeedles(binDir))
	if err != nil {
		return false, "", err
	}
	if mentioned {
		return false, "", nil // already present — nothing to do
	}

	if err = os.MkdirAll(filepath.Dir(guard), 0o755); err != nil {
		return false, "", fmt.Errorf("create profile dir: %w", err)
	}

	f, ferr := os.OpenFile(guard, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if ferr != nil {
		return false, "", fmt.Errorf("open %s: %w", guard, ferr)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", guard, cerr)
			wrote = false
			file = ""
		}
	}()

	info, _ := f.Stat()
	prefix := "\n"
	if info != nil && info.Size() == 0 {
		prefix = ""
	}
	line := exportLine(shell, binDir)
	if _, werr := fmt.Fprintf(f, "%s# Driftr (precedence guard — keep last)\n%s\n", prefix, line); werr != nil {
		return false, "", fmt.Errorf("write %s: %w", guard, werr)
	}

	return true, guard, nil
}

// TargetProfile returns the recommended rc file to export PATH from for the
// widest shell coverage. For zsh this is .zshenv (every invocation). For fish
// this is conf.d (every invocation). For bash this is .bash_profile (login
// shells); non-interactive children inherit PATH from the login shell env, so
// coverage is not truly universal — it depends on the terminal launching a
// login shell, which most macOS and Linux terminal emulators do.
func TargetProfile(shell Shell) (string, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	switch shell {
	case ShellZsh:
		if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
			return filepath.Join(zdotdir, ".zshenv"), nil
		}
		return filepath.Join(home, ".zshenv"), nil
	case ShellBash:
		// .bash_profile is read by login shells, which most macOS/Linux
		// terminal sessions are. Non-interactive children inherit PATH
		// from the login shell env.
		return filepath.Join(home, ".bash_profile"), nil
	case ShellFish:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "fish", "conf.d", "driftr.fish"), nil
	default:
		return filepath.Join(home, ".profile"), nil
	}
}

// StaleCandidates returns rc files that may contain legacy PATH entries.
func StaleCandidates(shell Shell) ([]string, error) {
	home, err := platform.HomeDir()
	if err != nil {
		return nil, err
	}

	switch shell {
	case ShellZsh:
		zdotdir := os.Getenv("ZDOTDIR")
		if zdotdir == "" {
			zdotdir = home
		}
		return []string{
			filepath.Join(zdotdir, ".zshrc"),
			filepath.Join(zdotdir, ".zprofile"),
			filepath.Join(home, ".profile"),
		}, nil
	case ShellBash:
		return []string{
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".profile"),
		}, nil
	case ShellFish:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return []string{filepath.Join(base, "fish", "config.fish")}, nil
	default:
		return nil, nil
	}
}

func scanStale(shell Shell, target string, needles []string) ([]string, error) {
	candidates, err := StaleCandidates(shell)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, c := range candidates {
		if c == target {
			continue
		}
		ok, err := fileMentionsAny(c, needles)
		if err != nil {
			return nil, err
		}
		if ok {
			stale = append(stale, c)
		}
	}
	return stale, nil
}

func fileMentionsAny(path string, needles []string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		// A commented-out export configures nothing.
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			for _, n := range needles {
				if namesDir(line, n) {
					return true, nil
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
	}
}

// namesDir reports whether line contains dir as a whole path, not as the
// prefix of a longer one such as ".driftr/bin-legacy".
func namesDir(line, dir string) bool {
	for i := 0; ; {
		j := strings.Index(line[i:], dir)
		if j < 0 {
			return false
		}
		end := i + j + len(dir)
		if end == len(line) || strings.ContainsRune(":\"' \t\r\n;)}", rune(line[end])) {
			return true
		}
		i += j + 1
	}
}

// fishEscaper escapes the two characters fish treats specially inside single
// quotes.
var fishEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func exportLine(shell Shell, binDir string) string {
	if shell == ShellFish {
		return fmt.Sprintf("set -gx PATH '%s' $PATH", fishEscaper.Replace(binDir))
	}
	return fmt.Sprintf(`export PATH="%s:$PATH"`, binDir)
}
