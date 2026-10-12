package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/resolver"
	"github.com/stackmade/driftr/internal/version"
)

// pinProblem is one thing `driftr check` reports, with what to do about it.
type pinProblem struct {
	msg  string
	hint string
}

func newCheckCmd() *cobra.Command {
	var exitCode bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check that the project's version pins agree with each other",
		Long: "List every file that pins a tool version for the current directory, mark the one\n" +
			"the resolver uses, and report the ones that disagree with it. The Node.js version\n" +
			"in use is also checked against every engines.node range on the way up.\n\n" +
			"Two pins agree when at least one version satisfies both: \"22\" and \"22.14.0\"\n" +
			"agree, \"22.14\" and \"22.15.0\" do not. Nothing is fetched from the network.\n\n" +
			"Examples:\n" +
			"  driftr check\n" +
			"  driftr check --exit-code",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine current directory: %w", err)
			}
			return runCheck(cmd.OutOrStdout(), cwd, exitCode)
		},
	}

	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "Exit non-zero when a problem is found (for CI)")

	return cmd
}

func runCheck(w io.Writer, cwd string, exitCode bool) error {
	fail := func() error {
		if exitCode {
			return &ExitError{Code: 1}
		}
		return nil
	}

	pins, err := resolver.ProjectSources(cwd)
	if err != nil {
		// The resolver stops on the same error, so no pin below it means anything.
		fmt.Fprintln(w, ioutil.Failure(err.Error()))
		fmt.Fprintln(w, "    "+ioutil.Dim("Fix the file, then run `driftr check` again."))
		return fail()
	}
	if len(pins) == 0 {
		fmt.Fprintf(w, "No version pins found in %s or its parent directories.\n", cwd)
		return nil
	}

	byTool := map[string][]resolver.Pin{}
	var tools []string
	for _, p := range pins {
		if _, seen := byTool[p.Tool]; !seen {
			tools = append(tools, p.Tool)
		}
		byTool[p.Tool] = append(byTool[p.Tool], p)
	}

	total := 0
	for i, tool := range tools {
		if i > 0 {
			fmt.Fprintln(w)
		}
		toolPins := byTool[tool]
		printPinSources(w, cwd, toolPins)
		problems := checkToolPins(cwd, toolPins, resolver.ResolvePin)
		for _, p := range problems {
			fmt.Fprintln(w, "  "+ioutil.Failure(p.msg))
			fmt.Fprintln(w, "      "+ioutil.Dim(p.hint))
		}
		total += len(problems)
	}

	fmt.Fprintln(w)
	if total == 0 {
		fmt.Fprintln(w, ioutil.Success(ioutil.Bold("All pins agree.")))
		return nil
	}
	fmt.Fprintln(w, ioutil.Warn(fmt.Sprintf("Found %d problem(s).", total)))
	return fail()
}

// printPinSources prints a tool's sources, the winning one first and marked.
func printPinSources(w io.Writer, cwd string, pins []resolver.Pin) {
	win := pins[0]
	head := ioutil.Bold(win.Tool) + "  " + win.Version
	if win.Source != resolver.SourceEnginesNode {
		if v, ok, err := resolver.ResolvePin(win.Tool, win.Version); err == nil {
			switch {
			case !ok:
				head += ioutil.Dim(" (not installed)")
			case v != win.Version:
				head += ioutil.Dim(" (resolves to " + v + ")")
			}
		}
	}
	fmt.Fprintln(w, head)

	width := 0
	for _, p := range pins {
		width = max(width, len(pinLabel(cwd, p)))
	}
	for i, p := range pins {
		// Pad before coloring: ANSI codes would throw the column off.
		label := fmt.Sprintf("%-*s", width, pinLabel(cwd, p))
		if i == 0 {
			fmt.Fprintf(w, "  %s %s  %s\n", ioutil.Green(">"), label, ioutil.Bold(p.Version))
			continue
		}
		fmt.Fprintf(w, "    %s  %s\n", label, p.Version)
	}
}

// pinLabel names a source by its file relative to cwd, plus the field for the
// sources that share package.json.
func pinLabel(cwd string, p resolver.Pin) string {
	path := filepath.Join(p.Dir, p.Source.File())
	if rel, err := filepath.Rel(cwd, path); err == nil {
		path = rel
	}
	switch p.Source {
	case resolver.SourcePackageJSON:
		return path + " (driftr)"
	case resolver.SourcePackageManager:
		return path + " (packageManager)"
	case resolver.SourceEnginesNode:
		return path + " (engines.node)"
	}
	return path
}

// pinRange returns the versions a pin accepts. engines.node is a semver range;
// every other source holds a version or an alias, matched as the resolver
// matches installed versions.
func pinRange(cwd string, p resolver.Pin) (version.Range, *pinProblem, error) {
	if p.Source == resolver.SourceEnginesNode {
		r, err := version.ParseRange(p.Version)
		return r, nil, err
	}
	v, err := version.Parse(p.Version)
	if err != nil {
		return version.Range{}, nil, err
	}
	r, ok := v.Range()
	if !ok {
		return version.Range{}, &pinProblem{
			msg:  fmt.Sprintf("%s pins %s, an LTS codename this driftr does not know, so it cannot be checked offline", pinLabel(cwd, p), p.Version),
			hint: "Pin a version number instead, or run `driftr self-update` if the line is new.",
		}, nil
	}
	return r, nil, nil
}

// checkToolPins compares every source of one tool with the first, which is
// the one the resolver uses. pins must be non-empty and in resolution order.
//
// Two sources agree when at least one version satisfies both: "22" and
// "22.14.0" agree, "22.14" and "22.15.0" do not, "lts" agrees with any line
// known to be LTS. engines.node is stricter: the version in use must satisfy
// it, meaning the installed version the winning pin resolves to, or the pin
// itself when nothing matching is installed.
//
// resolve is resolver.ResolvePin; tests pass a fake.
func checkToolPins(cwd string, pins []resolver.Pin, resolve func(tool, spec string) (string, bool, error)) []pinProblem {
	var problems []pinProblem
	ranges := make([]*version.Range, len(pins))
	for i, p := range pins {
		r, unknown, err := pinRange(cwd, p)
		switch {
		case err != nil:
			label := pinLabel(cwd, p)
			problems = append(problems, pinProblem{
				msg:  fmt.Sprintf("%s: cannot read %q: %s", label, p.Version, err),
				hint: fmt.Sprintf("Edit %s to hold a valid version.", label),
			})
		case unknown != nil:
			problems = append(problems, *unknown)
		default:
			ranges[i] = &r
		}
	}

	win := pins[0]
	if ranges[0] == nil {
		return problems // nothing to compare against
	}
	winLabel := pinLabel(cwd, win)
	pinnable := pinnableHere(cwd, win)

	for i, p := range pins[1:] {
		r := ranges[i+1]
		if r == nil {
			continue
		}
		label := pinLabel(cwd, p)

		if p.Source == resolver.SourceEnginesNode && win.Source != resolver.SourceEnginesNode {
			inUse, installed := "", version.Version{}
			if v, found, err := resolve(win.Tool, win.Version); err == nil && found {
				if pv, err := version.Parse(v); err == nil {
					inUse, installed = v, pv
				}
			}
			if inUse != "" && r.Matches(installed) {
				continue
			}
			if inUse == "" && r.Intersects(*ranges[0]) {
				continue
			}
			what := fmt.Sprintf("%s %s from %s", win.Tool, win.Version, winLabel)
			if inUse != "" && inUse != win.Version {
				what += " resolves to " + inUse + ", which"
			}
			hint := fmt.Sprintf("Pin a version inside %q in %s, or widen the range in %s.", p.Version, winLabel, label)
			if major, has := r.LowerBoundMajor(); has && pinnable {
				hint = fmt.Sprintf("Run `driftr pin %s@%d`, or widen the range in %s.", win.Tool, major, label)
			} else if has {
				hint = fmt.Sprintf("Pin a version inside %q in %s (for example %d), or widen the range in %s.", p.Version, winLabel, major, label)
			}
			problems = append(problems, pinProblem{
				msg:  fmt.Sprintf("%s is outside %s %q", what, label, p.Version),
				hint: hint,
			})
			continue
		}

		if r.Intersects(*ranges[0]) {
			continue
		}
		hint := fmt.Sprintf("Change %s to agree with %s, or change %s if %s is the version you want.", label, win.Version, winLabel, p.Version)
		if pinnable {
			hint = fmt.Sprintf("Change %s to agree with %s, or run `driftr pin %s@%s` if %s is the version you want.", label, win.Version, win.Tool, p.Version, p.Version)
		}
		problems = append(problems, pinProblem{
			msg:  fmt.Sprintf("%s says %s, but %s says %s and wins", label, p.Version, winLabel, win.Version),
			hint: hint,
		})
	}
	return problems
}

// pinnableHere reports whether `driftr pin`, run in cwd, would rewrite the
// winning pin rather than add a new one in front of it.
func pinnableHere(cwd string, win resolver.Pin) bool {
	if win.Dir != cwd {
		return false
	}
	switch detectPinFormat(cwd) {
	case formatTOML:
		return win.Source == resolver.SourceProject
	case formatPackageJSON:
		return win.Source == resolver.SourcePackageJSON
	}
	return false
}
