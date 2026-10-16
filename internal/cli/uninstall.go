package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/resolver"
	"github.com/stackmade/driftr/internal/shim"
	"github.com/stackmade/driftr/internal/version"
)

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <tool@version>",
		Short: "Remove an installed tool version",
		Long:  "Remove a previously installed tool version and free disk space.\n\nExamples:\n  driftr uninstall node@22.14.0\n  driftr uninstall npm@10.9.2\n  driftr uninstall pnpm@9.15.0\n  driftr uninstall yarn@1.22.22\n  driftr uninstall bun@1.2.0",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool, versionSpec := parseToolVersion(args[0])

			// Resolve bundled tools to their parent (e.g. npx → npm, pnpx → pnpm).
			tool, err := versionedTool(tool)
			if err != nil {
				return err
			}

			if versionSpec == "" {
				return fmt.Errorf("version required. Usage: driftr uninstall %s@<version>", tool)
			}

			// Validate and normalize the version string to prevent path traversal.
			v, err := version.Parse(versionSpec)
			if err != nil {
				return fmt.Errorf("invalid version: %w", err)
			}
			// Removal takes an exact version: a partial spec could mean any of
			// several installs, so name the one it selects instead of guessing.
			if v.Latest || v.LTS || v.IsPartial() {
				match, ok, err := resolver.ResolvePin(tool, versionSpec)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("no installed %s version matches %s. Run `driftr list %s` to see what is installed", tool, versionSpec, tool)
				}
				return fmt.Errorf("uninstall needs an exact version. Run `driftr uninstall %s@%s` to remove the newest match, or `driftr list %s` to see every installed version", tool, match, tool)
			}
			versionStr := v.String()

			// Verify the version is installed.
			versionDir, err := platform.ToolVersionDir(tool, versionStr)
			if err != nil {
				return err
			}
			if _, err := os.Stat(versionDir); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("%s %s is not installed. Run `driftr list %s` to see what is installed", tool, versionStr, tool)
				}
				return fmt.Errorf("failed to check installed version %s %s: %w", tool, versionStr, err)
			}

			// Warn if this is the global default.
			cfg, err := config.LoadGlobal()
			if err == nil && cfg.Default.GetTool(tool) == versionStr {
				fmt.Println(ioutil.Warn(fmt.Sprintf("%s %s is the current global default. Run `driftr default %s@<version>` to set a new one.", tool, versionStr, tool)))
			}

			// Warn if this version is pinned anywhere in the project tree.
			// The resolver walks up from the cwd, so a pin in a parent directory
			// applies here too and deserves the same warning.
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				warnIfPinned(cwd, tool, versionStr)
			}

			if verbose {
				fmt.Println(ioutil.Dim(fmt.Sprintf("  Removing: %s", versionDir)))
			}

			if err := os.RemoveAll(versionDir); err != nil {
				return fmt.Errorf("failed to remove %s %s: %w. Manual cleanup: rm -rf %q", tool, versionStr, err, versionDir)
			}

			fmt.Println(ioutil.Success(fmt.Sprintf("Uninstalled %s %s", tool, ioutil.Bold(versionStr))))
			if tool == "node" {
				refreshShims()
			}
			return nil
		},
	}
}

// refreshShims regenerates the shims after node versions are removed, so the
// shims of package bins that went with them do not linger. A failure only
// warns: the removal itself already succeeded.
func refreshShims() {
	if _, err := shim.GenerateShims(Version); err != nil {
		fmt.Fprintln(os.Stderr, ioutil.Warn(fmt.Sprintf("Could not refresh shims: %s. Run 'driftr setup'", err)))
	}
}

// warnIfPinned warns once for each pin that selects tool@versionStr, in the
// project containing dir and in every recorded project. Pins come from the
// same sources the resolver reads (.nvmrc, packageManager, engines, partial
// specs, …), so removing what any of them resolves to is flagged.
func warnIfPinned(dir, tool, versionStr string) {
	dirs := []string{dir}
	if recorded, err := config.LoadProjects(); err == nil {
		dirs = append(dirs, recorded...)
	}

	warned := map[string]bool{}
	for _, d := range dirs {
		// A recorded project that is gone would walk up into its parents.
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			continue
		}
		pins, err := resolver.ProjectPins(d)
		if err != nil {
			continue
		}
		for _, pin := range pins {
			if pin.Tool != tool {
				continue
			}
			if pin.Range != "" {
				// engines.node: the shim runs the newest installed match, not
				// the lower-bound major in pin.Version.
				if resolved, ok, err := resolver.ResolveRange(tool, pin.Range); err != nil || !ok || resolved != versionStr {
					continue
				}
			} else if pin.Version != versionStr {
				if resolved, ok, err := resolver.ResolvePin(tool, pin.Version); err != nil || !ok || resolved != versionStr {
					continue
				}
			}
			file := filepath.Join(pin.Dir, pin.Source.File())
			if warned[file] {
				continue
			}
			warned[file] = true
			printPinWarning(file, tool, versionStr)
		}
	}
}

func printPinWarning(file, tool, versionStr string) {
	fmt.Println(ioutil.Warn(fmt.Sprintf(
		"%s@%s is pinned in %s — uninstalling will break that project until you run 'driftr install %s@%s' or update the pin",
		tool, versionStr, file, tool, versionStr)))
}
