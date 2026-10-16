package process

import "os"

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
