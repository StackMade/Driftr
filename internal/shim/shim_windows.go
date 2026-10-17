package shim

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/platform"
)

// On Windows a shim is driftr.exe itself under the tool's name, hard-linked
// where the file system allows and copied where it does not. A .cmd script
// would make cmd.exe parse every argument a second time and ask "Terminate
// batch job (Y/N)?" on Ctrl-C. driftr tells which tool it runs as from the
// name it was started under; see InvokedAs.

// manifestFile lists the package shims driftr wrote, one per line. A copy of
// driftr.exe does not say which tool it stands for, so this is how a file the
// user keeps in the bin dir is told apart from a shim.
const manifestFile = ".driftr-shims"

// shimFile returns the name of tool's shim in the bin dir.
func shimFile(tool string) string {
	return tool + ".exe"
}

// shimTool returns the tool that the bin dir file named file is a shim for,
// or "" when it cannot be one.
func shimTool(file string) string {
	ext := filepath.Ext(file)
	if !strings.EqualFold(ext, ".exe") {
		return ""
	}
	return strings.TrimSuffix(file, ext)
}

// shimOwner reports which files in binDir are shims driftr wrote: the package
// shims the last run recorded. The core tools' shims are always rewritten.
func shimOwner(binDir string) func(path, tool string) bool {
	recorded := map[string]bool{}
	if f, err := os.Open(filepath.Join(binDir, manifestFile)); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if name := strings.TrimSpace(sc.Text()); name != "" {
				recorded[strings.ToLower(name)] = true
			}
		}
		f.Close()
	}
	return func(_, tool string) bool {
		return recorded[strings.ToLower(tool)]
	}
}

// recordShims writes the manifest of package shims.
func recordShims(binDir string, tools []string) error {
	slices.Sort(tools)
	var b strings.Builder
	for _, t := range tools {
		b.WriteString(t + "\n")
	}
	return ioutil.WriteFileAtomic(filepath.Join(binDir, manifestFile), []byte(b.String()), 0o644)
}

// BakedPath returns "": a Windows shim is a copy of driftr, not a script
// naming it.
func BakedPath(string) string {
	return ""
}

func writeShim(binDir, tool, driftrBin string) error {
	path := filepath.Join(binDir, shimFile(tool))
	if sameFile(path, driftrBin) {
		return nil
	}
	sweepRetired(path)

	tmp, err := os.CreateTemp(binDir, "."+tool+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	os.Remove(tmpPath)
	if err := os.Link(driftrBin, tmpPath); err != nil {
		// Another volume, or a file system without hard links.
		if err := ioutil.CopyFile(driftrBin, tmpPath); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// A shim that is running cannot be replaced, but it can be renamed.
		if retireErr := retire(path); retireErr != nil {
			os.Remove(tmpPath)
			return errors.Join(err, retireErr)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}
	return nil
}

// removeShim deletes the shim at path, or moves it aside while it runs.
func removeShim(path string) error {
	if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
		return err
	}
	return retire(path)
}

// retire renames a shim that is in use out of the way. The next refresh
// deletes it once nothing runs it.
func retire(path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.old")
	if err != nil {
		return err
	}
	old := f.Name()
	f.Close()
	if err := os.Rename(path, old); err != nil {
		os.Remove(old)
		return fmt.Errorf("cannot move %s aside: %w", path, err)
	}
	return nil
}

// sweepRetired deletes the copies retire left for path, as far as nothing
// still runs them.
func sweepRetired(path string) {
	old, _ := filepath.Glob(path + ".*.old")
	for _, f := range old {
		os.Remove(f)
	}
}

// InvokedAs reports the tool driftr runs as when it was started through one
// of its shims: a copy of driftr.exe in the bin dir named after the tool.
func InvokedAs() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	tool := shimTool(filepath.Base(exe))
	if tool == "" || strings.EqualFold(tool, "driftr") {
		return "", false
	}
	binDir, err := platform.BinDir()
	if err != nil || !sameFile(filepath.Dir(exe), binDir) {
		return "", false
	}
	// Windows ignores case, so NODE and node both start node.exe.
	if _, ok := platform.LookupTool(strings.ToLower(tool)); ok {
		tool = strings.ToLower(tool)
	}
	return tool, true
}
