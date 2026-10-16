package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stackmade/driftr/internal/resolver"
)

func openSlave(t *testing.T, name string) *os.File {
	t.Helper()
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open pty slave: %v", err)
	}
	t.Cleanup(func() { slave.Close() })
	return slave
}

func ttyStdin(t *testing.T, input string) {
	t.Helper()
	master, slave := openPTY(t)
	if _, err := master.WriteString(input); err != nil {
		t.Fatal(err)
	}
	// Unread echo makes closing the slave wait for a drain timeout on darwin.
	go io.Copy(io.Discard, master)
	orig := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = orig })
}

func TestPromptPinFormat_Terminal(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  pinFormat
	}{
		"package.json": {"2\n", formatPackageJSON},
		"toml":         {"1\n", formatTOML},
		"enter":        {"\n", formatTOML},
	} {
		input, want := tc.input, tc.want
		t.Run(name, func(t *testing.T) {
			ttyStdin(t, input)
			var stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetErr(&stderr)

			got, err := promptPinFormat(cmd)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("answer %q = %v, want %v", input, got, want)
			}
			if !strings.Contains(stderr.String(), "2) package.json") {
				t.Errorf("menu not shown: %q", stderr.String())
			}
		})
	}
}

func TestHintEval_OnlyWhenStdoutIsTerminal(t *testing.T) {
	_, slave := openPTY(t)
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)

	hintEval(cmd, "node@22", true)
	if stderr.Len() != 0 {
		t.Fatalf("hint printed while stdout is a pipe: %q", stderr.String())
	}

	orig := os.Stdout
	os.Stdout = slave
	hintEval(cmd, "node@22", true)
	os.Stdout = orig
	if want := `eval "$(driftr use --unset node@22)"`; !strings.Contains(stderr.String(), want) {
		t.Errorf("hint = %q, want %q", stderr.String(), want)
	}
}

func TestHandleShimError_DeclinedPromptInstallsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ttyStdin(t, "n\n")
	notInstalled := &resolver.NotInstalledError{Tool: "node", Version: "22.14.0", Context: ".nvmrc"}

	var rb *resolver.ResolvedBinary
	var err error
	out := captureStderr(t, func() { rb, err = HandleShimError(notInstalled, "node") })

	var exit *ExitError
	if rb != nil || !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("got %v, %v; want ExitError{1}", rb, err)
	}
	if !strings.Contains(out, "node 22.14.0 (.nvmrc) is not installed") || !strings.Contains(out, "Install now?") {
		t.Errorf("prompt = %q", out)
	}
	if strings.Contains(out, "Installing") {
		t.Errorf("installed after a no: %q", out)
	}
}
