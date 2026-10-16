package process

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exec replaces the process on success, so only its failure path is testable here.
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
		{name: "killed by a signal reports 128+signal", binary: "/bin/sh", args: []string{"-c", "kill -TERM $$"}, wantCode: 143},
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

func TestRunForwardsSignalsSentToDriftr(t *testing.T) {
	// The child exits 5 on SIGINT and 0 when left alone.
	child := []string{"-c", "trap 'exit 5' INT; sleep 1 & wait; exit 0"}
	for _, tt := range []struct {
		name       string
		foreground bool
		want       int
	}{
		// kill -INT <driftr pid> from a supervisor: pass it on.
		{"not the terminal foreground", false, 5},
		// Ctrl-C: the terminal already signaled the child, so no second copy.
		{"terminal foreground", true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			orig := inForeground
			inForeground = func() bool { return tt.foreground }
			t.Cleanup(func() { inForeground = orig })

			go func() {
				time.Sleep(300 * time.Millisecond)
				_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
			}()
			code, err := Run("/bin/sh", child)
			if err != nil {
				t.Fatal(err)
			}
			if code != tt.want {
				t.Errorf("exit code %d, want %d", code, tt.want)
			}
		})
	}
}
