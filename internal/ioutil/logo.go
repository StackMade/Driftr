package ioutil

import (
	"os"
	"strings"
)

// The signet is a letter D in two strokes, as in brand/: a green stem
// (#b8ff3d) and a light bowl (#f5f5f2) with the counter left open.
var logoStem = []string{"██", "██", "██", "██", "██"}
var logoBowl = []string{" ▄▄▄▄▄   ", "      ▀▄ ", "       █ ", "      ▄▀ ", " ▀▀▀▀▀   "}

// brandColor picks the exact brand color where the terminal declares 24-bit
// support and the nearest 256-color entry elsewhere (Terminal.app, tmux
// without Tc), which would otherwise approximate badly.
func brandColor(s string, truecolor, ansi256 string) string {
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return colorize(s, "38;2;"+truecolor)
	}
	return colorize(s, "38;5;"+ansi256)
}

// Logo returns the colored signet with the wordmark beside it, or "" when
// color is off: piped help stays plain and unchanged.
func Logo(version, tagline string) string {
	if !colorEnabled() {
		return ""
	}
	side := []string{"", Bold("driftr"), Dim(version), tagline, ""}
	var b strings.Builder
	for i := range logoStem {
		b.WriteString("  ")
		b.WriteString(brandColor(logoStem[i], "184;255;61", "155"))
		b.WriteString(brandColor(logoBowl[i], "245;245;242", "255"))
		if side[i] != "" {
			b.WriteString("  " + side[i])
		}
		b.WriteByte('\n')
	}
	return b.String()
}
