package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the child: DRIFTR_TEST_EXIT makes it exit at once.
func TestMain(m *testing.M) {
	if code := os.Getenv("DRIFTR_TEST_EXIT"); code != "" {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	// DRIFTR_TEST_PIDFILE makes it a long-running child that records its PID;
	// DRIFTR_TEST_SHIM makes it the shim that runs such a child.
	if pidFile := os.Getenv("DRIFTR_TEST_PIDFILE"); pidFile != "" && os.Getenv("DRIFTR_TEST_SHIM") == "" {
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if os.Getenv("DRIFTR_TEST_SHIM") != "" {
		os.Unsetenv("DRIFTR_TEST_SHIM")
		self, _ := os.Executable()
		code, _ := Run(self, nil)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

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
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{0, 3} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Setenv("DRIFTR_TEST_EXIT", strconv.Itoa(code))
			got, err := Run(self, nil)
			if err != nil || got != code {
				t.Errorf("Run() = %d, %v; want %d", got, err, code)
			}
		})
	}

	got, err := Run(filepath.Join(t.TempDir(), "nope"), nil)
	if err == nil || got != 1 {
		t.Errorf("missing binary: Run() = %d, %v; want 1 and an error", got, err)
	}
}
