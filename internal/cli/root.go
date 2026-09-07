package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
		newWhichCmd(),
		newRunCmd(),
		newShimCmd(),
		newSetupCmd(),
		newCacheCmd(),
		newDoctorCmd(),
		newUpdateCmd(),
		newNodeCmd(),
	)

	return root
}

func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
