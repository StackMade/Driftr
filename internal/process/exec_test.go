package process

import (
	"path/filepath"
	"strings"
	"testing"
)

// The success path of Exec replaces the process, so it can only be observed
// from the outside — the e2e suite does that by running a real shim. What is
// testable here is the failure path, which must name the binary it could not
// start rather than surfacing a bare errno.
func TestExecMissingBinaryNamesIt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-a-binary")

	err := Exec(missing, []string{"--version"})
	if err == nil {
		t.Fatal("Exec on a missing binary returned nil; the process is still here, so it did not exec")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error does not name the binary: %v", err)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		args     []string
		wantCode int
		wantErr  bool
	}{
		{name: "exit zero", binary: "/bin/sh", args: []string{"-c", "exit 0"}, wantCode: 0},
		{name: "exit code is passed through", binary: "/bin/sh", args: []string{"-c", "exit 3"}, wantCode: 3},
		{name: "missing binary is an error", binary: filepath.Join(t.TempDir(), "nope"), wantCode: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := Run(tt.binary, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if code != tt.wantCode {
				t.Errorf("Run() code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}
