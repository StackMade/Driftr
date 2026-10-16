package ioutil

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// signet is logo.png's D sampled at 16x16: g the green stem, l the bowl,
// . background. Two pixel rows become one line of half blocks.
var signet = []string{
	".ggglllllll.....",
	"gggglllllllll...",
	"ggggllllllllll..",
	"gggglllllllllll.",
	"gggg......lllll.",
	"gggg.......lllll",
	"gggg........llll",
	"gggg........llll",
	"gggg........llll",
	"gggg........llll",
	"gggg.......lllll",
	"gggg......lllll.",
	"gggglllllllllll.",
	"ggggllllllllll..",
	"gggglllllllll...",
	".ggglllllll.....",
}

// wordmark is "driftr" in the ANSI Shadow figlet font: █ is the letter,
// the box-drawing characters its drop shadow.
var wordmark = []string{
	"██████╗ ██████╗ ██╗███████╗████████╗██████╗ ",
	"██╔══██╗██╔══██╗██║██╔════╝╚══██╔══╝██╔══██╗",
	"██║  ██║██████╔╝██║█████╗     ██║   ██████╔╝",
	"██║  ██║██╔══██╗██║██╔══╝     ██║   ██╔══██╗",
	"██████╔╝██║  ██║██║██║        ██║   ██║  ██║",
	"╚═════╝ ╚═╝  ╚═╝╚═╝╚═╝        ╚═╝   ╚═╝  ╚═╝",
}

type rgb [3]int

var (
	brandGreen  = rgb{184, 255, 61}  // #b8ff3d
	brandLight  = rgb{245, 245, 242} // #f5f5f2
	shadowDark  = rgb{70, 90, 60}    // drop shadow on a dark background
	shadowLight = rgb{92, 128, 30}   // drop shadow on a light background
)

// fg returns the SGR parameters for a foreground color: exact where the
// terminal declares 24-bit color, the nearest xterm-256 entry elsewhere
// (Terminal.app, tmux without Tc), which would otherwise approximate badly.
func fg(c rgb) string {
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return fmt.Sprintf("38;2;%d;%d;%d", c[0], c[1], c[2])
	}
	return "38;5;" + strconv.Itoa(xterm256(c))
}

// xterm256 returns the xterm-256 entry nearest to c: the 6x6x6 color cube
// (16-231) or the gray ramp (232-255). Rounding each channel on its own
// tints near-neutral colors, so a gradient would wander through mint and
// yellow on its way to the light end.
func xterm256(c rgb) int {
	levels := []int{0, 95, 135, 175, 215, 255}
	dist := func(r, g, b int) int {
		return (c[0]-r)*(c[0]-r) + (c[1]-g)*(c[1]-g) + (c[2]-b)*(c[2]-b)
	}
	best, bestDist := 16, 1<<30
	for i := 0; i < 216; i++ {
		if d := dist(levels[i/36], levels[i/6%6], levels[i%6]); d < bestDist {
			best, bestDist = 16+i, d
		}
	}
	for i := 0; i < 24; i++ {
		v := 8 + 10*i
		if d := dist(v, v, v); d < bestDist {
			best, bestDist = 232+i, d
		}
	}
	return best
}

func lerp(a, b rgb, t float64) rgb {
	var c rgb
	for i := range c {
		c[i] = a[i] + int(float64(b[i]-a[i])*t+0.5)
	}
	return c
}

// lightBackground reports whether the terminal says its background is light.
// COLORFGBG ("15;0", "0;15") is set by iTerm2, Konsole and rxvt; without it
// the background is assumed dark, as most terminals are.
func lightBackground() bool {
	v := os.Getenv("COLORFGBG")
	bg := v[strings.LastIndex(v, ";")+1:]
	return bg == "7" || bg == "15"
}

// signetLine renders pixel rows 2i and 2i+1 as one line of half blocks. The
// bowl uses the terminal's own foreground, so it is light on a dark
// background and dark on a light one, as the brand's bowl is.
func signetLine(i int) string {
	top, bot := signet[2*i], signet[2*i+1]
	code := func(p byte) string {
		if p == 'g' {
			return fg(brandGreen)
		}
		return "39"
	}
	var b strings.Builder
	for c := 0; c < len(top); c++ {
		t, u := top[c], bot[c]
		switch {
		case t == '.' && u == '.':
			b.WriteByte(' ')
		case t == u:
			b.WriteString(colorize("█", code(t)))
		case t == '.':
			b.WriteString(colorize("▄", code(u)))
		default: // u == '.'; signet never stacks g and l in one cell (see test)
			b.WriteString(colorize("▀", code(t)))
		}
	}
	return b.String()
}

// wordmarkLine colors one wordmark row: the letters run from the brand green
// to the light neutral, mirroring the signet's stem and bowl, over a dim
// shadow. On a light background the letters take the terminal's foreground
// instead, since the light end would vanish.
func wordmarkLine(i int) string {
	light := lightBackground()
	shadow := shadowDark
	if light {
		shadow = shadowLight
	}
	runes := []rune(wordmark[i])
	var b strings.Builder
	for x, r := range runes {
		switch {
		case r == ' ':
			b.WriteRune(' ')
		case r == '█' && light:
			b.WriteString(colorize("█", "1;39"))
		case r == '█':
			b.WriteString(colorize("█", fg(lerp(brandGreen, brandLight, float64(x)/float64(len(runes)-1)))))
		default:
			b.WriteString(colorize(string(r), fg(shadow)))
		}
	}
	return b.String()
}

// hyperlink wraps text in an OSC 8 link, which iTerm2, Ghostty, kitty,
// WezTerm and VS Code make clickable. Terminals without OSC 8 skip the
// sequence, and the text still shows the whole URL.
func hyperlink(url, text string) string {
	return "\033]8;;" + url + "\033\\" + text + "\033]8;;\033\\"
}

// Logo returns the signet beside the wordmark, with the version and tagline
// under it and the maker's address below, or "" when color is off: piped help
// stays plain and unchanged.
func Logo(version, tagline, url string) string {
	if !colorEnabled() {
		return ""
	}
	var b strings.Builder
	b.WriteByte('\n')
	for i := 0; i < len(signet)/2; i++ {
		b.WriteString("  " + signetLine(i) + "   ")
		switch {
		case i >= 1 && i <= len(wordmark):
			b.WriteString(wordmarkLine(i - 1))
		case i == len(wordmark)+1:
			b.WriteString(Dim(version+"  ·  ") + tagline)
		}
		b.WriteString("\n")
	}
	// Under the tagline, in the wordmark's column. The brand green is too
	// pale to read on a light background, so that case takes the darker one.
	green := brandGreen
	if lightBackground() {
		green = shadowLight
	}
	b.WriteString(strings.Repeat(" ", 2+len(signet[0])+3))
	b.WriteString(Dim("by StackMade  ·  ") + hyperlink(url, colorize(url, "4;"+fg(green))) + "\n")
	return b.String()
}
