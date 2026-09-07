package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/installer"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/resolver"
	"github.com/stackmade/driftr/internal/version"
)

// outdated status states, in reporting order of severity.
const (
	stateCurrent = iota
	stateBehindMajor
	stateBehind
	stateUnknown
)

// outdatedRow is one line of the report: what this tool is on now and what
// upstream has.
type outdatedRow struct {
	tool    string
	current string
	source  string
	line    string // newest upstream release on the current major line
	latest  string // newest upstream release overall
	lts     string // newest LTS release (node only)
	status  string
	state   int
}

func newOutdatedCmd() *cobra.Command {
	var only string
	var pre bool
	var exitCode bool

	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "Compare pinned versions against upstream releases",
		Long: "Compare the versions this project resolves to against the newest releases upstream.\n\n" +
			"For each tool the report shows the version in use, where that version came from,\n" +
			"the newest release on the same major line, and the newest release overall.\n" +
			"Node.js also gets its newest LTS.\n\n" +
			"Examples:\n" +
			"  driftr outdated\n" +
			"  driftr outdated --tool node\n" +
			"  driftr outdated --pre\n" +
			"  driftr outdated --exit-code",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutdated(cmd.OutOrStdout(), only, pre, exitCode)
		},
	}

	cmd.Flags().StringVar(&only, "tool", "", "Check a single tool")
	cmd.Flags().BoolVar(&pre, "pre", false, "Include pre-release versions (npm packages only)")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "Exit non-zero when a tool is behind (for CI)")

	return cmd
}

func runOutdated(w io.Writer, only string, includePre, exitCode bool) error {
	tools := versionedTools
	if only != "" {
		found := false
		for _, t := range versionedTools {
			if t == only {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown tool %q (known: %s)", only, strings.Join(versionedTools, ", "))
		}
		tools = []string{only}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	pins, _ := resolver.ProjectPins(cwd)
	cfg, _ := config.LoadGlobal()

	var rows []outdatedRow
	failed := 0
	behind := 0
	for _, tool := range tools {
		cur, source := outdatedCurrent(tool, pins, cfg)
		if cur == "" {
			continue // nothing installed, nothing pinned
		}
		row := outdatedRow{tool: tool, current: cur, source: source, lts: "-"}
		cv, parseErr := version.Parse(cur)

		major := -1
		if parseErr == nil {
			major = cv.Major
		}
		latest, line, lts, fetchErr := upstreamVersions(tool, major, includePre)
		switch {
		case fetchErr != nil:
			row.state, row.status = stateUnknown, "unknown: "+fetchErr.Error()
			failed++
		default:
			row.latest, row.line = latest, line
			if lts != "" {
				row.lts = lts
			}
			row.state, row.status = outdatedStatus(cur, cv, parseErr, latest, line)
			if row.state == stateBehind || row.state == stateBehindMajor {
				behind++
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		fmt.Fprintln(w, "Nothing to check — no versions installed and no pins found.")
		fmt.Fprintln(w, "Run `driftr install node@lts` to get started.")
		return nil
	}

	printOutdated(w, rows)

	if failed == len(rows) {
		return fmt.Errorf("could not reach any upstream release source")
	}
	if exitCode && behind > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

// outdatedCurrent reports the version a tool is on right now and where that
// came from. Resolution wins; an uninstalled pin or default still counts, so
// that `outdated` works before `install` does.
func outdatedCurrent(tool string, pins []resolver.Pin, cfg *config.GlobalConfig) (ver, source string) {
	if res, err := resolver.ResolveTool(tool, "", false); err == nil && res != nil {
		return res.Version, res.Source.String()
	}
	for _, p := range pins {
		if p.Tool == tool {
			return p.Version, p.Source.String() + " (not installed)"
		}
	}
	if cfg != nil {
		if v := cfg.Default.GetTool(tool); v != "" {
			return v, "global default (not installed)"
		}
	}
	if versions, err := installer.ListInstalledToolVersions(tool); err == nil {
		if newest := newestVersion(versions); newest != "" {
			return newest, "installed"
		}
	}
	return "", ""
}

// upstreamVersions returns the newest upstream release, the newest release on
// the given major line, and the newest LTS release (node only). A major of -1
// means the current version could not be parsed, so no line is reported.
func upstreamVersions(tool string, major int, includePre bool) (latest, line, lts string, err error) {
	if tool == "node" {
		releases, err := installer.FetchNodeIndex()
		if err != nil {
			return "", "", "", err
		}
		if len(releases) == 0 {
			return "", "", "", fmt.Errorf("no Node.js releases found")
		}
		// The index is newest-first, so the first match in each category wins.
		latest = releases[0].Version
		for _, rel := range releases {
			if lts == "" && rel.IsLTS() {
				lts = rel.Version
			}
			if line == "" && major >= 0 {
				if rv, perr := version.Parse(rel.Version); perr == nil && rv.Major == major {
					line = rel.Version
				}
			}
		}
		return latest, line, lts, nil
	}

	pkg, ok := npmPackage[tool]
	if !ok {
		return "", "", "", fmt.Errorf("no upstream release source for %s", tool)
	}
	versions, err := installer.ListRemoteVersions(pkg, includePre)
	if err != nil {
		return "", "", "", err
	}
	if len(versions) == 0 {
		return "", "", "", fmt.Errorf("no %s releases found", tool)
	}
	latest = versions[0]
	if major >= 0 {
		for _, v := range versions {
			if pv, perr := version.Parse(v); perr == nil && pv.Major == major {
				line = v
				break
			}
		}
	}
	return latest, line, "", nil
}

// outdatedStatus classifies a current version against upstream. Aliases and
// partial pins are deliberately not called "behind" on patch level: they float,
// so only a newer major line is news.
func outdatedStatus(raw string, cv version.Version, parseErr error, latest, line string) (int, string) {
	lv, lerr := version.Parse(latest)
	if parseErr != nil || lerr != nil {
		return stateUnknown, "unknown"
	}

	if cv.Latest || cv.LTS {
		return stateCurrent, "tracks " + raw
	}

	if cv.IsPartial() {
		if lv.Major > cv.Major {
			return stateBehindMajor, fmt.Sprintf("newer major: %d.x", lv.Major)
		}
		return stateCurrent, "up to date"
	}

	if compareVersions(cv, lv) >= 0 {
		return stateCurrent, "up to date"
	}
	if line != "" && line != latest {
		if pv, err := version.Parse(line); err == nil {
			if compareVersions(cv, pv) >= 0 {
				return stateBehindMajor, fmt.Sprintf("newest in %d.x, newer major: %s", cv.Major, latest)
			}
			return stateBehind, fmt.Sprintf("behind: %s in %d.x, latest %s", line, cv.Major, latest)
		}
	}
	return stateBehind, "behind: " + latest
}

func compareVersions(a, b version.Version) int {
	if c := cmp.Compare(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Minor, b.Minor); c != 0 {
		return c
	}
	return cmp.Compare(a.Patch, b.Patch)
}

// newestVersion returns the highest parseable version in the list.
func newestVersion(versions []string) string {
	best := ""
	var bestV version.Version
	for _, v := range versions {
		pv, err := version.Parse(v)
		if err != nil {
			continue
		}
		if best == "" || compareVersions(pv, bestV) > 0 {
			best, bestV = v, pv
		}
	}
	return best
}

// printOutdated renders the rows aligned. Colour is applied per line after the
// tabwriter has flushed, because ANSI escapes count towards cell width and
// would throw the columns off.
func printOutdated(w io.Writer, rows []outdatedRow) {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tCURRENT\tSOURCE\tLINE LATEST\tLATEST\tLTS\tSTATUS")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.tool, r.current, r.source, dash(r.line), dash(r.latest), r.lts, r.status)
	}
	tw.Flush()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	fmt.Fprintln(w, ioutil.Bold(lines[0]))
	for i, r := range rows {
		line := lines[i+1]
		switch r.state {
		case stateCurrent:
			line = ioutil.Dim(line)
		case stateBehind:
			line = ioutil.Yellow(line)
		case stateUnknown:
			line = ioutil.Red(line)
		}
		fmt.Fprintln(w, line)
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
