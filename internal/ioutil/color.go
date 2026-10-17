package ioutil

import (
	"os"
	"regexp"
	"sync"
)

var (
	ttyOnce sync.Once
	isTTY   bool
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// One switch for both streams: helpers do not know where their string
	// goes, and warnings go to stderr. Checking stdout alone wrote escape
	// codes into `driftr ... 2>log`.
	ttyOnce.Do(func() { isTTY = IsTerminal(os.Stdout) && IsTerminal(os.Stderr) && enableANSI() })
	return isTTY
}

func colorize(s, ansiCode string) string {
	return "\033[" + ansiCode + "m" + s + "\033[0m"
}

func Green(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "32")
}

func Bold(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "1")
}

func Dim(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "2")
}

func Yellow(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "33")
}

func Red(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "31")
}

func Cyan(s string) string {
	if !colorEnabled() {
		return s
	}
	return colorize(s, "36")
}

// Semantic helpers build a consistent vocabulary across commands. They compose
// the primitives above so callers express intent ("this is a success line")
// rather than a color. All degrade to plain text when color is disabled.

// Title formats a section heading (bold cyan).
func Title(s string) string { return Bold(Cyan(s)) }

// Success prefixes a green check mark.
func Success(s string) string { return Green("✓ ") + Commands(s) }

// Warn prefixes a yellow warning sign, matching Success/Failure which color
// only the marker and leave the message in the default weight.
func Warn(s string) string { return Yellow("⚠ ") + Commands(s) }

// Failure prefixes a red cross.
func Failure(s string) string { return Red("✗ ") + Commands(s) }

var backticked = regexp.MustCompile("`([^`\n]+)`")

// Commands colors every `backticked` span, the form messages use for the
// command to run next, and drops the backticks. Without color the text is
// returned as is, so logs and CI output keep the backticks.
func Commands(s string) string {
	if !colorEnabled() {
		return s
	}
	return backticked.ReplaceAllStringFunc(s, func(m string) string {
		return Bold(Cyan(m[1 : len(m)-1]))
	})
}

// Bullet prefixes a dim bullet for secondary/no-op lines.
func Bullet(s string) string { return Dim("• " + s) }

// Label dims a field label so values stand out next to it.
func Label(s string) string { return Dim(s) }
