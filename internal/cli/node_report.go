package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/nodeenv"
)

func newNodeReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Report dependency storage usage",
		Long:  "Show the current project's node_modules size alongside the shared pnpm store size.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			r := nodeenv.NewExecRunner()
			return runNodeReport(nodeenv.NewPnpm(r), dir, cmd.OutOrStdout())
		},
	}
}

// runNodeReport prints node_modules and shared-store sizes for the current
// project. Scope is intentionally single-project: a cross-project "savings"
// figure would be fabricated here, so the report states real sizes plus a note
// that the store is shared machine-wide.
func runNodeReport(p *nodeenv.Pnpm, projectDir string, w io.Writer) error {
	nodeModulesSize, _ := nodeenv.DirSize(filepath.Join(projectDir, "node_modules"))

	var storeSize int64
	storeKnown := false
	storeNote := "unknown"
	switch {
	case !p.Installed():
		storeNote = "unknown (pnpm not found)"
	default:
		path, err := p.StorePath()
		if err != nil || path == "" {
			storeNote = "unknown (could not determine store path)"
		} else if s, err := nodeenv.DirSize(path); err != nil {
			storeNote = "unknown (could not measure store)"
		} else {
			storeSize = s
			storeKnown = true
		}
	}

	row := func(label, value string) {
		fmt.Fprintf(w, "%s %s\n", ioutil.Label(fmt.Sprintf("%-21s", label)), value)
	}

	fmt.Fprintln(w, ioutil.Title("Driftr Storage Report"))
	fmt.Fprintln(w)
	row("Project:", projectDir)
	row("Project node_modules:", ioutil.Bold(formatSize(nodeModulesSize)))
	if storeKnown {
		row("Shared pnpm store:", ioutil.Bold(formatSize(storeSize)))
	} else {
		row("Shared pnpm store:", ioutil.Dim(storeNote))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, ioutil.Dim("The shared pnpm store is reused across every project on this machine,"))
	fmt.Fprintln(w, ioutil.Dim("so its cost is amortized — more projects means greater savings overall."))
	return nil
}
