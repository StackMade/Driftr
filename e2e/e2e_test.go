// Package e2e runs the real driftr binary per script against test/fixture upstreams.
package e2e

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/stackmade/driftr/internal/ioutil"
	"github.com/stackmade/driftr/test/fixture"
)

var (
	driftrBin string
	// fixtureURL outlives every script: testscript runs them as parallel subtests.
	fixtureURL string
	// Copied before scripts start: copy-then-exec beside concurrent forks hits ETXTBSY.
	updatableBin, updatableSrc string
	coverDir                   string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run is separate from TestMain so deferred cleanup runs before os.Exit.
func run(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "driftr-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: create temp dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	driftrBin = filepath.Join(tmp, "driftr"+exe)

	// Not GOCOVERDIR: `go test -coverprofile` overwrites it in this process.
	var buildFlags []string
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
		// Absolute: ./... from the e2e directory would match only this package.
		buildFlags = append(buildFlags, "-cover", "-covermode=atomic", "-coverpkg=github.com/stackmade/driftr/...")
	}

	// Real binaries: shims embed os.Executable() of the driftr that wrote them.
	updatableBin = filepath.Join(tmp, "driftr-updatable"+exe)
	updatableSrc = filepath.Join(tmp, "driftr-updatable.orig")
	for _, out := range []string{driftrBin, updatableSrc} {
		flags := buildFlags
		if out == updatableSrc {
			// self-update refuses a dev build, so this one claims a release.
			flags = append(slices.Clone(buildFlags), "-ldflags", "-X github.com/stackmade/driftr/internal/cli.Version=1.0.0")
		}
		if err := buildDriftr(out, flags); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build %s: %v\n", filepath.Base(out), err)
			return 1
		}
	}

	// Windows cannot run the fixture's shell scripts, so its archives carry
	// a real program instead.
	var windowsExe []byte
	if runtime.GOOS == "windows" {
		fakebin := filepath.Join(tmp, "fakebin.exe")
		if err := build(fakebin, "../test/fakebin", nil); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build fakebin: %v\n", err)
			return 1
		}
		if windowsExe, err = os.ReadFile(fakebin); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: read fakebin: %v\n", err)
			return 1
		}
	}

	handler, err := fixture.Handler(windowsExe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build fixture: %v\n", err)
		return 1
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	fixtureURL = srv.URL

	code := m.Run()
	if coverDir != "" {
		if err := collectCoverage(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: collect coverage: %v\n", err)
			return 1
		}
	}
	return code
}

// Meta files share name and content across scripts, so overwriting one is harmless.
func collectCoverage() error {
	subs, err := filepath.Glob(filepath.Join(coverDir, "script-*"))
	if err != nil {
		return err
	}
	for _, sub := range subs {
		files, err := os.ReadDir(sub)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := os.Rename(filepath.Join(sub, f.Name()), filepath.Join(coverDir, f.Name())); err != nil {
				return err
			}
		}
		if err := os.Remove(sub); err != nil {
			return err
		}
	}
	return nil
}

func buildDriftr(out string, flags []string) error {
	return build(out, "../cmd/driftr", flags)
}

func build(out, pkg string, flags []string) error {
	args := append([]string{"build", "-o", out}, flags...)
	cmd := exec.Command("go", append(args, pkg)...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func TestDriftr(t *testing.T) {
	// self-update replaced the copy in an earlier -count run.
	if err := ioutil.CopyFile(updatableSrc, updatableBin); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		// [root]: permissions do not bind root (the Linux container runs as root).
		Condition: func(cond string) (bool, error) {
			if cond == "root" {
				return os.Geteuid() == 0, nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
		Setup: func(env *testscript.Env) error {
			home := filepath.Join(env.WorkDir, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				return err
			}
			env.Setenv("HOME", home)
			// Windows reads the home directory from USERPROFILE.
			env.Setenv("USERPROFILE", home)
			// On Windows, setup would write the shim dir into the runner's
			// own registry Path; unit tests cover that against a scratch key.
			if runtime.GOOS == "windows" {
				env.Setenv("DRIFTR_NO_MODIFY_PATH", "1")
			}

			// Shims first, so `node` in a script never reaches the runner's own.
			shimDir := filepath.Join(home, ".driftr", "bin")
			env.Setenv("PATH", shimDir+string(os.PathListSeparator)+
				filepath.Dir(driftrBin)+string(os.PathListSeparator)+env.Getenv("PATH"))

			env.Setenv("DRIFTR_NODE_MIRROR", fixtureURL)
			env.Setenv("DRIFTR_NPM_REGISTRY", fixtureURL+"/registry")
			env.Setenv("DRIFTR_BUN_RELEASES", fixtureURL+"/bun/releases")
			env.Setenv("DRIFTR_BUN_MIRROR", fixtureURL+"/bun/download")
			env.Setenv("DRIFTR_UPDATE_API", fixtureURL+"/update/api")
			env.Setenv("DRIFTR_UPDATE_MIRROR", fixtureURL+"/update/download")

			// self-update overwrites its own binary, so it gets a separate one.
			env.Setenv("DRIFTR_BIN", driftrBin)
			env.Setenv("DRIFTR_UPDATABLE_BIN", updatableBin)

			if coverDir != "" {
				// Per script: runtime temp meta-file names collide across processes.
				dir := filepath.Join(coverDir, filepath.Base(env.WorkDir))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				env.Setenv("GOCOVERDIR", dir)
			}

			env.Setenv("NO_COLOR", "1")

			env.Setenv("NODE_VERSION", fixture.NodeVersion)
			env.Setenv("NPM_VERSION", fixture.NpmVersion)
			env.Setenv("PNPM_VERSION", fixture.PnpmVersion)
			env.Setenv("YARN_VERSION", fixture.YarnVersion)
			env.Setenv("BUN_VERSION", fixture.BunVersion)
			env.Setenv("DRIFTR_UPDATE_VERSION", fixture.DriftrVersion)
			return nil
		},
	})
}
