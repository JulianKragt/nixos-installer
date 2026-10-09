package runner

import (
	"fmt"
	"io"
	"strings"
)

// fileSink writes every event, one line each, without ANSI codes:
//
//	15:12:03.501 │ Base install › Partitioning disk | Creating new GPT entries
type fileSink struct {
	w io.Writer
}

// fileEvent records an event for n in the log file. Caller must hold l.mu.
func (l *Runner) fileEvent(sym string, n *node, msg string) {
	if l.file == nil {
		return
	}
	ts := l.now().Format("15:04:05.000")
	path := n.path()
	for _, m := range strings.Split(stripANSILines(msg), "\n") {
		var line string
		if path == "" {
			line = fmt.Sprintf("%s %s | %s\n", ts, sym, m)
		} else {
			line = fmt.Sprintf("%s %s %s | %s\n", ts, sym, path, m)
		}
		if _, err := io.WriteString(l.file.w, line); err != nil && l.writeErr == nil {
			l.writeErr = err
		}
	}
}

// stripANSILines removes ANSI codes but keeps newlines.
func stripANSILines(s string) string {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = stripControl(p)
	}
	return strings.Join(parts, "\n")
}
