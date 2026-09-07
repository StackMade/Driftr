package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/pathsetup"
	"github.com/stackmade/driftr/internal/resolver"
)

func newUseCmd() *cobra.Command {
	var unset bool
	var shellFlag string

	cmd := &cobra.Command{
		Use:   "use <tool@version>",
		Short: "Print a shell snippet pinning a tool version for this shell",
		Long: `Print a shell snippet that pins a tool version for the current shell only.

The snippet sets DRIFTR_<TOOL>, which the resolver reads before any project
config, so the override holds until you unset it or close the shell.

Nothing changes unless you evaluate the output:

  eval "$(driftr use node@24)"
  eval "$(driftr use node@24.14.0)"
  eval "$(driftr use pnpm@9)"
  eval "$(driftr use --unset node)"

The syntax follows $SHELL. Pass --shell to say which one to write for.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fish, err := useFishSyntax(shellFlag)
			if err != nil {
				return err
			}

			tool, versionSpec := parseToolVersion(args[0])
			if unset {
				// With --unset the argument is a bare tool name, so anything
				// after "@" is a mistake worth naming.
				tool = args[0]
				versionSpec = ""
			}

			envVar := resolver.EnvVarName(tool)
			if envVar == "" {
				return fmt.Errorf("unknown tool: %s. Supported tools: node, npm, npx, pnpm, pnpx, yarn", tool)
			}

			if unset {
				fmt.Fprintln(cmd.OutOrStdout(), unsetSnippet(envVar, fish))
				hintEval(cmd, args[0], true)
				return nil
			}

			if versionSpec == "" {
				return fmt.Errorf("version required. Use '%s@<version>', e.g. `driftr use %s@24`", tool, tool)
			}

			ver, _, err := resolver.RequireToolInstalled(tool, versionSpec)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), setSnippet(envVar, ver, fish))
			hintEval(cmd, args[0], false)
			return nil
		},
	}

	cmd.Flags().BoolVar(&unset, "unset", false, "print the snippet that drops the override")
	cmd.Flags().StringVar(&shellFlag, "shell", "", "shell syntax to print: bash, zsh, fish or sh (default: $SHELL)")

	return cmd
}

func setSnippet(envVar, ver string, fish bool) string {
	if fish {
		return fmt.Sprintf("set -gx %s %s", envVar, ver)
	}
	return fmt.Sprintf("export %s=%s", envVar, ver)
}

func unsetSnippet(envVar string, fish bool) string {
	if fish {
		return fmt.Sprintf("set -e %s", envVar)
	}
	return fmt.Sprintf("unset %s", envVar)
}

// hintEval reminds the user to wrap the command in eval, but only when stdout
// is a terminal: the snippet is the whole output, so the hint must stay on
// stderr and stay out of the way when the shell is actually reading it.
func hintEval(cmd *cobra.Command, arg string, unset bool) {
	if !ioutil.IsTerminal(os.Stdout) {
		return
	}
	flag := ""
	if unset {
		flag = "--unset "
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "This only prints the snippet. Apply it with: eval \"$(driftr use %s%s)\"\n", flag, arg)
}

// useFishSyntax reports whether to print fish syntax, from the --shell flag or
// $SHELL. Every other supported shell takes the POSIX form.
func useFishSyntax(shellFlag string) (bool, error) {
	if shellFlag == "" {
		return pathsetup.DetectShell() == pathsetup.ShellFish, nil
	}
	switch shellFlag {
	case "fish":
		return true, nil
	case "bash", "zsh", "sh", "posix":
		return false, nil
	default:
		return false, fmt.Errorf("unknown shell: %s. Use bash, zsh, fish or sh", shellFlag)
	}
}
