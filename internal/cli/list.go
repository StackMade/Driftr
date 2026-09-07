package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/installer"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/resolver"
)

// npmPackage maps tool names to their npm package names.
// Tools not in this map are not listable from the npm registry.
var npmPackage = map[string]string{
	"pnpm": "pnpm",
	"yarn": "yarn",
}

func newListCmd() *cobra.Command {
	var remote bool
	var pre bool
	var limit int

	cmd := &cobra.Command{
		Use:     "list [tool]",
		Aliases: []string{"ls"},
		Short:   "List installed versions",
		Long:    "List installed versions for a tool. Defaults to node.\n\nExamples:\n  driftr list\n  driftr list node\n  driftr list --remote node\n  driftr list --remote pnpm --pre\n  driftr list --remote bun\n  driftr list --remote node --limit 10",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := "node"
			if len(args) > 0 {
				tool = args[0]
			}

			if remote {
				return listRemote(tool, pre, limit)
			}

			versions, err := installer.ListInstalledToolVersions(tool)
			if err != nil {
				return fmt.Errorf("failed to list versions: %w", err)
			}

			if len(versions) == 0 {
				fmt.Printf("No %s versions installed.\n", tool)
				fmt.Printf("Run `driftr install %s@<version>` to get started.\n", tool)
				return nil
			}

			cfg, err := config.LoadGlobal()
			if err != nil {
				return fmt.Errorf("failed to load global config: %w", err)
			}

			defaultVer := cfg.Default.GetTool(tool)

			res, resolveErr := resolver.ResolveTool(tool, "", false)
			activeVer := ""
			if resolveErr == nil && res != nil {
				activeVer = res.Version
			} else if resolveErr != nil && !strings.HasPrefix(resolveErr.Error(), "no "+tool+" version configured") {
				fmt.Fprintf(os.Stderr, "warning: could not determine active %s version: %v\n", tool, resolveErr)
			}

			fmt.Printf("Installed %s versions:\n", tool)
			for _, v := range versions {
				isActive := activeVer != "" && v == activeVer
				isDefault := defaultVer != "" && v == defaultVer

				activeMark := " "
				if isActive {
					activeMark = ">"
				}
				defaultMark := " "
				if isDefault {
					defaultMark = "*"
				}
				line := activeMark + defaultMark + " " + v

				switch {
				case isActive && isDefault:
					line = ioutil.Green(ioutil.Bold(line))
				case isActive:
					line = ioutil.Green(line)
				case !isActive && !isDefault:
					line = ioutil.Dim(line)
				}

				fmt.Printf("  %s\n", line)
			}

			var legend []string
			if activeVer != "" {
				legend = append(legend, "  > = active (current directory)")
			}
			if defaultVer != "" {
				legend = append(legend, "  * = global default")
			}
			if len(legend) > 0 {
				fmt.Println()
				for _, l := range legend {
					fmt.Println(l)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&remote, "remote", false, "List available remote versions")
	cmd.Flags().BoolVar(&pre, "pre", false, "Include pre-release versions (npm packages only)")
	cmd.Flags().IntVar(&limit, "limit", 30, "Max versions to show (0 = all)")

	return cmd
}

func listRemote(tool string, includePre bool, limit int) error {
	installedVers, err := installer.ListInstalledToolVersions(tool)
	if err != nil {
		return fmt.Errorf("failed to list installed versions: %w", err)
	}
	installed := make(map[string]bool, len(installedVers))
	for _, v := range installedVers {
		installed[v] = true
	}

	cfg, _ := config.LoadGlobal()
	defaultVer := ""
	if cfg != nil {
		defaultVer = cfg.Default.GetTool(tool)
	}

	res, _ := resolver.ResolveTool(tool, "", false)
	activeVer := ""
	if res != nil {
		activeVer = res.Version
	}

	switch tool {
	case "node":
		return listRemoteNode(limit, installed, activeVer, defaultVer)
	case "bun":
		return listRemoteBun(limit, installed, activeVer, defaultVer)
	}

	pkg, ok := npmPackage[tool]
	if !ok {
		supported := []string{"node", "bun"}
		for k := range npmPackage {
			supported = append(supported, k)
		}
		return fmt.Errorf("remote listing not supported for %q (supported: %s)", tool, strings.Join(supported, ", "))
	}

	return listRemoteNpm(pkg, tool, includePre, limit, installed, activeVer, defaultVer)
}

func listRemoteNode(limit int, installed map[string]bool, activeVer, defaultVer string) error {
	releases, err := installer.FetchNodeIndex()
	if err != nil {
		return fmt.Errorf("failed to fetch Node.js release list: %w", err)
	}

	total := len(releases)
	if limit > 0 && len(releases) > limit {
		releases = releases[:limit]
	}

	header := fmt.Sprintf("Available node versions (latest %d of %d):", len(releases), total)
	if limit == 0 {
		header = fmt.Sprintf("Available node versions (all %d):", total)
	}
	fmt.Println(header)

	for _, rel := range releases {
		printRemoteVersion(rel.Version, ltsCodename(rel.LTS), installed, activeVer, defaultVer)
	}

	printRemoteLegend(activeVer, defaultVer)
	return nil
}

func listRemoteNpm(pkg, tool string, includePre bool, limit int, installed map[string]bool, activeVer, defaultVer string) error {
	versions, err := installer.ListRemoteVersions(pkg, includePre)
	if err != nil {
		return fmt.Errorf("failed to fetch %s versions: %w", tool, err)
	}
	return printRemoteList(tool, versions, limit, installed, activeVer, defaultVer)
}

// listRemoteBun lists bun releases from GitHub — bun ships binaries there, not
// through the npm registry.
func listRemoteBun(limit int, installed map[string]bool, activeVer, defaultVer string) error {
	versions, err := installer.ListBunReleases()
	if err != nil {
		return fmt.Errorf("failed to fetch bun versions: %w", err)
	}
	return printRemoteList("bun", versions, limit, installed, activeVer, defaultVer)
}

func printRemoteList(tool string, versions []string, limit int, installed map[string]bool, activeVer, defaultVer string) error {
	total := len(versions)
	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
	}

	header := fmt.Sprintf("Available %s versions (latest %d of %d):", tool, len(versions), total)
	if limit == 0 {
		header = fmt.Sprintf("Available %s versions (all %d):", tool, total)
	}
	fmt.Println(header)

	for _, v := range versions {
		printRemoteVersion(v, "", installed, activeVer, defaultVer)
	}

	printRemoteLegend(activeVer, defaultVer)
	return nil
}

// printRemoteVersion prints a single version line with markers.
func printRemoteVersion(ver, lts string, installed map[string]bool, activeVer, defaultVer string) {
	isInstalled := installed[ver]
	isActive := activeVer != "" && ver == activeVer
	isDefault := defaultVer != "" && ver == defaultVer

	activeMark := " "
	if isActive {
		activeMark = ">"
	}
	defaultMark := " "
	if isDefault {
		defaultMark = "*"
	}
	installedMark := " "
	if isInstalled {
		installedMark = "●"
	}

	suffix := ""
	if lts != "" {
		suffix = " (LTS: " + lts + ")"
	}

	line := activeMark + defaultMark + installedMark + " " + ver + suffix

	switch {
	case isActive && isDefault:
		line = ioutil.Green(ioutil.Bold(line))
	case isActive:
		line = ioutil.Green(line)
	case isInstalled:
		// installed but not active: normal brightness
	default:
		line = ioutil.Dim(line)
	}

	fmt.Printf("  %s\n", line)
}

func printRemoteLegend(activeVer, defaultVer string) {
	fmt.Println()
	if activeVer != "" {
		fmt.Println("  > = active (current directory)")
	}
	if defaultVer != "" {
		fmt.Println("  * = global default")
	}
	fmt.Println("  ● = installed")
}

// ltsCodename extracts the LTS codename from the NodeRelease.LTS field.
// Returns "" for non-LTS releases.
func ltsCodename(lts any) string {
	s, ok := lts.(string)
	if !ok {
		return ""
	}
	return s
}
