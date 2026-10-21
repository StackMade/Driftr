package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Killing a shim with TerminateProcess, as child.kill() and Task Manager do,
// must take the node it runs down with it.
func TestRunChildDiesWithShim(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	shim := exec.Command(self)
	shim.Env = append(os.Environ(), "DRIFTR_TEST_SHIM=1", "DRIFTR_TEST_PIDFILE="+pidFile)
	if err := shim.Start(); err != nil {
		t.Fatal(err)
	}

	var pid int
	for deadline := time.Now().Add(10 * time.Second); pid == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = shim.Process.Kill()
			t.Fatal("the child never wrote its PID")
		}
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(string(data))
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)

	if err := shim.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = shim.Wait()
	if ev, _ := windows.WaitForSingleObject(child, 5000); ev != windows.WAIT_OBJECT_0 {
		_ = windows.TerminateProcess(child, 1)
		t.Fatal("the child outlived its killed shim")
	}
}
