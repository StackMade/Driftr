package process

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Exec runs binary as a child and exits with its code: Windows cannot replace
// a running process image.
func Exec(binary string, args []string) error {
	code, err := Run(binary, args)
	if err != nil {
		return err
	}
	os.Exit(code)
	return nil
}

// inForeground is always true: Ctrl-C reaches every process on the console,
// the child included, so a forwarded copy would only duplicate it.
var inForeground = func() bool { return true }

// dieWithParent puts the child in a job object that kills it when driftr's
// last handle to the job closes, which happens however driftr ends. Tools stop
// node with TerminateProcess (child.kill(), Task Manager, an IDE's stop
// button), and that kills only the shim; without the job, node.exe would keep
// running and holding its port. The child's own children break away from the
// job, so a daemon node starts (pm2) outlives the shim, as on Unix. Best
// effort: a failure leaves the child as it was.
func dieWithParent(p *os.Process) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
	}
	// The job handle stays open on purpose: closing it is what kills the child.
}
