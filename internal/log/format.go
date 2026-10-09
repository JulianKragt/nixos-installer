package log

import (
	"fmt"
	"regexp"
	"strconv"
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
)

var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// format renders a message with its args. If the message contains a format
// verb ("%"), args are applied with fmt.Sprintf. Otherwise args are treated
// as alternating key/value pairs and appended as key=value.
func format(message string, args []any) string {
	if len(args) == 0 {
		return message
	}

	if strings.Contains(message, "%") {
		return fmt.Sprintf(message, args...)
	}

	var b strings.Builder
	b.WriteString(message)

	for i := 0; i < len(args); i += 2 {
		b.WriteByte(' ')
		b.WriteString(fmt.Sprint(args[i]))
		b.WriteByte('=')

		if i+1 >= len(args) {
			b.WriteString("<missing>")
			break
		}

		b.WriteString(formatValue(args[i+1]))
	}

	return b.String()
}

// formatValue quotes values that are empty or contain spaces, so they stay
// readable in key=value output.
func formatValue(v any) string {
	s := fmt.Sprint(v)
	if s == "" || strings.ContainsAny(s, " \t\"") {
		return strconv.Quote(s)
	}
	return s
}

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

// truncate cuts s to at most max runes, ending in an ellipsis if it was cut.
func truncate(s string, max int) string {
	if max < 1 {
		max = 1
	}
	if runeLen(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}
