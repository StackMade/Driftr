package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/pathsetup"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/shim"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Initialize Driftr directories and generate shims",
		Long:  "Create the Driftr directory structure, generate shim scripts, and configure\nyour shell PATH so the shims are found.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.EnsureDirs(); err != nil {
				return fmt.Errorf("failed to create directories: %w", err)
			}

			skipped, err := shim.GenerateShims(Version)
			if err != nil {
				return fmt.Errorf("failed to generate shims: %w", err)
			}
			printSkippedShims(cmd.OutOrStdout(), skipped)

			binDir, err := platform.BinDir()
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), ioutil.Success(ioutil.Bold("Driftr setup complete!")))
			fmt.Fprintln(cmd.OutOrStdout())
			configureSetupPath(cmd.OutOrStdout(), binDir)
			return nil
		},
	}
}

// configureSetupPath sets up the shell PATH the same way `doctor --fix` and
// `self-update` do — writing to the rc file every shell invocation sources
// (e.g. ~/.zshenv) rather than an interactive-only file like ~/.zshrc. On any
// failure it falls back to shell-agnostic manual instructions.
func configureSetupPath(w io.Writer, binDir string) {
	r, err := pathsetup.Detect(binDir)
	if err != nil {
		manualPathInstructions(w, binDir)
		return
	}

	if !r.NeedsFix() {
		fmt.Fprintln(w, ioutil.Success(fmt.Sprintf("PATH already configured in %s", r.Target)))
		printStaleNote(w, r)
		applyShadowGuard(w, binDir)
		if runtime.GOOS == "windows" {
			fmt.Fprintln(w, ioutil.Dim("Open a new terminal if this one does not find the shims."))
		} else {
			fmt.Fprintln(w, ioutil.Dim(fmt.Sprintf("Restart your shell or run: source %s", r.Target)))
		}
		return
	}

	wrote, file, applyErr := pathsetup.Apply(r)
	if applyErr != nil || !wrote {
		manualPathInstructions(w, binDir)
		return
	}

	fmt.Fprintln(w, ioutil.Success(fmt.Sprintf("Added %s to PATH in %s", binDir, file)))
	printStaleNote(w, r)
	applyShadowGuard(w, binDir)
	if runtime.GOOS == "windows" {
		fmt.Fprintln(w, ioutil.Dim("Open a new terminal to start using driftr."))
	} else {
		fmt.Fprintln(w, ioutil.Dim("Restart your shell to start using driftr."))
	}
}

// applyShadowGuard adds a PATH precedence guard to the interactive rc file when
// a managed tool is currently shadowed by another install earlier in PATH
// (Homebrew, Volta, …). The universal config alone can't win in that case
// because those tools re-prepend their dirs during interactive startup.
func applyShadowGuard(w io.Writer, binDir string) {
	if len(shadowedShims(binDir)) == 0 {
		return
	}
	wrote, file, err := pathsetup.ApplyGuard(pathsetup.DetectShell(), binDir)
	if err != nil || !wrote {
		return
	}
	fmt.Fprintln(w, ioutil.Success(fmt.Sprintf("Added a PATH precedence guard to %s (another install was shadowing driftr)", file)))
}

func printStaleNote(w io.Writer, r pathsetup.Result) {
	if len(r.StaleFiles) > 0 {
		fmt.Fprintln(w, ioutil.Dim(fmt.Sprintf("  Note: legacy entries remain in %s — safe to remove.", strings.Join(r.StaleFiles, ", "))))
	}
}

// manualPathInstructions prints shell-agnostic fallback guidance when driftr
// can't determine or write the rc file (e.g. unknown shell).
func manualPathInstructions(w io.Writer, binDir string) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(w, "Add the shim directory to your user PATH, e.g. from PowerShell:")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", ioutil.Bold(fmt.Sprintf(`[Environment]::SetEnvironmentVariable("Path", "%s;" + [Environment]::GetEnvironmentVariable("Path", "User"), "User")`, binDir)))
		fmt.Fprintln(w)
		fmt.Fprintln(w, ioutil.Dim("Then open a new terminal."))
		return
	}
	fmt.Fprintln(w, "Add the following to your shell profile so the shims are found:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", ioutil.Bold(fmt.Sprintf("export PATH=\"%s:$PATH\"", binDir)))
	fmt.Fprintln(w)
	fmt.Fprintln(w, ioutil.Dim("Then restart your shell."))
}
