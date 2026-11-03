package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/resolver"
)

// resolvedDefault returns the installed version the global default of tool
// selects, as the resolver picks it: a hand-edited "22" or "lts" names the
// newest installed match. It returns the default as written when nothing
// installed matches, and "" when there is no default.
func resolvedDefault(cfg *config.GlobalConfig, tool string) string {
	def := cfg.Default.GetTool(tool)
	if def == "" {
		return ""
	}
	if v, ok, err := resolver.ResolvePin(tool, def); err == nil && ok {
		return v
	}
	return def
}

func newDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <tool@version>",
		Short: "Set the global default version for a tool",
		Long:  "Set which version is used outside of pinned projects.\n\nExamples:\n  driftr default node@24.0.0\n  driftr default node@24",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool, versionSpec := parseToolVersion(args[0])
			tool, err := versionedTool(tool)
			if err != nil {
				return err
			}

			versionStr, _, err := resolver.RequireToolInstalled(tool, versionSpec)
			if err != nil {
				return err
			}

			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}

			cfg.Default.SetTool(tool, versionStr)
			if err := config.SaveGlobal(cfg); err != nil {
				return err
			}

			fmt.Println(ioutil.Success(fmt.Sprintf("Set global default to %s %s", tool, ioutil.Bold(versionStr))))
			return nil
		},
	}
}
