package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Exec replaces the current process with the given binary.
// On Unix, this uses syscall.Exec for zero-overhead process replacement.
// Falls back to exec.Command on systems where Exec is not available.
func Exec(binary string, args []string) error {
	// Prepend binary as argv[0].
	argv := append([]string{binary}, args...)

	// Use syscall.Exec to replace the current process.
	// This preserves stdin/stdout/stderr and exit codes.
	// On success this never returns; on failure the raw errno says nothing
	// about which binary was missing, so name it.
	if err := syscall.Exec(binary, argv, os.Environ()); err != nil {
		return fmt.Errorf("failed to execute %s: %w", binary, err)
	}
	return nil
}

// Run executes a binary as a child process and returns the exit code.
// driftr outlives the child, so it does not die on a signal meant for it:
// Ctrl-C and Ctrl-\ reach the child from the terminal anyway, and SIGTERM or
// SIGHUP sent to driftr is passed on. A child killed by a signal reports
// 128+signal, as a shell would.
func Run(binary string, args []string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to execute %s: %w", binary, err)
	}

	// Notify, not Ignore: an ignored signal stays ignored in the child.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	defer func() {
		signal.Stop(sigs)
		close(sigs) // ends the forwarding goroutine; Stop guarantees no more sends
	}()
	go func() {
		for sig := range sigs {
			if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
				_ = cmd.Process.Signal(sig)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal()), nil
			}
			return exitErr.ExitCode(), nil
		}
		return 1, fmt.Errorf("failed to execute %s: %w", binary, err)
	}

	return 0, nil
}
