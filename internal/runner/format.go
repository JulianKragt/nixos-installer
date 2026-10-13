package runner

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	reset  = "\033[0m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	dim    = "\033[2m"

	hideCursor = "\033[?25l"
	showCursor = "\033[?25h"
	clearDown  = "\r\033[J" // to column 0, then erase to the end of the screen
)

// cursorUp moves the cursor n rows up.
func cursorUp(n int) string { return fmt.Sprintf("\033[%dA", n) }

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func indent(depth int) string {
	return strings.Repeat("  ", depth)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// stripControl removes ANSI escape sequences and control characters.
// Tabs become spaces.
func stripControl(s string) string {
	s = ansiRE.ReplaceAllString(s, "")
	s = strings.ToValidUTF8(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		}
		return r
	}, s)
}

// oneLine turns arbitrary text into a single printable line.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "; ")
	return stripControl(s)
}

// fmtDuration renders 120ms, 1.2s, 12s and 2m03s.
func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		s := int(d.Seconds())
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// truncate cuts s to at most width runes, ending in an ellipsis if it was cut.
func truncate(s string, width int) string {
	if width < 1 {
		width = 1
	}
	if runeLen(s) <= width {
		return s
	}
	return string([]rune(s)[:width-1]) + "…"
}
