package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/resolver"
)

func newWhichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which <tool>",
		Short: "Show which binary Driftr would execute",
		Long:  "Display the resolved binary path and the source of the version decision.\n\nExample:\n  driftr which node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := args[0]

			row := func(label, value string) {
				// Pad the plain label before coloring so the (runtime zero-width)
				// ANSI codes don't throw off column alignment.
				fmt.Printf("%s %s\n", ioutil.Label(fmt.Sprintf("%-8s", label)), value)
			}

			// Anything that is not a driftr tool is a bin from a global npm install.
			entry, known := platform.LookupTool(tool)
			if !known {
				rb, err := resolver.ResolveBinaryFull(tool, "")
				if err != nil {
					return err
				}
				row("Tool:", tool)
				if rb.PathPrefix == "" {
					row("Binary:", ioutil.Green(rb.ToolPath))
					row("Source:", "found on PATH outside driftr, so driftr leaves it alone")
					return nil
				}
				row("Node:", ioutil.Bold(filepath.Base(filepath.Dir(rb.PathPrefix))))
				row("Binary:", ioutil.Green(rb.ToolPath))
				row("Source:", "global npm package")
				return nil
			}

			// npx and pnpx follow the version of npm and pnpm.
			res, err := resolver.ResolveTool(entry.Parent, "", verbose)
			var notConfigured *resolver.NotConfiguredError
			if entry.Parent == "npm" && errors.As(err, &notConfigured) {
				// Nothing pins npm, so the shim runs the npm bundled with node.
				rb, err := resolver.ResolveBinaryFull(tool, "")
				if err != nil {
					return err
				}
				row("Tool:", tool)
				row("Binary:", ioutil.Green(rb.ToolPath))
				row("Source:", "bundled with Node.js "+filepath.Base(filepath.Dir(filepath.Dir(rb.ToolPath))))
				return nil
			}
			if err != nil {
				return err
			}

			binPath, err := platform.ToolBinary(tool, res.Version)
			if err != nil {
				return err
			}

			row("Tool:", tool)
			row("Version:", ioutil.Bold(res.Version))
			row("Binary:", ioutil.Green(binPath))
			source := fmt.Sprint(res.Source)
			if res.Source == resolver.SourceEnv {
				source = fmt.Sprintf("%s (%s)", source, resolver.EnvVarName(res.Tool))
			}
			row("Source:", source)
			if res.ProjectDir != "" {
				row("Project:", res.ProjectDir)
			}

			return nil
		},
	}
}
