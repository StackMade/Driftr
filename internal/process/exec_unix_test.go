//go:build darwin || linux

package process

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestRunKilledBySignalReports128Plus(t *testing.T) {
	if code, err := Run("/bin/sh", []string{"-c", "kill -TERM $$"}); err != nil || code != 143 {
		t.Errorf("Run() = %d, %v; want 143", code, err)
	}
}

func TestRunForwardsSignalsSentToDriftr(t *testing.T) {
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
