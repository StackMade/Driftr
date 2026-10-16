// Package pathsetup detects and repairs shell PATH configuration for driftr.
//
// The goal is "work everywhere": driftr must be on PATH in interactive shells,
// non-interactive shells, scripts, cron, and IDE subprocesses. On Unix that
// requires PATH to be exported from a file that every invocation of the shell
// sources (e.g. .zshenv for zsh), not only interactive rc files (.zshrc,
// .bashrc). On Windows it is the user's Path in the registry, which every new
// process inherits whatever the shell.
package pathsetup

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/stackmade/driftr/internal/platform"
)

// Shell is the detected user shell.
type Shell string

const (
	ShellZsh     Shell = "zsh"
	ShellBash    Shell = "bash"
	ShellFish    Shell = "fish"
	ShellUnknown Shell = "unknown"
)

// Result describes the current PATH configuration state for the given binDir.
type Result struct {
	Shell         Shell
	BinDir        string
	Target        string   // recommended rc file for universal coverage
	InTarget      bool     // target file already configures binDir
	StaleFiles    []string // other rc files that configure binDir
	InProcessPATH bool     // current process's PATH includes binDir
}

// NeedsFix returns true when binDir is not configured in the recommended
// target file. Stale entries in other rc files still count as "needs fix"
// because they won't cover non-interactive shells.
func (r Result) NeedsFix() bool {
	return !r.InTarget
}

// errModifyDisabled is what Apply and ApplyGuard return when
// DRIFTR_NO_MODIFY_PATH is set: the user keeps PATH themselves, e.g. in
// dotfiles they manage, and driftr only says what to add.
var errModifyDisabled = errors.New("DRIFTR_NO_MODIFY_PATH is set, so driftr leaves PATH to you")

func modifyDisabled() bool {
	return os.Getenv("DRIFTR_NO_MODIFY_PATH") != ""
}

// DetectShell returns the user's shell based on $SHELL, defaulting to unknown.
func DetectShell() Shell {
	sh := os.Getenv("SHELL")
	if sh == "" {
		return ShellUnknown
	}
	switch filepath.Base(sh) {
	case "zsh":
		return ShellZsh
	case "bash":
		return ShellBash
	case "fish":
		return ShellFish
	default:
		return ShellUnknown
	}
}

// pathContains reports whether the PATH list pathEnv has binDir as an entry.
func pathContains(pathEnv, binDir string) bool {
	for _, dir := range filepath.SplitList(pathEnv) {
		if platform.SamePath(dir, binDir) {
			return true
		}
	}
	return false
}
