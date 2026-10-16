//go:build darwin || linux

package ioutil

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal reports whether f is connected to a real terminal.
// Returns false for /dev/null, pipes, and redirected input.
// Uses ioctl to distinguish a real TTY from other char devices.
func IsTerminal(f *os.File) bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(), ioctlReadTermios, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}

// enableANSI reports whether the terminals take escape codes; POSIX ones do.
func enableANSI() bool { return true }
