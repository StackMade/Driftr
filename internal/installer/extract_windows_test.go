package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/platform"
	"github.com/stackmade/driftr/internal/testutil"
)

// The Windows archive keeps node.exe and npm at its top level; driftr unpacks
// it into bin/, where npm then puts global package bins too.
func TestExtract_WindowsZip(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	const ver = "99.0.0"
	prefix := nodePrefix(ver)
	archive := writeTestZip(t, map[string]string{
		prefix + "node.exe":                               "MZ node",
		prefix + "npm.cmd":                                "@ECHO off\r\n",
		prefix + "node_modules/npm/bin/npm-cli.js":        "// npm",
		prefix + "node_modules/npm/bin/npx-cli.js":        "// npx",
		prefix + "node_modules/corepack/dist/corepack.js": "// corepack",
	})

	if err := Extract(archive, ver, false, &installCleanup{}); err != nil {
		t.Fatal(err)
	}

	nodeBin, _ := platform.NodeBinary(ver)
	if got, err := os.ReadFile(nodeBin); err != nil || string(got) != "MZ node" {
		t.Errorf("node.exe = %q, %v", got, err)
	}
	for _, tool := range []string{"npm", "npx"} {
		bin, _ := platform.BundledNpmBinary(tool, ver)
		if _, err := os.Stat(bin); err != nil {
			t.Errorf("bundled %s missing: %v", tool, err)
		}
	}
	dir, _ := platform.NodeVersionDir(ver)
	if left := tmpLeftovers(t, filepath.Dir(dir)); len(left) > 0 {
		t.Errorf("work dirs left behind: %v", left)
	}
}

func TestExtract_WindowsZipWrongLayout(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	const ver = "99.0.0"
	archive := writeTestZip(t, map[string]string{"node-v1.0.0-win-x64/node.exe": "MZ"})

	err := Extract(archive, ver, false, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected layout") {
		t.Fatalf("got %v, want an unexpected-layout error", err)
	}
	dir, _ := platform.NodeVersionDir(ver)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("version dir exists after a failed extract: %v", err)
	}
}
