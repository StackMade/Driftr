//go:build !windows

package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/internal/ioutil"
)

// shimFile returns the name of tool's shim in the bin dir.
func shimFile(tool string) string {
	return tool
}

// shimTool returns the tool that the bin dir file named file is a shim for.
func shimTool(file string) string {
	return file
}

// shimOwner reports which files in binDir are shims driftr wrote, so that
// nothing else there is overwritten or removed. On Unix a shim says so itself.
func shimOwner(string) func(path, tool string) bool {
	return isShimFile
}

// removeShim deletes the shim at path.
func removeShim(path string) error {
	return os.Remove(path)
}

// InvokedAs reports the tool driftr runs as when started through a shim that
// is a copy of it. Unix shims are scripts that run `driftr shim <tool>`
// instead, so there is none.
func InvokedAs() (string, bool) {
	return "", false
}

// recordShims is a no-op on Unix, where every shim names its tool.
func recordShims(string, []string) error {
	return nil
}

// BakedPath returns the driftr path a shim execs, or "".
func BakedPath(shimPath string) string {
	data, err := os.ReadFile(shimPath)
	if err != nil {
		return ""
	}
	path, _, _ := parseShim(string(data))
	return path
}

// parseShim returns the driftr path and tool of a shim writeShim made. Shims
// from older versions double-quote the path instead; they parse too, so they
// are still recognized as ours and refreshed.
func parseShim(content string) (driftrBin, tool string, ok bool) {
	line, ok := strings.CutPrefix(content, "#!/bin/sh\nexec ")
	if !ok || strings.Count(line, "\n") != 1 {
		return "", "", false
	}
	line, ok = strings.CutSuffix(line, ` "$@"`+"\n")
	if !ok {
		return "", "", false
	}
	i := strings.LastIndex(line, " shim ")
	if i < 2 {
		return "", "", false
	}
	quoted, tool := line[:i], line[i+len(" shim "):]
	inner := quoted[1 : len(quoted)-1]
	switch {
	case quoted[0] == '\'' && quoted[len(quoted)-1] == '\'':
		return strings.ReplaceAll(inner, `'\''`, "'"), tool, true
	case quoted[0] == '"' && quoted[len(quoted)-1] == '"':
		return inner, tool, true
	}
	return "", "", false
}

// isShimFile reports whether path holds the shim writeShim makes for tool.
// The size check keeps it from reading a binary such as driftr itself.
func isShimFile(path, tool string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && isShimFor(string(data), tool)
}

// isShimFor matches what writeShim produces for tool, with any driftr path:
// the path changes when driftr moves, and an old shim is still ours.
func isShimFor(content, tool string) bool {
	_, t, ok := parseShim(content)
	return ok && t == tool
}

func writeShim(binDir, tool, driftrBin string) error {
	// Single quotes keep $, `, " and \ in the path literal; a ' inside ends
	// the quote, adds an escaped ', and reopens it.
	quoted := "'" + strings.ReplaceAll(driftrBin, "'", `'\''`) + "'"
	content := fmt.Sprintf(`#!/bin/sh
exec %s shim %s "$@"
`, quoted, tool)
	return ioutil.WriteFileAtomic(filepath.Join(binDir, tool), []byte(content), 0o755)
}
