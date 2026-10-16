package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cmdShimTarget reads the name.cmd that a global npm install wrote into
// binDir on Windows and returns the file it launches, and whether that file
// is a script node runs. The .cmd itself is never run: cmd.exe would parse
// the user's arguments a second time, with its own quoting rules.
//
// npm's cmd-shim launches its target from a line that ends in %*:
//
//	"%_prog%"  "%dp0%\node_modules\typescript\bin\tsc" %*    a node script
//	"%dp0%\node_modules\pkg\bin\tool.exe"   %*                a native binary
//
// The .cmd files node ships, such as corepack.cmd, name node.exe directly
// and write %~dp0 instead of %dp0%.
func cmdShimTarget(binDir, name string) (target string, needsNode bool, err error) {
	path := filepath.Join(binDir, name+".cmd")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	content := string(data)
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "%*") {
			continue
		}
		quoted := quotedWords(line)
		for i := len(quoted) - 1; i >= 0; i-- {
			rel, ok := dp0Relative(quoted[i])
			if !ok || strings.EqualFold(rel, "node.exe") {
				continue
			}
			// shortcut: arguments between node and the script, which a
			// "#!/usr/bin/env node --flag" shebang puts there, are dropped.
			for _, prog := range quoted[:i] {
				progRel, isPath := dp0Relative(prog)
				switch {
				case isPath && strings.EqualFold(progRel, "node.exe"):
					needsNode = true
				case prog == "%_prog%":
					// cmd-shim sets _prog from the script's shebang.
					if !strings.Contains(content, `_prog=%dp0%\node.exe"`) {
						return "", false, fmt.Errorf("%s runs a program driftr cannot start on Windows", path)
					}
					needsNode = true
				}
			}
			return filepath.Join(binDir, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/"))), needsNode, nil
		}
	}
	return "", false, fmt.Errorf("%s does not have the layout npm writes", path)
}

// quotedWords returns the double-quoted strings in line, without the quotes.
func quotedWords(line string) []string {
	var words []string
	for {
		start := strings.IndexByte(line, '"')
		if start < 0 {
			return words
		}
		end := strings.IndexByte(line[start+1:], '"')
		if end < 0 {
			return words
		}
		words = append(words, line[start+1:start+1+end])
		line = line[start+1+end+1:]
	}
}

// dp0Relative strips the %dp0%\ or %~dp0\ prefix from a path in a .cmd file,
// which stands for the directory the .cmd is in.
func dp0Relative(path string) (string, bool) {
	for _, prefix := range []string{`%dp0%\`, `%~dp0\`} {
		if rel, ok := strings.CutPrefix(path, prefix); ok {
			return rel, true
		}
	}
	return "", false
}
