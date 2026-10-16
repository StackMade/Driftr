package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
)

// ExitError signals that the process should exit with a specific code
// without printing an error message (e.g. forwarding a child process exit code).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

// Build metadata, injected via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var verbose bool

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "driftr",
		Short:         "Fast, cross-platform JavaScript toolchain manager",
		Long:          "Driftr manages tool versions with automatic per-project resolution and shim-based execution.",
		Version:       fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	root.AddCommand(
		newInstallCmd(),
		newUninstallCmd(),
		newPruneCmd(),
		newDefaultCmd(),
		newPinCmd(),
		newUseCmd(),
		newListCmd(),
		newOutdatedCmd(),
		newCheckCmd(),
		newWhichCmd(),
		newRunCmd(),
		newShimCmd(),
		newSetupCmd(),
		newCacheCmd(),
		newDoctorCmd(),
		newUpdateCmd(),
		newNodeCmd(),
	)
	setColorHelp(root)

	return root
}

func Execute() {
	if err := RefreshStaleShims(); err != nil {
		fmt.Fprintln(os.Stderr, ioutil.Warn(fmt.Sprintf("Could not update the shims for this driftr version: %s. Run `driftr doctor`", err)))
	}
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", ioutil.Red("Error:"), ioutil.Commands(err.Error()))
		os.Exit(1)
	}
}
