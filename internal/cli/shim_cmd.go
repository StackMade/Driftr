package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/process"
	"github.com/stackmade/driftr/internal/resolver"
	"github.com/stackmade/driftr/internal/shim"
)

// newShimCmd creates the hidden `driftr shim <tool>` command invoked by shim scripts.
func newShimCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "shim <tool>",
		Short:              "Execute a shimmed tool (internal)",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("shim: no tool specified")
			}
			tool := args[0]
			toolArgs := args[1:]

			rb, err := resolver.ResolveBinaryFull(tool, "")
			if err != nil {
				rb, err = HandleShimError(err, tool)
				if err != nil {
					return fmt.Errorf("error: %w", err)
				}
			}

			return ExecResolved(tool, rb, toolArgs)
		},
	}
}

// RefreshStaleShims regenerates the shims once after driftr was upgraded, by
// whatever route. Called on every invocation, shims included, so it must stay
// cheap: one small file read when the shims are current. The shim path drops
// the error, since a warning on every node call would bury the tool's own
// output and the command still runs; driftr commands print it, and
// `driftr doctor` reports the stale shims until they are fixed.
func RefreshStaleShims() error {
	return shim.RefreshIfStale(Version)
}

// ExecResolved replaces the process with a resolved tool. Called from the
// fast path in main.go and from the cobra shim command.
func ExecResolved(tool string, rb *resolver.ResolvedBinary, args []string) error {
	binary, argv, err := prepareResolved(rb, args)
	if err != nil {
		return err
	}

	if tool == "npm" && isGlobalNpm(args) {
		// npm -g adds or removes package bins, so it runs as a child: exec
		// would leave no process behind to refresh the shims afterwards.
		code, err := runResolved(tool, binary, argv, args)
		if err != nil {
			return err
		}
		if code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	}

	return process.Exec(binary, argv)
}

// prepareResolved returns the binary and argv that run a resolved tool. For a
// package bin it puts the bin's node first on PATH, so a "#!/usr/bin/env node"
// script runs under the node it was installed with.
func prepareResolved(rb *resolver.ResolvedBinary, args []string) (string, []string, error) {
	if rb.PathPrefix != "" {
		if err := os.Setenv("PATH", rb.PathPrefix+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
			return "", nil, err
		}
	}
	// For tools that need Node.js (e.g. yarn), run node with the tool script.
	if rb.NodePath != "" {
		return rb.NodePath, append([]string{rb.ToolPath}, args...), nil
	}
	return rb.ToolPath, args, nil
}

// runResolved runs a prepared tool as a child and returns its exit code. After
// npm -g it regenerates the shims, since that is what adds or removes bins.
func runResolved(tool, binary string, argv, args []string) (int, error) {
	code, err := process.Run(binary, argv)
	if err != nil {
		return code, err
	}
	if tool == "npm" && isGlobalNpm(args) {
		skipped, err := shim.GenerateShims(Version)
		if err != nil {
			fmt.Fprintln(os.Stderr, ioutil.Commands(fmt.Sprintf("driftr: could not refresh shims: %s. Run `driftr setup`", err)))
		}
		printSkippedShims(os.Stderr, skipped)
	}
	return code, nil
}

// printSkippedShims names the package bins that got no shim, so a bin that
// keeps running some other install is not a mystery.
func printSkippedShims(w io.Writer, skipped []string) {
	for _, note := range skipped {
		fmt.Fprintln(w, ioutil.Dim("driftr: no shim for "+note))
	}
}

// isGlobalNpm reports whether npm args carry a global flag before any "--".
func isGlobalNpm(args []string) bool {
	for i, a := range args {
		switch {
		case a == "--":
			return false
		case a == "-g", a == "--global", a == "--location=global":
			return true
		case a == "--location" && i+1 < len(args) && args[i+1] == "global":
			return true
		}
	}
	return false
}

// HandleShimError checks if the error is a NotInstalledError and offers to install.
// Returns the resolved binary on successful install, or the original error.
// Called from the fast path in main.go and from the cobra shim command.
func HandleShimError(err error, tool string) (*resolver.ResolvedBinary, error) {
	var notInstalled *resolver.NotInstalledError
	if !errors.As(err, &notInstalled) {
		return nil, err
	}

	// Auto-install if configured.
	cfg, cfgErr := config.LoadGlobal()
	if cfgErr == nil && cfg.AutoInstall {
		return autoInstall(notInstalled, tool)
	}

	// Prompt only in interactive terminals.
	if !ioutil.IsTerminal(os.Stdin) {
		return nil, err
	}

	if !promptInstall(os.Stdin, notInstalled) {
		// User declined — fail quietly with a non-zero code; the prompt
		// already explained the situation.
		return nil, &ExitError{Code: 1}
	}

	return autoInstall(notInstalled, tool)
}

func autoInstall(e *resolver.NotInstalledError, tool string) (*resolver.ResolvedBinary, error) {
	fmt.Fprintf(os.Stderr, "Installing %s@%s...\n", e.Tool, e.Version)
	if _, err := installTool(e.Tool, e.Version, false); err != nil {
		return nil, fmt.Errorf("auto-install failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Installed %s %s\n", e.Tool, e.Version)

	// Retry resolution after install.
	return resolver.ResolveBinaryFull(tool, "")
}

// promptInstall asks the user whether to install a missing version.
func promptInstall(in io.Reader, e *resolver.NotInstalledError) bool {
	msg := fmt.Sprintf("%s %s is not installed", e.Tool, e.Version)
	if e.Context != "" {
		msg = fmt.Sprintf("%s %s (%s) is not installed", e.Tool, e.Version, e.Context)
	}
	fmt.Fprintf(os.Stderr, "%s\nInstall now? [Y/n] ", msg)

	// ReadString returns what it read alongside io.EOF when the input ends
	// without a newline, so a piped "y" has to be honored rather than
	// discarded with the error. Only an empty read is a refusal, matching how
	// confirm reads its answer.
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && answer == "" {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}
