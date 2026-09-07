// Package e2e runs the driftr binary against its own HOME and real archives
// on disk. Every upstream it would otherwise fetch over the network is served
// by test/fixture from an in-process HTTP server, so the suite needs neither
// Docker nor a network connection.
//
// Each script in testdata/script gets its own work directory and its own
// HOME, so installs land in $WORK/home/.driftr and never touch the developer's
// ~/.driftr. Run them with:
//
//	go test ./e2e
//	go test ./e2e -run 'TestDriftr/install_node'   # one script
//	go test ./e2e -v                               # per-command tracing
//
// Shell-specific behavior is not covered here: writing PATH into .zshenv,
// .bashrc or config.fish lives in test_path_e2e.sh, which starts a real login
// shell for each shell it tests.
package e2e

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/stackmade/driftr/test/fixture"
)

var (
	// driftrBin holds the path to the binary built once for the whole suite.
	driftrBin string
	// fixtureURL is the base URL of the fake upstream server. It outlives
	// every script: testscript runs them as parallel subtests, so a server
	// torn down when the parent test function returns would be gone before
	// they finish.
	fixtureURL string
	// coverDir, when set, is where the coverage-instrumented driftr binary
	// writes its raw profiles. See run.
	coverDir string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run builds driftr and hands control to the test suite. It is separate from
// TestMain so the deferred cleanup runs before os.Exit.
func run(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "driftr-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: create temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	driftrBin = filepath.Join(tmp, "driftr")

	// The e2e suite runs driftr as a subprocess, so `go test -cover` sees
	// nothing it executes. Setting DRIFTR_E2E_COVERDIR — ci.yml does — builds
	// the binary instrumented instead, and every driftr run drops a raw
	// profile in that directory for `go tool covdata textfmt` to convert
	// afterwards. Unset, which is the plain `go test ./e2e` case, nothing
	// changes. It is not GOCOVERDIR because `go test -coverprofile` overwrites
	// that variable in the test binary's environment with a temp directory of
	// its own.
	args := []string{"build", "-o", driftrBin}
	if dir := os.Getenv("DRIFTR_E2E_COVERDIR"); dir != "" {
		coverDir, err = filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: resolve DRIFTR_E2E_COVERDIR: %v\n", err)
			return 1
		}
		if err := os.MkdirAll(coverDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: create DRIFTR_E2E_COVERDIR: %v\n", err)
			return 1
		}
		// The pattern is absolute rather than ./... — the build runs from the
		// e2e directory, where ./... would match this package and nothing else.
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=github.com/stackmade/driftr/...")
	}
	args = append(args, "../cmd/driftr")

	// The shims driftr generates embed the absolute path of the binary that
	// created them (os.Executable), so this has to be a genuine binary rather
	// than a testscript-registered command backed by the test process.
	build := exec.Command("go", args...)
	build.Stderr = os.Stderr
	build.Stdout = os.Stdout
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build driftr: %v\n", err)
		return 1
	}

	handler, err := fixture.Handler()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build fixture: %v\n", err)
		return 1
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	fixtureURL = srv.URL

	return m.Run()
}

func TestDriftr(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		Setup: func(env *testscript.Env) error {
			home := filepath.Join(env.WorkDir, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				return err
			}
			env.Setenv("HOME", home)

			// The shim directory goes on PATH ahead of everything else, so a
			// script that calls `node` gets driftr's shim rather than whatever
			// the runner has installed.
			shimDir := filepath.Join(home, ".driftr", "bin")
			env.Setenv("PATH", shimDir+string(os.PathListSeparator)+
				filepath.Dir(driftrBin)+string(os.PathListSeparator)+env.Getenv("PATH"))

			// Point every upstream at the fixture.
			env.Setenv("DRIFTR_NODE_MIRROR", fixtureURL)
			env.Setenv("DRIFTR_NPM_REGISTRY", fixtureURL+"/registry")
			env.Setenv("DRIFTR_BUN_RELEASES", fixtureURL+"/bun/releases")
			env.Setenv("DRIFTR_BUN_MIRROR", fixtureURL+"/bun/download")
			env.Setenv("DRIFTR_UPDATE_API", fixtureURL+"/update/api")
			env.Setenv("DRIFTR_UPDATE_MIRROR", fixtureURL+"/update/download")

			// self-update overwrites the binary it is running from, so the
			// script has to copy this one first. Every script shares it.
			env.Setenv("DRIFTR_BIN", driftrBin)

			if coverDir != "" {
				env.Setenv("GOCOVERDIR", coverDir)
			}

			// Plain output keeps the assertions readable.
			env.Setenv("NO_COLOR", "1")

			// Fixture versions, so a script asserts on $NODE_VERSION instead
			// of a literal that has to be updated in a dozen places.
			env.Setenv("NODE_VERSION", fixture.NodeVersion)
			env.Setenv("PNPM_VERSION", fixture.PnpmVersion)
			env.Setenv("YARN_VERSION", fixture.YarnVersion)
			env.Setenv("BUN_VERSION", fixture.BunVersion)
			env.Setenv("DRIFTR_UPDATE_VERSION", fixture.DriftrVersion)
			return nil
		},
	})
}
