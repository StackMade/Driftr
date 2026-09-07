package nodeenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBinDir prepends a fresh directory to PATH for the duration of the test and
// returns it, so scripts written there shadow any real binary of the same name.
func fakeBinDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// writeScript places an executable /bin/sh script named name into dir.
func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestExecRunner_RunSuccess(t *testing.T) {
	tests := []struct {
		name string
		body string
		args []string
		want string
	}{
		{
			name: "trims surrounding whitespace",
			body: `printf '  9.1.0 \n\n'`,
			want: "9.1.0",
		},
		{
			name: "passes arguments through in order",
			body: `echo "$@"`,
			args: []string{"config", "get", "store-dir"},
			want: "config get store-dir",
		},
		{
			name: "empty output is not an error",
			body: `exit 0`,
			want: "",
		},
		{
			name: "stderr on success is discarded",
			body: `echo noise >&2; echo /store/pnpm`,
			args: []string{"store", "path"},
			want: "/store/pnpm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakeBinDir(t)
			writeScript(t, dir, "driftr-fake-pnpm", tt.body)

			got, err := NewExecRunner().Run("driftr-fake-pnpm", tt.args...)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecRunner_RunFailure(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		args     []string
		contains []string
		absent   string
	}{
		{
			name:     "non-zero exit includes stderr and command line",
			body:     `echo "ERR_PNPM_NO_LOCKFILE" >&2; exit 3`,
			args:     []string{"install"},
			contains: []string{"driftr-fake-pnpm install", "exit status 3", "ERR_PNPM_NO_LOCKFILE"},
		},
		{
			name:     "non-zero exit with silent stderr still names the command",
			body:     `exit 1`,
			args:     []string{"config", "get", "store-dir"},
			contains: []string{"driftr-fake-pnpm config get store-dir", "exit status 1"},
			// Nothing was written to stderr, so the message must not end with a
			// dangling ": " from an empty stderr suffix.
			absent: "exit status 1: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakeBinDir(t)
			writeScript(t, dir, "driftr-fake-pnpm", tt.body)

			got, err := NewExecRunner().Run("driftr-fake-pnpm", tt.args...)
			if err == nil {
				t.Fatalf("Run() = %q, want error", got)
			}
			if got != "" {
				t.Errorf("Run() output on error = %q, want empty", got)
			}
			for _, want := range tt.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if tt.absent != "" && strings.Contains(err.Error(), tt.absent) {
				t.Errorf("error %q unexpectedly contains %q", err, tt.absent)
			}
		})
	}
}

func TestExecRunner_RunMissingBinary(t *testing.T) {
	fakeBinDir(t) // empty: nothing named below exists on PATH
	got, err := NewExecRunner().Run("driftr-fake-absent-tool", "--version")
	if err == nil {
		t.Fatalf("Run() = %q, want error for a binary not on PATH", got)
	}
	if got != "" {
		t.Errorf("Run() output = %q, want empty", got)
	}
	if !strings.Contains(err.Error(), "driftr-fake-absent-tool --version") {
		t.Errorf("error %q does not name the command", err)
	}
}

func TestExecRunner_LookPath(t *testing.T) {
	dir := fakeBinDir(t)
	writeScript(t, dir, "driftr-fake-corepack", "exit 0")

	got, err := NewExecRunner().LookPath("driftr-fake-corepack")
	if err != nil {
		t.Fatalf("LookPath() error = %v", err)
	}
	if want := filepath.Join(dir, "driftr-fake-corepack"); got != want {
		t.Errorf("LookPath() = %q, want %q", got, want)
	}

	if _, err := NewExecRunner().LookPath("driftr-fake-absent-tool"); err == nil {
		t.Error("expected LookPath to fail for a binary not on PATH")
	}
}

// A file on PATH that is not executable must not resolve — otherwise Installed()
// would report a tool that cannot be run.
func TestExecRunner_LookPathIgnoresNonExecutable(t *testing.T) {
	dir := fakeBinDir(t)
	if err := os.WriteFile(filepath.Join(dir, "driftr-fake-pnpm"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := NewExecRunner().LookPath("driftr-fake-pnpm"); err == nil {
		t.Errorf("LookPath() = %q, want error for a non-executable file", got)
	}
}
