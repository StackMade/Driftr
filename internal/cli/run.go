package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/resolver"
)

func newRunCmd() *cobra.Command {
	var nodeVersion string

	cmd := &cobra.Command{
		Use:                "run [flags] -- <command> [args...]",
		Short:              "Run a command under a specific Node.js version",
		Long:               "Execute a command using an explicitly specified Node.js version without changing defaults.\n\nExamples:\n  driftr run --node 24.0.0 -- npm test\n  driftr run --node 24 -- node -v",
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified. Usage: driftr run --node <version> -- <command> [args...]")
			}

			tool := args[0]
			toolArgs := args[1:]

			if nodeVersion != "" {
				// npm, npx, yarn and package bins are "#!/usr/bin/env node"
				// scripts, and whatever they spawn looks node up on PATH too.
				// Left alone, those lookups find driftr's shim (the project's
				// node) or whatever node comes first on PATH. The requested
				// node's bin dir goes first on PATH, and DRIFTR_NODE covers
				// tools reached through shims, so the whole tree stays on it.
				res, err := resolver.ResolveTool("node", nodeVersion, verbose)
				if err != nil {
					return err
				}
				nodeVersion = res.Version // "20" becomes the 20.x it resolved to
				if err := os.Setenv(resolver.EnvVarName("node"), res.Version); err != nil {
					return err
				}
				if err := os.Setenv("PATH", filepath.Dir(res.BinaryPath)+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
					return err
				}
			}

			rb, err := resolver.ResolveBinaryFull(tool, nodeVersion)
			if err != nil {
				return err
			}
			binary, argv, err := prepareResolved(rb, toolArgs)
			if err != nil {
				return err
			}

			exitCode, err := runResolved(tool, binary, argv, toolArgs)
			if err != nil {
				return err
			}

			return &ExitError{Code: exitCode}
		},
	}

	cmd.Flags().StringVar(&nodeVersion, "node", "", "Node.js version to use")

	return cmd
}
