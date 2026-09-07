package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stackmade/driftr/internal/resolver"
)

// captureStderr runs fn and returns everything it printed to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// closedReader returns a reader that fails with something other than io.EOF.
func closedReader(t *testing.T) io.Reader {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestConfirm_Answers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty line defaults to no", "\n", false},
		{"y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"uppercase YES", "YES\n", true},
		{"padded yes", "  yes  \n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"unrelated word", "maybe\n", false},
		{"eof with no input", "", false},
		// A final line without a trailing newline still counts: ReadString
		// returns io.EOF alongside the text, and confirm uses the text.
		{"y without trailing newline", "y", true},
		{"n without trailing newline", "n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				got, err := confirm(strings.NewReader(tt.input), "Remove? [y/N] ")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("confirm(%q) = %v, want %v", tt.input, got, tt.want)
				}
			})
			if !strings.Contains(out, "Remove? [y/N] ") {
				t.Errorf("prompt not printed, got %q", out)
			}
		})
	}
}

func TestConfirm_ReadError(t *testing.T) {
	captureStdout(t, func() {
		ok, err := confirm(closedReader(t), "Remove? ")
		if err == nil {
			t.Fatal("expected an error from a closed reader")
		}
		if ok {
			t.Error("a failed read must not be treated as a yes")
		}
		if !strings.Contains(err.Error(), "cannot read confirmation") {
			t.Errorf("error = %v, want it to mention reading the confirmation", err)
		}
	})
}

func TestPromptInstall_Answers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Unlike confirm, this prompt is [Y/n] — an empty line means yes.
		{"empty line defaults to yes", "\n", true},
		{"y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"unrelated word", "maybe\n", false},
		{"eof with no input", "", false},
		// ReadString hands back the text along with io.EOF when the input
		// ends without a newline. A piped answer has to count, the same way
		// confirm counts it.
		{"y without trailing newline", "y", true},
		{"n without trailing newline", "n", false},
	}

	e := &resolver.NotInstalledError{Tool: "node", Version: "22.14.0"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			out := captureStderr(t, func() {
				got = promptInstall(strings.NewReader(tt.input), e)
			})
			if got != tt.want {
				t.Errorf("promptInstall(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if !strings.Contains(out, "node 22.14.0 is not installed") {
				t.Errorf("prompt = %q, want it to name the missing version", out)
			}
			if !strings.Contains(out, "[Y/n]") {
				t.Errorf("prompt = %q, want it to show the [Y/n] default", out)
			}
		})
	}
}

func TestPromptInstall_ReadError(t *testing.T) {
	e := &resolver.NotInstalledError{Tool: "node", Version: "22.14.0"}
	var got bool
	captureStderr(t, func() {
		got = promptInstall(closedReader(t), e)
	})
	if got {
		t.Error("a failed read must not be treated as a yes")
	}
}

func TestPromptInstall_ShowsContext(t *testing.T) {
	e := &resolver.NotInstalledError{Tool: "node", Version: "22.14.0", Context: "pinned in .driftr.toml"}
	out := captureStderr(t, func() {
		promptInstall(strings.NewReader("n\n"), e)
	})
	if !strings.Contains(out, "node 22.14.0 (pinned in .driftr.toml) is not installed") {
		t.Errorf("prompt = %q, want it to include the resolution context", out)
	}
}

func TestHandleShimError_PassesThroughUnrelatedErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := errors.New("permission denied")
	rb, got := HandleShimError(want, "node")
	if rb != nil {
		t.Errorf("resolved binary = %+v, want nil", rb)
	}
	if !errors.Is(got, want) {
		t.Errorf("error = %v, want the original error unchanged", got)
	}
}

func TestHandleShimError_NonInteractiveDoesNotPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Auto-install is off and stdin under `go test` is not a terminal, so the
	// missing version must be reported rather than silently installed.
	notInstalled := &resolver.NotInstalledError{Tool: "node", Version: "22.14.0"}
	rb, got := HandleShimError(notInstalled, "node")
	if rb != nil {
		t.Errorf("resolved binary = %+v, want nil", rb)
	}
	if !errors.Is(got, error(notInstalled)) {
		t.Errorf("error = %v, want the not-installed error passed back", got)
	}
}
