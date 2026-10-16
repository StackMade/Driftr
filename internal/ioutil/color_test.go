package ioutil

import (
	"os"
	"strings"
	"sync"
	"testing"
)

// resetColorCache resets the memoized TTY state so tests get a clean slate.
func resetColorCache() {
	ttyOnce = sync.Once{}
	isTTY = false
}

func TestColorize_AppliesANSI(t *testing.T) {
	got := colorize("hello", "32")
	if got != "\033[32mhello\033[0m" {
		t.Errorf("colorize() = %q, want ANSI-wrapped string", got)
	}
}

func TestColorDisabled_NO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if Green("x") != "x" {
		t.Error("Green() should return plain text when NO_COLOR is set")
	}
	if Bold("x") != "x" {
		t.Error("Bold() should return plain text when NO_COLOR is set")
	}
	if Dim("x") != "x" {
		t.Error("Dim() should return plain text when NO_COLOR is set")
	}
	if Yellow("x") != "x" || Red("x") != "x" || Cyan("x") != "x" {
		t.Error("Yellow/Red/Cyan should return plain text when NO_COLOR is set")
	}
}

func TestSemanticHelpers_PlainWhenDisabled(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	for name, got := range map[string]string{
		"Title":   Title("Heading"),
		"Success": Success("done"),
		"Warn":    Warn("careful"),
		"Failure": Failure("nope"),
		"Bullet":  Bullet("skipped"),
		"Label":   Label("Key:"),
	} {
		if strings.Contains(got, "\033") {
			t.Errorf("%s should not emit ANSI when color disabled, got %q", name, got)
		}
	}

	// Prefixes/markers must survive regardless of color state.
	if !strings.HasPrefix(Success("done"), "✓ ") {
		t.Errorf("Success missing check mark: %q", Success("done"))
	}
	if !strings.HasPrefix(Warn("x"), "⚠ ") {
		t.Errorf("Warn missing sign: %q", Warn("x"))
	}
	if !strings.HasPrefix(Bullet("x"), "• ") {
		t.Errorf("Bullet missing marker: %q", Bullet("x"))
	}
}

func TestColorDisabled_NonTTY(t *testing.T) {
	// Reset cached TTY state so this test controls when it fires.
	resetColorCache()
	t.Cleanup(resetColorCache)

	// Guarantee non-TTY stdout via a pipe, regardless of test runner environment.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
		r.Close()
		w.Close()
	})

	// Ensure NO_COLOR is unset so we test TTY detection, not the NO_COLOR path.
	prev, prevOK := os.LookupEnv("NO_COLOR")
	os.Unsetenv("NO_COLOR")
	t.Cleanup(func() {
		if prevOK {
			os.Setenv("NO_COLOR", prev)
		}
	})

	if strings.Contains(Green("x"), "\033") {
		t.Error("Green() should not emit ANSI codes when stdout is not a TTY")
	}
	if strings.Contains(Bold("x"), "\033") {
		t.Error("Bold() should not emit ANSI codes when stdout is not a TTY")
	}
	if strings.Contains(Dim("x"), "\033") {
		t.Error("Dim() should not emit ANSI codes when stdout is not a TTY")
	}
}

// forceColor makes colorEnabled report true for the rest of the test.
func forceColor(t *testing.T) {
	t.Helper()
	resetColorCache()
	t.Cleanup(resetColorCache)
	ttyOnce.Do(func() {})
	isTTY = true
	prev, prevOK := os.LookupEnv("NO_COLOR")
	os.Unsetenv("NO_COLOR")
	t.Cleanup(func() {
		if prevOK {
			os.Setenv("NO_COLOR", prev)
		}
	})
}

func TestCommands(t *testing.T) {
	msg := "node 22 is not installed. Run `driftr install node@22` or `driftr list`"

	t.Setenv("NO_COLOR", "1")
	if got := Commands(msg); got != msg {
		t.Errorf("without color Commands() = %q, want the text unchanged", got)
	}

	forceColor(t)
	want := "node 22 is not installed. Run " + Bold(Cyan("driftr install node@22")) + " or " + Bold(Cyan("driftr list"))
	if got := Commands(msg); got != want {
		t.Errorf("Commands() = %q, want %q", got, want)
	}
	if got := Warn("Run `driftr doctor`"); !strings.Contains(got, Bold(Cyan("driftr doctor"))) {
		t.Errorf("Warn() does not highlight the command: %q", got)
	}
}

func TestLogo(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := Logo("v1.0.0", "tag"); got != "" {
		t.Errorf("Logo() without color = %q, want empty so plain help is unchanged", got)
	}

	forceColor(t)
	t.Setenv("COLORTERM", "")
	got := Logo("v1.0.0", "tag")
	for _, want := range []string{"38;5;155m██", "38;5;255m", "driftr", "v1.0.0", "tag"} {
		if !strings.Contains(got, want) {
			t.Errorf("Logo() missing %q:\n%s", want, got)
		}
	}
	t.Setenv("COLORTERM", "truecolor")
	if got := Logo("v1.0.0", "tag"); !strings.Contains(got, "38;2;184;255;61m██") {
		t.Errorf("Logo() with COLORTERM=truecolor does not use the brand green:\n%s", got)
	}
}
