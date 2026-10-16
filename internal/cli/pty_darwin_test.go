package cli

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	t.Cleanup(func() { master.Close() })
	var name [128]byte
	for _, req := range []struct {
		op  uintptr
		arg uintptr
	}{
		{syscall.TIOCPTYGRANT, 0},
		{syscall.TIOCPTYUNLK, 0},
		{syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))},
	} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), req.op, req.arg); errno != 0 {
			t.Skipf("pty ioctl: %v", errno)
		}
	}
	return master, openSlave(t, string(name[:bytes.IndexByte(name[:], 0)]))
}
