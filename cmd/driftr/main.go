package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/stackmade/driftr/internal/cli"
	"github.com/stackmade/driftr/internal/resolver"
)

func main() {
	// Fast path: bypass Cobra entirely for shim invocations.
	// Shim scripts call "driftr shim <tool> [args...]".
	if len(os.Args) >= 3 && os.Args[1] == "shim" {
		tool := os.Args[2]
		_ = cli.RefreshStaleShims()
		rb, err := resolver.ResolveBinaryFull(tool, "")
		if err != nil {
			rb, err = cli.HandleShimError(err, tool)
		}
		if err == nil {
			err = cli.ExecResolved(tool, rb, os.Args[3:])
		}
		if err != nil {
			var exitErr *cli.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.Code)
			}
			fmt.Fprintf(os.Stderr, "driftr: %s\n", err)
			os.Exit(1)
		}
		return
	}

	cli.Execute()
}
