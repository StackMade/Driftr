package cli

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/resolver"
)

// pinFormat represents the config storage format.
type pinFormat int

const (
	formatNone        pinFormat = iota
	formatTOML                  // .driftr.toml
	formatPackageJSON           // package.json driftr key
)

func newPinCmd() *cobra.Command {
	var migrate, bump, major, dryRun bool

	cmd := &cobra.Command{
		Use:   "pin <tool@version> | --bump [tool...]",
		Short: "Pin a tool version to the current project",
		Long: `Pin a tool version to the current project.

The version is stored in .driftr.toml or package.json (driftr key).
On first use, you'll be asked which format to use. Subsequent pins
reuse the detected format automatically.

With --bump, the pins the project already has move to the newest release
on their major line, in whichever file holds them. A pin keeps its precision:
22.14.0 becomes the newest 22.x.y, while 22 already floats and stays put.
--major allows a newer major line. Nothing is installed.

Examples:
  driftr pin node@22.14.0
  driftr pin node@22.14.0 --migrate   # switch storage format
  driftr pin --bump                   # every pinned tool, same major
  driftr pin --bump node pnpm --major # these tools, newest overall
  driftr pin --bump --dry-run         # show old -> new, write nothing`,
		Args: func(cmd *cobra.Command, args []string) error {
			if !bump {
				if major || dryRun {
					return fmt.Errorf("--major and --dry-run only apply to --bump. Run `driftr pin --bump --major` or `driftr pin --bump --dry-run`")
				}
				return cobra.ExactArgs(1)(cmd, args)
			}
			if migrate {
				return fmt.Errorf("--bump and --migrate cannot be combined. Run `driftr pin --bump` first, then `driftr pin <tool@version> --migrate`")
			}
			for _, a := range args {
				if tool, ver, ok := strings.Cut(a, "@"); ok {
					return fmt.Errorf("--bump takes tool names, not versions: %q. Run `driftr pin %s@%s` to set that version, or `driftr pin --bump %s` to move the pin to the newest release", a, tool, ver, tool)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if bump {
				return runPinBump(cmd.OutOrStdout(), args, major, dryRun)
			}

			tool, versionSpec := parseToolVersion(args[0])
			tool, err := versionedTool(tool)
			if err != nil {
				return err
			}

			versionStr, _, err := resolver.RequireToolInstalled(tool, versionSpec)
			if err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine current directory: %w", err)
			}

			current := detectPinFormat(cwd)

			if migrate {
				if err := migratePin(cwd, tool, versionStr, current); err != nil {
					return err
				}
				rememberProject(cwd)
				return nil
			}

			format := current
			if format == formatNone {
				var err error
				format, err = promptPinFormat(cmd)
				if err != nil {
					return err
				}
			}

			if err := savePin(cwd, tool, versionStr, format); err != nil {
				return err
			}
			rememberProject(cwd)
			return nil
		},
	}

	cmd.Flags().BoolVar(&migrate, "migrate", false, "migrate config to the other storage format")
	cmd.Flags().BoolVar(&bump, "bump", false, "move existing pins to the newest release on their major line")
	cmd.Flags().BoolVar(&major, "major", false, "with --bump, allow a newer major line")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "with --bump, show what would change without writing")

	return cmd
}

// detectPinFormat checks which config format exists in the directory.
// .driftr.toml takes priority if both exist.
func detectPinFormat(dir string) pinFormat {
	tomlPath := filepath.Join(dir, config.ProjectConfigFile)
	if _, err := os.Stat(tomlPath); err == nil {
		return formatTOML
	}

	pkg, _ := config.LoadPackageJSON(dir)
	if pkg != nil {
		return formatPackageJSON
	}

	return formatNone
}

// promptPinFormat asks the user to choose a storage format.
func promptPinFormat(cmd *cobra.Command) (pinFormat, error) {
	// Non-interactive: default to .driftr.toml.
	// Check for a real terminal (not just a char device like /dev/null).
	if !ioutil.IsTerminal(os.Stdin) {
		return formatTOML, nil
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "No existing project config found. How should the version be stored?")
	fmt.Fprintln(cmd.ErrOrStderr(), "  1) .driftr.toml (recommended)")
	fmt.Fprintln(cmd.ErrOrStderr(), "  2) package.json (driftr key)")
	fmt.Fprint(cmd.ErrOrStderr(), "Choose [1/2]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return formatTOML, err
	}

	switch strings.TrimSpace(input) {
	case "2":
		return formatPackageJSON, nil
	default:
		return formatTOML, nil
	}
}

// savePin writes the version in the given format.
func savePin(dir, tool, versionStr string, format pinFormat) error {
	switch format {
	case formatPackageJSON:
		if err := config.SavePackageJSONTool(dir, tool, versionStr); err != nil {
			return err
		}
		fmt.Println(ioutil.Success(fmt.Sprintf("Pinned %s %s in %s/package.json", tool, ioutil.Bold(versionStr), dir)))
	default:
		cfg, err := config.LoadProject(dir)
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &config.ProjectConfig{}
		}
		cfg.Tools.SetTool(tool, versionStr)
		if err := config.SaveProject(dir, cfg); err != nil {
			return err
		}
		fmt.Println(ioutil.Success(fmt.Sprintf("Pinned %s %s in %s/%s", tool, ioutil.Bold(versionStr), dir, config.ProjectConfigFile)))
	}
	return nil
}

// migratePin switches from the current format to the other one.
func migratePin(dir, tool, versionStr string, current pinFormat) error {
	switch current {
	case formatTOML:
		// Migrate TOML → package.json, carrying every pinned tool across.
		cfg, err := config.LoadProject(dir)
		if err != nil {
			return err
		}
		tools := map[string]string{}
		if cfg != nil {
			maps.Copy(tools, cfg.Tools)
		}
		tools[tool] = versionStr
		for _, t := range slices.Sorted(maps.Keys(tools)) {
			if tools[t] == "" {
				continue
			}
			if err := config.SavePackageJSONTool(dir, t, tools[t]); err != nil {
				return err
			}
		}
		if err := os.Remove(filepath.Join(dir, config.ProjectConfigFile)); err != nil {
			return fmt.Errorf("migrated to package.json but failed to remove %s: %w. Remove it manually — it takes precedence over package.json", config.ProjectConfigFile, err)
		}
		fmt.Println(ioutil.Success(fmt.Sprintf("Migrated %s %s from %s to package.json", tool, ioutil.Bold(versionStr), config.ProjectConfigFile)))

	case formatPackageJSON:
		// Migrate package.json → TOML, carrying every pinned tool across.
		pkg, err := config.LoadPackageJSON(dir)
		if err != nil {
			return err
		}
		cfg := &config.ProjectConfig{}
		if pkg != nil {
			for t, v := range pkg.Driftr {
				if v != "" {
					cfg.Tools.SetTool(t, v)
				}
			}
		}
		cfg.Tools.SetTool(tool, versionStr)
		if err := config.SaveProject(dir, cfg); err != nil {
			return err
		}
		if err := config.RemoveDriftrFromPackageJSON(dir); err != nil {
			return err
		}
		fmt.Println(ioutil.Success(fmt.Sprintf("Migrated %s %s from package.json to %s", tool, ioutil.Bold(versionStr), config.ProjectConfigFile)))

	case formatNone:
		return fmt.Errorf("no existing config to migrate. Run `driftr pin %s@<version>` first", tool)
	}
	return nil
}
