//go:build darwin || linux

package process

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Exec replaces the current process with binary, keeping stdio and the exit
// code. It returns only on failure.
func Exec(binary string, args []string) error {
	argv := append([]string{binary}, args...)
	// The raw errno does not say which binary was missing, so name it.
	if err := syscall.Exec(binary, argv, os.Environ()); err != nil {
		return fmt.Errorf("failed to execute %s: %w", binary, err)
	}
	return nil
}

// inForeground reports whether this process group is the foreground of its
// controlling terminal. It is a variable so tests can fix the answer.
var inForeground = func() bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false // no controlling terminal: a signal came from kill or a supervisor
	}
	defer tty.Close()
	var pgrp int32 // pid_t on Linux and macOS
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), uintptr(syscall.TIOCGPGRP), uintptr(unsafe.Pointer(&pgrp)))
	return errno == 0 && int(pgrp) == syscall.Getpgrp()
}
