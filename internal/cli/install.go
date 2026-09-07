package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/installer"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/resolver"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install [tool[@version]]",
		Short: "Install a tool version, or everything the project pins",
		Long:  "Download and install a tool version.\n\nWithout an argument, installs every tool the current project pins,\nreading .driftr.toml, the package.json driftr key, packageManager,\n.nvmrc and .node-version.\n\nA bare tool name installs the newest release.\n\nExamples:\n  driftr install             # everything this project pins\n  driftr install pnpm        # latest pnpm\n  driftr install node        # latest node\n  driftr install node@24\n  driftr install pnpm@9\n  driftr install yarn@1\n  driftr install node@latest\n  driftr install node@lts    # newest LTS release (node only)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return installProjectPins()
			}

			raw := args[0]
			tool, versionSpec := parseToolVersion(raw)
			if versionSpec == "" {
				// "tool@" is a malformed spec (the user wrote "@" but omitted the
				// version); reject it rather than silently installing latest. A
				// bare tool name ("driftr install pnpm") has no "@" and means
				// "install the newest release".
				if strings.Contains(raw, "@") {
					return fmt.Errorf("version required after '@'. Use 'driftr install %s' for the latest, or '%s@<version>'", tool, tool)
				}
				versionSpec = "latest"
			}

			fmt.Println(ioutil.Dim(fmt.Sprintf("Installing %s@%s...", tool, versionSpec)))

			resolved, err := installTool(tool, versionSpec, verbose)
			if err != nil {
				return fmt.Errorf("installation failed: %w", err)
			}

			fmt.Println(ioutil.Success(fmt.Sprintf("Installed %s %s", tool, ioutil.Bold(resolved))))
			return nil
		},
	}
}

// installProjectPins installs every tool version the current project pins.
// One failing tool does not stop the others: the failures are collected and
// reported together at the end.
func installProjectPins() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	pins, err := resolver.ProjectPins(cwd)
	if err != nil {
		return err
	}
	if len(pins) == 0 {
		return fmt.Errorf("no tool versions pinned in %s or its parents. Pin one with `driftr pin node@<version>`, or name a tool: `driftr install node@24`", cwd)
	}

	var failures []error
	for _, pin := range pins {
		fmt.Println(ioutil.Dim(fmt.Sprintf("Installing %s@%s (from %s)...", pin.Tool, pin.Version, pin.Source)))

		resolved, err := installTool(pin.Tool, pin.Version, verbose)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s@%s: %w", pin.Tool, pin.Version, err))
			fmt.Println(ioutil.Failure(fmt.Sprintf("Failed to install %s %s: %v", pin.Tool, pin.Version, err)))
			continue
		}

		fmt.Println(ioutil.Success(fmt.Sprintf("Installed %s %s", pin.Tool, ioutil.Bold(resolved))))
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d pinned tools failed to install: %w", len(failures), len(pins), errors.Join(failures...))
	}
	return nil
}

func installTool(tool, versionSpec string, verbose bool) (string, error) {
	// Reconstruct the spec for installers that expect "tool@version" format.
	spec := tool + "@" + versionSpec

	switch tool {
	case "node":
		return installer.Install(spec, verbose)
	case "pnpm":
		return installer.InstallPnpm(versionSpec, verbose)
	case "yarn":
		return installer.InstallYarn(versionSpec, verbose)
	default:
		return "", fmt.Errorf("unknown tool: %s. Supported tools: node, pnpm, yarn", tool)
	}
}
