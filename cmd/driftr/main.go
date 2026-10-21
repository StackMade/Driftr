package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/stackmade/driftr/v2/internal/cli"
	"github.com/stackmade/driftr/v2/internal/ioutil"
	"github.com/stackmade/driftr/v2/internal/resolver"
	"github.com/stackmade/driftr/v2/internal/shim"
)

func main() {
	// Fast path: bypass Cobra entirely for shim invocations. Unix shim
	// scripts call "driftr shim <tool> [args...]"; a Windows shim is a copy
	// of driftr named after its tool, and "node shim x" passes "shim" to node.
	if tool, ok := shim.InvokedAs(); ok {
		runShim(tool, os.Args[1:])
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "shim" {
		runShim(os.Args[2], os.Args[3:])
		return
	}

	cli.Execute()
}

func runShim(tool string, args []string) {
	_ = cli.RefreshStaleShims()
	rb, err := resolver.ResolveBinaryFull(tool, "")
	if err != nil {
		rb, err = cli.HandleShimError(err, tool)
	}
	if err == nil {
		err = cli.ExecResolved(tool, rb, args)
	}
	if err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "driftr: %s\n", ioutil.Commands(err.Error()))
		os.Exit(1)
	}
}
