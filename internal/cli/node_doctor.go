package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/nodeenv"
)

// nodeDoctorInfo is the gathered state rendered by `driftr node doctor`.
type nodeDoctorInfo struct {
	projectDir         string
	packageManager     nodeenv.PackageManager
	nodeVersion        string // "" when node not found
	corepackAvailable  bool
	pnpmInstalled      bool
	pnpmVersion        string
	nodeModulesSize    int64
	storePath          string // "" when pnpm missing/unknown
	globalVirtualStore bool
}

func newNodeDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Analyze the Node.js dependency environment",
		Long:  "Inspect the current project's package manager and pnpm shared-store configuration, and recommend optimizations.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			r := nodeenv.NewExecRunner()
			info := gatherNodeDoctor(r, dir)
			renderNodeDoctor(cmd.OutOrStdout(), info)
			return nil
		},
	}
}

// gatherNodeDoctor collects environment state. It never fails on missing
// tooling — absence is reported, not treated as an error.
func gatherNodeDoctor(r nodeenv.Runner, projectDir string) nodeDoctorInfo {
	p := nodeenv.NewPnpm(r)
	info := nodeDoctorInfo{
		projectDir:        projectDir,
		packageManager:    nodeenv.DetectPackageManager(projectDir),
		corepackAvailable: p.CorepackAvailable(),
		pnpmInstalled:     p.Installed(),
	}

	if v, err := r.Run("node", "--version"); err == nil {
		info.nodeVersion = strings.TrimPrefix(v, "v")
	}

	if size, err := nodeenv.DirSize(filepath.Join(projectDir, "node_modules")); err == nil {
		info.nodeModulesSize = size
	}

	if info.pnpmInstalled {
		if v, err := p.Version(); err == nil {
			info.pnpmVersion = v
		}
		if path, err := p.StorePath(); err == nil {
			info.storePath = path
		}
		if val, err := p.ConfigGet(nodeenv.GlobalVirtualStoreKey); err == nil {
			info.globalVirtualStore = val == "true"
		}
	}

	return info
}

func renderNodeDoctor(w io.Writer, info nodeDoctorInfo) {
	row := func(label, value string) {
		// Pad the plain label before coloring so alignment is unaffected by
		// the (zero-width at runtime) ANSI escape codes.
		fmt.Fprintf(w, "%s %s\n", ioutil.Label(fmt.Sprintf("%-21s", label)), value)
	}

	fmt.Fprintln(w, ioutil.Title("Driftr Node Doctor"))
	fmt.Fprintln(w)
	row("Project:", info.projectDir)
	row("Package manager:", string(info.packageManager))
	row("Node.js:", orNotFound(info.nodeVersion))
	row("Corepack:", availability(info.corepackAvailable))

	if info.pnpmInstalled {
		row("pnpm:", orNotFound(info.pnpmVersion))
	} else {
		row("pnpm:", ioutil.Red("not found"))
	}

	if info.nodeModulesSize > 0 {
		row("node_modules:", formatSize(info.nodeModulesSize))
	} else {
		row("node_modules:", ioutil.Dim("none"))
	}

	row("pnpm store:", orUnknown(info.storePath))
	row("global virtual store:", enabledDisabled(info.globalVirtualStore))

	if !info.pnpmInstalled {
		fmt.Fprintln(w)
		fmt.Fprintln(w, ioutil.Warn("Recommendation"))
		fmt.Fprintln(w, "pnpm is not installed. Enable corepack to get it, then optimize storage.")
		fmt.Fprintf(w, "Run: %s\n", ioutil.Bold("corepack enable && driftr node optimize"))
		return
	}

	if !info.globalVirtualStore {
		fmt.Fprintln(w)
		fmt.Fprintln(w, ioutil.Warn("Recommendation"))
		fmt.Fprintln(w, "Enable pnpm shared storage to reduce duplicated dependency data.")
		fmt.Fprintf(w, "Run: %s\n", ioutil.Bold("driftr node optimize"))
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, ioutil.Success("Shared dependency storage is enabled."))
}

func orNotFound(s string) string {
	if s == "" {
		return ioutil.Red("not found")
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return ioutil.Dim("unknown")
	}
	return s
}

func availability(ok bool) string {
	if ok {
		return ioutil.Green("available")
	}
	return ioutil.Red("not found")
}

func enabledDisabled(on bool) string {
	if on {
		return ioutil.Green("enabled")
	}
	return ioutil.Yellow("disabled")
}
