package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/pathsetup"
	"github.com/stackmade/driftr/v2/internal/resolver"
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

In PowerShell:

  driftr use node@24 | Invoke-Expression

The syntax follows $SHELL, or is PowerShell's on Windows when $SHELL is not
set. Pass --shell to say which one to write for.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			syntax, err := useSyntax(shellFlag)
			if err != nil {
				return err
			}

			tool, versionSpec := parseToolVersion(args[0])
			if unset {
				// With --unset only the tool matters: node and node@22 both
				// drop the node override.
				tool, _, _ = strings.Cut(args[0], "@")
				versionSpec = ""
			}

			tool, err = versionedTool(tool)
			if err != nil {
				return err
			}
			envVar := resolver.EnvVarName(tool)

			if unset {
				fmt.Fprintln(cmd.OutOrStdout(), unsetSnippet(envVar, syntax))
				hintEval(cmd, args[0], true, syntax)
				return nil
			}

			if versionSpec == "" {
				return fmt.Errorf("version required. Use '%s@<version>', e.g. `driftr use %s@24`", tool, tool)
			}

			ver, _, err := resolver.RequireToolInstalled(tool, versionSpec)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), setSnippet(envVar, ver, syntax))
			hintEval(cmd, args[0], false, syntax)
			return nil
		},
	}

	cmd.Flags().BoolVar(&unset, "unset", false, "print the snippet that drops the override")
	cmd.Flags().StringVar(&shellFlag, "shell", "", "shell syntax to print: bash, zsh, fish, sh or powershell (default: $SHELL)")

	return cmd
}

// snippetSyntax is the shell language driftr use prints.
type snippetSyntax int

const (
	syntaxPOSIX snippetSyntax = iota
	syntaxFish
	syntaxPowerShell
)

func setSnippet(envVar, ver string, syntax snippetSyntax) string {
	switch syntax {
	case syntaxFish:
		return fmt.Sprintf("set -gx %s %s", envVar, ver)
	case syntaxPowerShell:
		return fmt.Sprintf("$env:%s = '%s'", envVar, ver)
	}
	return fmt.Sprintf("export %s=%s", envVar, ver)
}

func unsetSnippet(envVar string, syntax snippetSyntax) string {
	switch syntax {
	case syntaxFish:
		return fmt.Sprintf("set -e %s", envVar)
	case syntaxPowerShell:
		return fmt.Sprintf("Remove-Item Env:%s -ErrorAction SilentlyContinue", envVar)
	}
	return fmt.Sprintf("unset %s", envVar)
}

// hintEval reminds the user to wrap the command in eval, but only when stdout
// is a terminal: the snippet is the whole output, so the hint must stay on
// stderr and stay out of the way when the shell is actually reading it.
func hintEval(cmd *cobra.Command, arg string, unset bool, syntax snippetSyntax) {
	if !ioutil.IsTerminal(os.Stdout) {
		return
	}
	flag := ""
	if unset {
		flag = "--unset "
	}
	if syntax == syntaxPowerShell {
		fmt.Fprintf(cmd.ErrOrStderr(), "This only prints the snippet. Apply it with: driftr use %s%s | Invoke-Expression\n", flag, arg)
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "This only prints the snippet. Apply it with: eval \"$(driftr use %s%s)\"\n", flag, arg)
}

// useSyntax picks the snippet language from the --shell flag or $SHELL. Every
// other supported Unix shell takes the POSIX form; on Windows, where only Git
// Bash and the like set $SHELL, it is PowerShell's.
func useSyntax(shellFlag string) (snippetSyntax, error) {
	if shellFlag == "" {
		switch {
		case pathsetup.DetectShell() == pathsetup.ShellFish:
			return syntaxFish, nil
		case runtime.GOOS == "windows" && os.Getenv("SHELL") == "":
			return syntaxPowerShell, nil
		}
		return syntaxPOSIX, nil
	}
	switch shellFlag {
	case "fish":
		return syntaxFish, nil
	case "bash", "zsh", "sh", "posix":
		return syntaxPOSIX, nil
	case "powershell", "pwsh":
		return syntaxPowerShell, nil
	default:
		return syntaxPOSIX, fmt.Errorf("unknown shell: %s. Use bash, zsh, fish, sh or powershell", shellFlag)
	}
}
