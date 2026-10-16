package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/nodeenv"
)

func newNodeCleanCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove node_modules and prune the shared store",
		Long: "Reclaim disk space by removing the project's node_modules, reinstalling with\n" +
			"the project's package manager, and pruning orphaned packages from the pnpm store.\n\n" +
			"Runs as a dry-run by default; pass --yes to perform the destructive operations.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			return runNodeClean(nodeenv.NewExecRunner(), dir, yes, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "actually perform the cleanup (otherwise dry-run)")
	return cmd
}

// runNodeClean removes node_modules, reinstalls with the project's package
// manager, and prunes the pnpm store. With doExecute=false it only reports what
// would happen and mutates nothing.
func runNodeClean(r nodeenv.Runner, projectDir string, doExecute bool, w io.Writer) error {
	p := nodeenv.NewPnpm(r)
	pm := nodeenv.DetectPackageManager(projectDir)
	if pm == nodeenv.NoManager {
		// No lockfile or packageManager field: npm ships with node, so it is
		// the one manager that can always restore what was removed.
		pm = nodeenv.NpmManager
	}
	install := string(pm) + " install"

	nodeModules := filepath.Join(projectDir, "node_modules")
	// Presence is decided by stat, not size: an empty node_modules (or one
	// holding only symlinks) still needs removing and reinstalling.
	hasNodeModules := false
	if info, err := os.Stat(nodeModules); err == nil && info.IsDir() {
		hasNodeModules = true
	}
	size, _ := nodeenv.DirSize(nodeModules)

	if !doExecute {
		fmt.Fprintln(w, ioutil.Title("Dry run"))
		fmt.Fprintln(w, ioutil.Dim("No changes made. Re-run with --yes to execute."))
		fmt.Fprintln(w)
		if hasNodeModules {
			fmt.Fprintf(w, "Would remove: %s %s\n", nodeModules, ioutil.Dim("("+formatSize(size)+")"))
			fmt.Fprintf(w, "Would run:    %s\n", ioutil.Bold(install))
		} else {
			fmt.Fprintln(w, ioutil.Bullet("no node_modules to remove"))
		}
		fmt.Fprintf(w, "Would run:    %s\n", ioutil.Bold("pnpm store prune"))
		return nil
	}

	// Check the reinstall can run before deleting anything: removing
	// node_modules and then failing to reinstall leaves the project broken.
	if hasNodeModules {
		if _, err := r.Run(string(pm), "--version"); err != nil {
			fix := "driftr install " + string(pm)
			if pm == nodeenv.NpmManager {
				fix = "driftr install node"
			}
			return fmt.Errorf("%s cannot run, so node_modules was left in place: %w. Run `%s`, then `driftr node clean --yes`", pm, err, fix)
		}
		if err := os.RemoveAll(nodeModules); err != nil {
			return fmt.Errorf("removing node_modules: %w", err)
		}
		fmt.Fprintln(w, ioutil.Success("removed node_modules "+ioutil.Dim("("+formatSize(size)+")")))

		fmt.Fprintf(w, "Running %s...\n", install)
		if _, err := r.Run(string(pm), "install"); err != nil {
			return fmt.Errorf("%s failed: %w. Fix the cause, then run `%s` to restore dependencies", install, err, install)
		}
		fmt.Fprintln(w, ioutil.Success("dependencies reinstalled"))
	} else {
		fmt.Fprintln(w, ioutil.Bullet("no node_modules to remove"))
	}

	// The shared store is pnpm's; without pnpm there is nothing to prune.
	if !p.Installed() {
		fmt.Fprintln(w, ioutil.Bullet("pnpm not found — skipped store prune"))
		return nil
	}

	if _, err := p.StorePrune(); err != nil {
		// A pnpm shim exists even when no pnpm is configured; in an npm or
		// yarn project that must not fail a clean that already succeeded.
		if pm != nodeenv.PnpmManager {
			fmt.Fprintln(w, ioutil.Bullet("skipped pnpm store prune: "+err.Error()))
			return nil
		}
		return fmt.Errorf("pnpm store prune failed: %w", err)
	}
	fmt.Fprintln(w, ioutil.Success("pruned orphaned packages from the shared store"))
	return nil
}
