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
	"github.com/stackmade/driftr/internal/version"
)

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <tool@version>",
		Short: "Remove an installed tool version",
		Long:  "Remove a previously installed tool version and free disk space.\n\nExamples:\n  driftr uninstall node@22.14.0\n  driftr uninstall pnpm@9.15.0\n  driftr uninstall yarn@1.22.22",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool, versionSpec := parseToolVersion(args[0])

			// Resolve bundled tools to their parent (e.g. npm → node, pnpx → pnpm).
			entry, ok := platform.LookupTool(tool)
			if !ok {
				return fmt.Errorf("unknown tool: %s. Supported tools: node, pnpm, yarn", tool)
			}
			tool = entry.Parent

			if versionSpec == "" {
				return fmt.Errorf("version required. Usage: driftr uninstall %s@<version>", tool)
			}

			// Validate and normalize the version string to prevent path traversal.
			v, err := version.Parse(versionSpec)
			if err != nil {
				return fmt.Errorf("invalid version: %w", err)
			}
			versionStr := v.String()

			// Verify the version is installed.
			versionDir, err := platform.ToolVersionDir(tool, versionStr)
			if err != nil {
				return err
			}
			if _, err := os.Stat(versionDir); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("%s %s is not installed", tool, versionStr)
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
			return nil
		},
	}
}

// warnIfPinned walks up from dir looking for a project config that pins
// tool@versionStr, and warns once for each config file that does.
func warnIfPinned(dir, tool, versionStr string) {
	for {
		if proj, err := config.LoadProject(dir); err == nil && proj != nil && proj.Tools.GetTool(tool) == versionStr {
			printPinWarning(".driftr.toml", dir, tool, versionStr)
		}
		if pkg, err := config.LoadPackageJSON(dir); err == nil && pkg != nil && pkg.Driftr.GetTool(tool) == versionStr {
			printPinWarning("package.json", dir, tool, versionStr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func printPinWarning(file, dir, tool, versionStr string) {
	fmt.Println(ioutil.Warn(fmt.Sprintf(
		"%s@%s is pinned in %s — uninstalling will break that project until you run 'driftr install %s@%s' or update the pin",
		tool, versionStr, filepath.Join(dir, file), tool, versionStr)))
}
