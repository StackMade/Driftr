package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/nodeenv"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/resolver"
)

// pruneScopeNote states what prune can and cannot see. It is printed on every
// run that is about to delete something, because the limit is the whole risk:
// a version another checkout pins looks unreferenced from here.
const pruneScopeNote = "Prune only knows about the global default and the pins of the current directory.\nA version that another project pins is not protected here."

// pruneCandidate is one installed version selected for removal.
type pruneCandidate struct {
	tool    string
	version string
	dir     string
	size    int64
}

func newPruneCmd() *cobra.Command {
	var dryRun, yes bool
	var toolFlag string

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove installed versions nothing references",
		Long: "Remove installed tool versions that neither the global default nor the current project uses.\n\n" +
			pruneScopeNote + "\nStart with --dry-run if you keep pinned projects elsewhere on this machine.\n\n" +
			"Examples:\n  driftr prune --dry-run\n  driftr prune\n  driftr prune --tool node -y",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tools := versionedTools
			if toolFlag != "" {
				entry, ok := platform.LookupTool(toolFlag)
				if !ok {
					return fmt.Errorf("unknown tool: %s. Supported tools: %s", toolFlag, strings.Join(versionedTools, ", "))
				}
				tools = []string{entry.Parent}
			}

			candidates, err := prunable(tools)
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				fmt.Println(ioutil.Success("Nothing to prune."))
				return nil
			}

			var total int64
			fmt.Printf("These versions are not referenced and would be removed:\n\n")
			for _, c := range candidates {
				total += c.size
				fmt.Printf("  %s %s  %s\n", c.tool, ioutil.Bold(c.version), ioutil.Dim(formatSize(c.size)))
			}
			fmt.Printf("\nFreeing %s.\n\n", ioutil.Bold(formatSize(total)))
			fmt.Println(ioutil.Warn(pruneScopeNote))

			if dryRun {
				fmt.Println(ioutil.Dim("\nDry run: nothing was removed."))
				return nil
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), fmt.Sprintf("\nRemove %d version(s)? [y/N] ", len(candidates)))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Println(ioutil.Dim("Aborted, nothing was removed."))
					return nil
				}
			}

			return removeCandidates(candidates)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be removed and exit")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Remove without asking for confirmation")
	cmd.Flags().StringVar(&toolFlag, "tool", "", "Only prune versions of this tool")

	return cmd
}

// prunable lists the installed versions of tools that nothing references.
// Every failure to classify aborts the whole run: a version that cannot be
// proven unreferenced must survive, and the safest way to guarantee that is to
// delete nothing at all.
func prunable(tools []string) ([]pruneCandidate, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, fmt.Errorf("cannot read the global config, refusing to prune: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}
	pins, err := resolver.ProjectPins(cwd)
	if err != nil {
		return nil, fmt.Errorf("cannot read the project pins, refusing to prune: %w", err)
	}

	var candidates []pruneCandidate
	for _, tool := range tools {
		keep := map[string]bool{}

		// The global default may be written as a full version, but resolve it
		// anyway so a hand-edited partial protects what it would select.
		if def := cfg.Default.GetTool(tool); def != "" {
			keep[def] = true
			if err := keepResolved(keep, tool, def); err != nil {
				return nil, err
			}
		}

		for _, pin := range pins {
			if pin.Tool != tool {
				continue
			}
			keep[pin.Version] = true
			if err := keepResolved(keep, tool, pin.Version); err != nil {
				return nil, err
			}
		}

		versions, err := platform.ListToolVersions(tool)
		if err != nil {
			return nil, fmt.Errorf("cannot list installed %s versions, refusing to prune: %w", tool, err)
		}
		for _, ver := range versions {
			if keep[ver] {
				continue
			}
			dir, err := platform.ToolVersionDir(tool, ver)
			if err != nil {
				return nil, err
			}
			size, err := nodeenv.DirSize(dir)
			if err != nil {
				return nil, fmt.Errorf("cannot measure %s %s, refusing to prune: %w", tool, ver, err)
			}
			candidates = append(candidates, pruneCandidate{tool: tool, version: ver, dir: dir, size: size})
		}
	}

	return candidates, nil
}

// keepResolved marks the installed version that spec selects, if any.
func keepResolved(keep map[string]bool, tool, spec string) error {
	resolved, ok, err := resolver.ResolvePin(tool, spec)
	if err != nil {
		return fmt.Errorf("cannot resolve %s@%s, refusing to prune: %w", tool, spec, err)
	}
	if ok {
		keep[resolved] = true
	}
	return nil
}

// removeCandidates deletes each version directory. One failure does not stop
// the rest: the errors are collected and reported together at the end.
func removeCandidates(candidates []pruneCandidate) error {
	var failures []error
	var freed int64
	removed := 0

	for _, c := range candidates {
		if verbose {
			fmt.Println(ioutil.Dim(fmt.Sprintf("  Removing: %s", c.dir)))
		}
		if err := os.RemoveAll(c.dir); err != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w. Manual cleanup: rm -rf %q", c.tool, c.version, err, c.dir))
			fmt.Println(ioutil.Failure(fmt.Sprintf("Failed to remove %s %s: %v", c.tool, c.version, err)))
			continue
		}
		removed++
		freed += c.size
	}

	fmt.Println(ioutil.Success(fmt.Sprintf("Removed %d version(s), freed %s.", removed, ioutil.Bold(formatSize(freed)))))

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d version(s) could not be removed: %w", len(failures), len(candidates), errors.Join(failures...))
	}
	return nil
}

// confirm asks a yes/no question and reports whether the answer was yes.
// Anything other than "y" or "yes" is a no, including end of input.
func confirm(in io.Reader, prompt string) (bool, error) {
	fmt.Print(prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, fmt.Errorf("cannot read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
