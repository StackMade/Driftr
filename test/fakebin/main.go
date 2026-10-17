// Command fakebin stands in for node.exe, bun.exe and driftr.exe in the
// fixture's Windows archives, which cannot hold the shell scripts the other
// platforms get. It acts on the name it runs under, the way those scripts do:
//
//	node -v               prints the fixture node version
//	node <script> ...     prints the script's "// driftr-fixture-prints: " line,
//	                      or else "v<version> ran: <script>"
//	bun ...               prints the fixture bun version
//	driftr* ...           prints "driftr version <version>"
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stackmade/driftr/v2/test/fixture"
)

// printsMarker starts the line of a fixture script that fakebin prints when
// node runs it, standing in for what the real script would print.
const printsMarker = "// driftr-fixture-prints: "

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(exe)), ".exe")
	args := os.Args[1:]

	switch {
	case name == "node":
		node(args)
	case name == "bun":
		fmt.Println(fixture.BunVersion)
	case strings.HasPrefix(name, "driftr"):
		fmt.Println("driftr version " + fixture.DriftrVersion)
	default:
		fmt.Fprintf(os.Stderr, "fakebin: unknown name %q\n", name)
		os.Exit(1)
	}
}

func node(args []string) {
	if len(args) == 0 || args[0] == "-v" || args[0] == "--version" {
		fmt.Println("v" + fixture.NodeVersion)
		return
	}
	if data, err := os.ReadFile(args[0]); err == nil {
		for line := range strings.Lines(string(data)) {
			if text, ok := strings.CutPrefix(strings.TrimSpace(line), printsMarker); ok {
				fmt.Println(text)
				return
			}
		}
	}
	fmt.Printf("v%s ran: %s\n", fixture.NodeVersion, args[0])
}
