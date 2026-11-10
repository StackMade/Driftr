package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/agents"
	"github.com/stackmade/driftr/v2/internal/ioutil"
)

func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agents",
		Aliases: []string{"agent"},
		Short:   "Teach AI coding agents how to use driftr",
		Long: "Install a skill that tells AI coding agents (Claude Code, Codex, Cursor, ...) how to use driftr.\n\n" +
			"Installed copies follow this driftr: they are refreshed after every upgrade.\n" +
			"For an agent without a skills directory, append the guide to its instructions:\n\n" +
			"  driftr agents show >> AGENTS.md",
	}

	cmd.AddCommand(newAgentsListCmd(), newAgentsShowCmd(), newAgentsInstallCmd(), newAgentsUninstallCmd())

	return cmd
}

func newAgentsListCmd() *cobra.Command {
	var detectedOnly bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported agents and whether each has the skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := agents.All()
			if err != nil {
				return err
			}
			if detectedOnly {
				for _, h := range all {
					if h.Detected {
						fmt.Fprintln(cmd.OutOrStdout(), h.Name)
					}
				}
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, h := range all {
				status, err := agents.StatusOf(h.File)
				if err != nil {
					return err
				}
				found := "not found"
				if h.Detected {
					found = "detected"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", h.Name, found, status, h.File)
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&detectedOnly, "detected", false, "Print only the names of agents found on this machine")

	return cmd
}

func newAgentsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the guide, to paste into any agent's instructions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), agents.Guide)
			return err
		},
	}
}

func newAgentsInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install [agent...]",
		Short: "Install the skill for the named agents, or for every agent found",
		Long: "Write the driftr skill into the skills directory of each named agent. With no names, every\n" +
			"agent found on this machine gets it. A skill named driftr that driftr did not write is left alone.\n\n" +
			"Examples:\n  driftr agents install\n  driftr agents install claude codex",
		RunE: func(cmd *cobra.Command, args []string) error {
			harnesses, err := selectHarnesses(args)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				harnesses = slices.DeleteFunc(harnesses, func(h agents.Harness) bool { return !h.Detected })
				if len(harnesses) == 0 {
					return errors.New("no coding agent found in your home directory. Name one, e.g. `driftr agents install claude`, or run `driftr agents list`")
				}
			}

			var errs []error
			for _, t := range byFile(harnesses) {
				if err := agents.Install(t.file); err != nil {
					errs = append(errs, err)
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), ioutil.Success(fmt.Sprintf("%s: %s", strings.Join(t.names, ", "), t.file)))
			}
			return errors.Join(errs...)
		},
	}
}

func newAgentsUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [agent...]",
		Short: "Remove the skill from the named agents, or from all of them",
		Long: "Remove the driftr skill. With no names, every copy driftr wrote is removed.\n" +
			"Several agents can share one copy, so removing it for one removes it for those too.",
		RunE: func(cmd *cobra.Command, args []string) error {
			harnesses, err := selectHarnesses(args)
			if err != nil {
				return err
			}

			removedAny := false
			var errs []error
			for _, t := range byFile(harnesses) {
				removed, err := agents.Uninstall(t.file)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				if removed {
					removedAny = true
					fmt.Fprintln(cmd.OutOrStdout(), ioutil.Success("Removed "+t.file))
				}
			}
			if !removedAny && len(errs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), ioutil.Dim("Nothing to remove."))
			}
			return errors.Join(errs...)
		},
	}
}

// selectHarnesses returns the named agents, or all of them for no names.
func selectHarnesses(names []string) ([]agents.Harness, error) {
	all, err := agents.All()
	if err != nil || len(names) == 0 {
		return all, err
	}

	selected := make([]agents.Harness, 0, len(names))
	for _, name := range names {
		i := slices.IndexFunc(all, func(h agents.Harness) bool { return h.Name == name })
		if i < 0 {
			supported := make([]string, 0, len(all))
			for _, h := range all {
				supported = append(supported, h.Name)
			}
			return nil, fmt.Errorf("unknown agent: %s. Supported agents: %s", name, strings.Join(supported, ", "))
		}
		selected = append(selected, all[i])
	}
	return selected, nil
}

// skillTarget is one skill file and the agents that read it.
type skillTarget struct {
	file  string
	names []string
}

// byFile groups agents by skill file, so a shared directory is written once.
func byFile(harnesses []agents.Harness) []skillTarget {
	var targets []skillTarget
	for _, h := range harnesses {
		i := slices.IndexFunc(targets, func(t skillTarget) bool { return t.file == h.File })
		if i < 0 {
			targets = append(targets, skillTarget{file: h.File})
			i = len(targets) - 1
		}
		if !slices.Contains(targets[i].names, h.Name) {
			targets[i].names = append(targets[i].names, h.Name)
		}
	}
	return targets
}
