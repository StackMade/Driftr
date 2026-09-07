package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/config"
	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/internal/pathsetup"
	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/shim"
)

// versionedTools are tools that have independently installed versions.
var versionedTools = []string{"node", "pnpm", "yarn", "bun"}

// conflicting node version managers to detect on PATH.
var conflictingBinaries = []string{"fnm", "volta", "n"}

const shimExecPrefix = `exec "`

func newDoctorCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check your driftr installation for problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			binDir, err := platform.BinDir()
			if err != nil {
				return fmt.Errorf("cannot determine driftr bin directory: %w", err)
			}
			cfg, cfgErr := config.LoadGlobal()

			toolVersions := make(map[string][]string)
			issues := 0
			for _, tool := range versionedTools {
				versions, err := platform.ListToolVersions(tool)
				if err != nil {
					warn(fmt.Sprintf("Cannot list installed versions for %s: %s", tool, err))
					issues++
					continue
				}
				toolVersions[tool] = versions
			}

			issues += checkPath(binDir)
			issues += checkShimShadowing(binDir, fix)
			issues += checkShellRCPlacement(binDir, fix)
			issues += checkShims(binDir, fix)
			issues += checkShimBinaryPath(binDir, fix)
			issues += checkGlobalDefault(cfg, cfgErr)
			issues += checkDefaultsInstalled(cfg)
			issues += checkConflictingManagers(binDir)
			issues += checkInstalledVersions(toolVersions)
			issues += checkNeedsNode(toolVersions)

			fmt.Println()
			if issues == 0 {
				fmt.Println(ioutil.Success(ioutil.Bold("No issues found.")))
			} else {
				fmt.Println(ioutil.Warn(fmt.Sprintf("Found %d issue(s).", issues)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "automatically fix detected PATH and shim problems")
	return cmd
}

func pass(msg string) {
	fmt.Printf("  %s  %s\n", ioutil.Green("ok"), msg)
}

func warn(msg string) {
	fmt.Printf("  %s  %s\n", ioutil.Yellow("!!"), msg)
}

func checkPath(binDir string) int {
	if binDirOnPath(binDir) {
		pass(binDir + " is on PATH")
		return 0
	}
	warn(binDir + " is not on PATH — shims won't be found")
	return 1
}

// binDirOnPath reports whether binDir appears as a PATH entry.
func binDirOnPath(binDir string) bool {
	cleanBinDir := filepath.Clean(binDir)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(dir) == cleanBinDir {
			return true
		}
	}
	return false
}

// checkShimShadowing detects when a managed tool resolves to a binary that is
// not driftr's shim — i.e. another install (Homebrew, Volta, asdf, …) sits
// earlier in PATH and wins. driftr can pin and resolve correctly, but the
// shadowing binary is what the shell actually runs, so the pin appears ignored.
//
// Only runs when the shim dir is on PATH at all; otherwise checkPath already
// reported the root problem and this would be redundant noise.
// shadowedShim records a managed tool whose resolved binary is not the shim.
type shadowedShim struct {
	tool     string
	resolved string
}

// shadowedShims returns the managed tools that resolve to a non-shim binary
// because another install sits earlier in PATH. Empty when the shim dir is not
// on PATH (checkPath owns that case) or nothing is shadowed.
func shadowedShims(binDir string) []shadowedShim {
	if !binDirOnPath(binDir) {
		return nil
	}
	shimDir := filepath.Clean(binDir)
	var shadows []shadowedShim
	for _, tool := range versionedTools {
		resolved, err := exec.LookPath(tool)
		if err != nil {
			continue // tool not on PATH anywhere — nothing to shadow
		}
		if filepath.Dir(filepath.Clean(resolved)) == shimDir {
			continue // driftr shim wins
		}
		shadows = append(shadows, shadowedShim{tool: tool, resolved: resolved})
	}
	return shadows
}

// checkShimShadowing detects when a managed tool resolves to a binary that is
// not driftr's shim — i.e. another install (Homebrew, Volta, asdf, …) sits
// earlier in PATH and wins. driftr can pin and resolve correctly, but the
// shadowing binary is what the shell actually runs, so the pin appears ignored.
//
// With fix=true it repairs the situation by appending a PATH precedence guard
// to the interactive rc file (see pathsetup.ApplyGuard).
func checkShimShadowing(binDir string, fix bool) int {
	shadows := shadowedShims(binDir)
	if len(shadows) == 0 {
		return 0
	}
	for _, s := range shadows {
		warn(fmt.Sprintf("%s resolves to %s, not the driftr shim — pins/defaults won't apply", s.tool, s.resolved))
	}

	if !fix {
		warn("  run `driftr doctor --fix` to add a PATH precedence guard, or remove the conflicting tool")
		return 1
	}

	wrote, file, err := pathsetup.ApplyGuard(pathsetup.DetectShell(), binDir)
	if err != nil {
		warn("  fix failed: " + err.Error())
		return 1
	}
	if wrote {
		pass(fmt.Sprintf("  fixed: added PATH precedence guard to %s (open a new shell to apply)", file))
		return 0
	}
	// Guard already present but the tool is still shadowed: PATH in this session
	// is stale (rc not re-sourced) or another tool prepends after the guard.
	warn("  precedence guard already present — restart your shell, or ensure " + binDir + " is exported last")
	return 1
}

// checkShellRCPlacement verifies that binDir is exported from a shell rc file
// that every invocation of the shell sources (e.g. .zshenv for zsh). Entries
// in interactive-only files (.zshrc, .bashrc) are flagged as stale because
// non-interactive shells, scripts, cron, and IDE subprocesses won't see them.
func checkShellRCPlacement(binDir string, fix bool) int {
	r, err := pathsetup.Detect(binDir)
	if err != nil {
		warn(fmt.Sprintf("Cannot inspect shell rc files: %s", err))
		return 1
	}

	if !r.NeedsFix() {
		pass(fmt.Sprintf("PATH configured in %s (universal shell coverage)", r.Target))
		return 0
	}

	switch {
	case len(r.StaleFiles) > 0:
		warn(fmt.Sprintf("PATH is only in interactive rc file(s): %s",
			strings.Join(r.StaleFiles, ", ")))
		warn(fmt.Sprintf("  scripts, cron, and non-interactive shells won't find driftr — target %s", r.Target))
	default:
		warn(fmt.Sprintf("PATH is not configured in any shell rc file — target %s", r.Target))
	}

	if !fix {
		warn("  run `driftr doctor --fix` to repair")
		return 1
	}

	wrote, file, applyErr := pathsetup.Apply(r)
	if applyErr != nil {
		warn(fmt.Sprintf("  fix failed: %s", applyErr))
		return 1
	}
	if wrote {
		pass(fmt.Sprintf("  fixed: added PATH export to %s (open a new shell to use it)", file))
		return 0
	}
	warn("  nothing to fix")
	return 1
}

// brokenShims returns the tools whose shim is missing or not executable.
func brokenShims(binDir string) []string {
	var broken []string
	for _, tool := range shim.ShimTools() {
		info, err := os.Stat(filepath.Join(binDir, tool))
		if err != nil || info.Mode()&0o111 == 0 {
			broken = append(broken, tool)
		}
	}
	return broken
}

func checkShims(binDir string, fix bool) int {
	broken := brokenShims(binDir)
	if len(broken) == 0 {
		pass(fmt.Sprintf("All %d shims installed", len(shim.ShimTools())))
		return 0
	}

	for _, tool := range broken {
		warn(fmt.Sprintf("Shim broken or missing: %s", tool))
	}

	if !fix {
		warn("  run `driftr doctor --fix` to regenerate the shims")
		return len(broken)
	}

	// GenerateShims rewrites every shim, so one call repairs the whole set.
	if err := shim.GenerateShims(); err != nil {
		warn("  fix failed: " + err.Error())
		return len(broken)
	}

	if still := brokenShims(binDir); len(still) > 0 {
		warn(fmt.Sprintf("  fix incomplete: %s still broken", strings.Join(still, ", ")))
		return len(still)
	}

	pass(fmt.Sprintf("  fixed: regenerated %d shim(s)", len(broken)))
	return 0
}

func checkShimBinaryPath(binDir string, fix bool) int {
	currentBin, err := os.Executable()
	if err != nil {
		return 0
	}
	// Keep the unresolved path on error — overwriting with "" would make the
	// comparison below always mismatch and produce a false warning.
	if resolved, err := filepath.EvalSymlinks(currentBin); err == nil {
		currentBin = resolved
	}

	shimPath := filepath.Join(binDir, "node")
	data, err := os.ReadFile(shimPath)
	if err != nil {
		return 0 // already covered by checkShims
	}

	content := string(data)
	if idx := strings.Index(content, shimExecPrefix); idx >= 0 {
		rest := content[idx+len(shimExecPrefix):]
		if end := strings.Index(rest, "\""); end >= 0 {
			shimBin := rest[:end]
			resolved, err := filepath.EvalSymlinks(shimBin)
			if err != nil || resolved == "" {
				resolved = shimBin
			}
			if resolved != currentBin {
				warn(fmt.Sprintf("Shims point to %s but driftr is at %s", shimBin, currentBin))
				if !fix {
					warn("  run `driftr doctor --fix` to point them at the current binary")
					return 1
				}
				if err := shim.GenerateShims(); err != nil {
					warn("  fix failed: " + err.Error())
					return 1
				}
				pass("  fixed: shims now point to " + currentBin)
				return 0
			}
			pass("Shims point to current driftr binary")
			return 0
		}
	}

	return 0
}

func checkGlobalDefault(cfg *config.GlobalConfig, cfgErr error) int {
	if cfgErr != nil {
		warn(fmt.Sprintf("Cannot read global config: %s", cfgErr))
		return 1
	}

	if cfg.Default.GetTool("node") == "" {
		warn("No global default node version — run `driftr default node@<version>`")
		return 1
	}

	pass(fmt.Sprintf("Global default: node %s", cfg.Default.GetTool("node")))
	return 0
}

func checkDefaultsInstalled(cfg *config.GlobalConfig) int {
	if cfg == nil {
		return 0
	}

	issues := 0
	for _, tool := range versionedTools {
		ver := cfg.Default.GetTool(tool)
		if ver == "" {
			continue
		}
		binPath, err := platform.ToolBinary(tool, ver)
		if err != nil {
			continue
		}
		if _, err := os.Stat(binPath); err != nil {
			warn(fmt.Sprintf("Default %s %s is not installed — run `driftr install %s@%s`", tool, ver, tool, ver))
			issues++
		}
	}

	if issues == 0 {
		pass("All default versions are installed")
	}
	return issues
}

func checkConflictingManagers(binDir string) int {
	issues := 0

	// nvm is a shell function, not a binary — check $NVM_DIR instead.
	if nvmDir := os.Getenv("NVM_DIR"); nvmDir != "" {
		if _, err := os.Stat(filepath.Join(nvmDir, "nvm.sh")); err == nil {
			warn(fmt.Sprintf("nvm detected ($NVM_DIR=%s) — may conflict with driftr shims", nvmDir))
			issues++
		}
	}

	for _, manager := range conflictingBinaries {
		path, err := exec.LookPath(manager)
		if err != nil {
			continue
		}
		if filepath.Dir(path) == binDir {
			continue
		}
		warn(fmt.Sprintf("%s detected at %s — may conflict with driftr shims", manager, path))
		issues++
	}

	if issues == 0 {
		pass("No conflicting version managers found")
	}
	return issues
}

func checkInstalledVersions(toolVersions map[string][]string) int {
	for _, tool := range versionedTools {
		if versions := toolVersions[tool]; len(versions) > 0 {
			pass(fmt.Sprintf("%d %s version(s) installed", len(versions), tool))
		}
	}
	return 0
}

func checkNeedsNode(toolVersions map[string][]string) int {
	if len(toolVersions["node"]) > 0 {
		return 0
	}

	issues := 0
	for _, tool := range []string{"pnpm", "yarn"} {
		if len(toolVersions[tool]) > 0 {
			warn(fmt.Sprintf("%s is installed but no node versions found — %s requires Node.js to run", tool, tool))
			issues++
		}
	}
	return issues
}
