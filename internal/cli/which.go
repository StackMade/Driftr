package cli

import (
	"fmt"

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

			res, err := resolver.ResolveTool(tool, "", verbose)
			if err != nil {
				return err
			}

			binPath, err := platform.ToolBinary(tool, res.Version)
			if err != nil {
				return err
			}

			row := func(label, value string) {
				// Pad the plain label before coloring so the (runtime zero-width)
				// ANSI codes don't throw off column alignment.
				fmt.Printf("%s %s\n", ioutil.Label(fmt.Sprintf("%-8s", label)), value)
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
