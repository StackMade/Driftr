package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The .cmd files below are verbatim output: the first three from the
// cmd-shim that npm 10 bundles, the last from the Node.js 22 Windows zip.
const (
	cmdShimNode = `@ECHO off
GOTO start
:find_dp0
SET dp0=%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0

IF EXIST "%dp0%\node.exe" (
  SET "_prog=%dp0%\node.exe"
) ELSE (
  SET "_prog=node"
  SET PATHEXT=%PATHEXT:;.JS;=;%
)

endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & "%_prog%"  "%dp0%\node_modules\typescript\bin\tsc" %*
`
	cmdShimNative = `@ECHO off
GOTO start
:find_dp0
SET dp0=%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0
"%dp0%\node_modules\pkg\bin\native.exe"   %*
`
	cmdShimSh = `@ECHO off
GOTO start
:find_dp0
SET dp0=%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0

IF EXIST "%dp0%\sh.exe" (
  SET "_prog=%dp0%\sh.exe"
) ELSE (
  SET "_prog=sh"
  SET PATHEXT=%PATHEXT:;.JS;=;%
)

endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & "%_prog%"  "%dp0%\node_modules\pkg\bin\run.sh" %*
`
	corepackCmd = `@SETLOCAL
@IF EXIST "%~dp0\node.exe" (
  "%~dp0\node.exe"  "%~dp0\node_modules\corepack\dist\corepack.js" %*
) ELSE (
  @SET PATHEXT=%PATHEXT:;.JS;=;%
  node  "%~dp0\node_modules\corepack\dist\corepack.js" %*
)
`
)

func TestCmdShimTarget(t *testing.T) {
	tests := []struct {
		name, content, want string
		needsNode           bool
		wantErr             bool
	}{
		{name: "tsc", content: cmdShimNode, want: "node_modules/typescript/bin/tsc", needsNode: true},
		{name: "native", content: cmdShimNative, want: "node_modules/pkg/bin/native.exe"},
		{name: "corepack", content: strings.ReplaceAll(corepackCmd, "\n", "\r\n"), want: "node_modules/corepack/dist/corepack.js", needsNode: true},
		{name: "run", content: cmdShimSh, wantErr: true},
		{name: "odd", content: "@echo off\r\necho hi\r\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, tt.name+".cmd"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			target, needsNode, err := cmdShimTarget(binDir, tt.name)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("got %q, want an error", target)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(binDir, filepath.FromSlash(tt.want)); target != want || needsNode != tt.needsNode {
				t.Errorf("got %q, node %v; want %q, node %v", target, needsNode, want, tt.needsNode)
			}
		})
	}
}
