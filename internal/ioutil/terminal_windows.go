package ioutil

import (
	"os"

	"golang.org/x/sys/windows"
)

// IsTerminal reports whether f is a console, not a pipe, file or NUL.
func IsTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

// enableANSI turns on escape-code processing for stdout and stderr. Consoles
// older than Windows 10 refuse, and then driftr prints without color.
func enableANSI() bool {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		h := windows.Handle(f.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) != nil {
			return false
		}
		if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
			return false
		}
	}
	return true
}
