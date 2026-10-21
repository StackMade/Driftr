package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/config"
	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/nodeenv"
	"github.com/stackmade/driftr/v2/internal/platform"
	"github.com/stackmade/driftr/v2/internal/resolver"
)

// pruneScopeNote states what prune can and cannot see. It is printed on every
// run that is about to delete something, because the limit is the whole risk:
// a version pinned by a checkout driftr has never recorded looks unreferenced.
const pruneScopeNote = "Prune protects the global default, the DRIFTR_<TOOL> override of this shell, the current directory and every project recorded by driftr pin or driftr install.\nA project pinned before driftr recorded projects, or only by editing files, is not protected until you run driftr install there once."

// pruneScope is everything prune reads pins from.
type pruneScope struct {
	pins     []resolver.Pin
	projects []string // recorded projects whose pins were read
	gone     []string // recorded projects whose directory no longer exists
}

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
		Long: "Remove installed tool versions that neither the global default nor any known project uses.\n\n" +
			pruneScopeNote + "\nRecorded projects live in ~/.driftr/config/projects; those whose directory is gone are forgotten.\n" +
			"Start with --dry-run to see which projects were checked.\n\n" +
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

			scope, err := loadPruneScope()
			if err != nil {
				return err
			}
			reportPruneScope(scope, dryRun)

			candidates, err := prunable(tools, scope.pins)
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

// loadPruneScope collects the pins of the current directory and of every
// recorded project. A recorded project that cannot be read aborts the run like
// any other failure: its pins are unknown, so nothing can be proven unused.
func loadPruneScope() (pruneScope, error) {
	var scope pruneScope

	cwd, err := os.Getwd()
	if err != nil {
		return scope, fmt.Errorf("cannot determine working directory: %w", err)
	}
	scope.pins, err = resolver.ProjectPins(cwd)
	if err != nil {
		return scope, fmt.Errorf("cannot read the project pins, refusing to prune: %w", err)
	}

	registry, err := platform.ProjectsRegistryPath()
	if err != nil {
		return scope, err
	}
	dirs, err := config.LoadProjects()
	if err != nil {
		return scope, fmt.Errorf("cannot read %s, refusing to prune: %w", registry, err)
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
			scope.gone = append(scope.gone, dir)
			continue
		}
		if err != nil {
			return scope, fmt.Errorf("cannot check recorded project %s, refusing to prune: %w. Fix access to it, or delete its line from %s", dir, err, registry)
		}
		pins, err := resolver.ProjectPins(dir)
		if err != nil {
			return scope, fmt.Errorf("cannot read the pins of recorded project %s, refusing to prune: %w. Fix the file, or delete its line from %s if the project is no longer used", dir, err, registry)
		}
		scope.pins = append(scope.pins, pins...)
		scope.projects = append(scope.projects, dir)
	}
	return scope, nil
}

// reportPruneScope says which recorded projects were checked and forgets the
// ones whose directory is gone. A dry run lists the projects and forgets
// nothing.
func reportPruneScope(scope pruneScope, dryRun bool) {
	fmt.Println(ioutil.Dim(fmt.Sprintf("Checked %d recorded project(s).", len(scope.projects))))
	if dryRun {
		for _, dir := range scope.projects {
			fmt.Println(ioutil.Bullet(dir))
		}
		for _, dir := range scope.gone {
			fmt.Println(ioutil.Bullet(dir + " (gone, would be forgotten)"))
		}
		return
	}
	if len(scope.gone) == 0 {
		return
	}
	if err := config.RemoveProjects(scope.gone); err != nil {
		fmt.Fprintln(os.Stderr, ioutil.Warn(fmt.Sprintf("Could not forget projects that no longer exist: %v", err)))
		return
	}
	fmt.Println(ioutil.Dim(fmt.Sprintf("Forgot %d recorded project(s) whose directory no longer exists.", len(scope.gone))))
}

// prunable lists the installed versions of tools that neither the global
// default nor any of pins references. Every failure to classify aborts the
// whole run: a version that cannot be proven unreferenced must survive, and
// the safest way to guarantee that is to delete nothing at all.
func prunable(tools []string, pins []resolver.Pin) ([]pruneCandidate, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, fmt.Errorf("cannot read the global config, refusing to prune: %w", err)
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

		// DRIFTR_<TOOL> overrides every pin in this shell, so what it selects
		// is in use here, partial specs resolved as the resolver does.
		if spec := strings.TrimSpace(os.Getenv(resolver.EnvVarName(tool))); spec != "" {
			if err := keepResolved(keep, tool, spec); err != nil {
				return nil, err
			}
		}

		for _, pin := range pins {
			if pin.Tool != tool {
				continue
			}
			if pin.Range != "" {
				// engines.node: keep what the shim runs, the newest installed
				// match, not the major install would fetch.
				v, ok, err := resolver.ResolveRange(tool, pin.Range)
				if err != nil {
					return nil, fmt.Errorf("cannot resolve engines.node %q in %s, refusing to prune: %w", pin.Range, pin.Dir, err)
				}
				if ok {
					keep[v] = true
				}
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
		if err := platform.RemoveVersionDir(c.dir); err != nil {
			failures = append(failures, fmt.Errorf("%s %s: %w", c.tool, c.version, err))
			fmt.Println(ioutil.Failure(fmt.Sprintf("Failed to remove %s %s: %v", c.tool, c.version, err)))
			continue
		}
		removed++
		freed += c.size
	}

	fmt.Println(ioutil.Success(fmt.Sprintf("Removed %d version(s), freed %s.", removed, ioutil.Bold(formatSize(freed)))))
	if removed > 0 {
		refreshShims()
	}

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

// rememberProject records the project dir belongs to, if any, so prune run
// from anywhere protects its pins. It is called by pin and install, never on
// the shim path, and never fails the command it is called from.
func rememberProject(dir string) {
	pins, err := resolver.ProjectPins(dir)
	if err != nil {
		return // best effort: a broken pin file is for the resolver to report
	}
	recordProject(pins)
}

// recordProject records the directory holding the nearest pin. Every other pin
// sits in one of its parents, so resolving from it finds the same pins as
// resolving from anywhere below it.
func recordProject(pins []resolver.Pin) {
	if len(pins) == 0 {
		return
	}
	root := pins[0].Dir
	for _, p := range pins[1:] {
		if len(p.Dir) > len(root) {
			root = p.Dir
		}
	}
	if err := config.AddProject(root); err != nil {
		fmt.Fprintln(os.Stderr, ioutil.Warn(fmt.Sprintf("Could not record %s for driftr prune: %v", root, err)))
	}
}
