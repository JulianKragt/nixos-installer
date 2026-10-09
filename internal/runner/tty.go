package runner

import (
	"fmt"
	"strings"
	"time"
)

const (
	minDurColumn = 40 // durations are aligned to at least this column
	tailShown    = 5  // output lines shown under a running task
)

// line is one rendered row before layout.
type line struct {
	depth int
	sym   string
	color string
	text  string
	dur   string
	dim   bool // whole line is dim (command output)
}

// draw redraws the live area. The ticker calls it every tickInterval.
func (l *Runner) draw() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drawLocked()
}

// drawLocked commits finished roots and writes them, plus (on a terminal)
// the live area, in a single Write.
func (l *Runner) drawLocked() {
	if l.suspended {
		return
	}
	l.commitLocked()

	if !l.tty {
		if l.pending.Len() > 0 {
			l.write(l.pending.String())
			l.pending.Reset()
		}
		return
	}

	live := l.liveRender()
	if l.pending.Len() == 0 && len(live) == 0 && l.liveLines == 0 {
		return
	}

	var b strings.Builder
	if !l.hidden {
		b.WriteString("\033[?25l")
		l.hidden = true
	}
	if l.liveLines > 0 {
		fmt.Fprintf(&b, "\033[%dA", l.liveLines)
	}
	b.WriteString("\r\033[J")
	b.WriteString(l.pending.String())
	l.pending.Reset()
	for _, s := range live {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	l.write(b.String())

	l.liveLines = len(live)
	l.frame++
}

// commitLocked moves finished roots into the pending output, in start order.
// A root is only committed once every root before it is.
func (l *Runner) commitLocked() {
	for l.committed < len(l.roots) && l.roots[l.committed].finished() {
		r := l.roots[l.committed]
		var lines []line
		l.collect(r, 0, l.now(), false, &lines)
		for _, s := range renderLines(lines, 0, l.tty) {
			l.pending.WriteString(s)
			l.pending.WriteByte('\n')
		}
		r.committed = true
		l.committed++
	}
}

// liveRender renders the roots that are not committed yet, cut to the
// terminal size. Caller must hold l.mu.
func (l *Runner) liveRender() []string {
	var lines []line
	now := l.now()
	for _, r := range l.roots[l.committed:] {
		l.collect(r, 0, now, true, &lines)
	}
	width, height := l.size()
	out := renderLines(lines, width, true)

	room := height - 1
	if height <= 0 || len(out) <= room {
		return out
	}
	if room < 3 {
		return out[len(out)-max(room, 1):]
	}
	newest := room - 2
	hidden := len(out) - 1 - newest
	marker := fmt.Sprintf("%s%s… %d lines%s", dim, indent(1), hidden, reset)
	return append([]string{out[0], marker}, out[len(out)-newest:]...)
}

// collect appends the rows for n and everything below it.
func (l *Runner) collect(n *node, depth int, now time.Time, live bool, out *[]line) {
	if n.kind == kindLog {
		sym, col := logStyle(n.level, depth == 0)
		*out = append(*out, line{depth: depth, sym: sym, color: col, text: n.title})
		return
	}

	ln := line{depth: depth, text: n.title}
	switch n.state {
	case stateRunning:
		ln.sym, ln.color = spinnerFrames[l.frame%len(spinnerFrames)], cyan
		if d := now.Sub(n.start); d >= time.Second {
			ln.dur = fmtDuration(d)
		}
	case stateOK:
		ln.sym, ln.color = "✓", green
		ln.dur = fmtDuration(n.end.Sub(n.start))
	case stateFailed:
		ln.sym, ln.color = "✗", red
		ln.text = failedTitle(n)
		ln.dur = fmtDuration(n.end.Sub(n.start))
	}
	*out = append(*out, ln)

	tail := n.tail
	switch {
	case n.state == stateFailed:
	case n.state == stateRunning && live:
		if len(tail) > tailShown {
			tail = tail[len(tail)-tailShown:]
		}
	default:
		tail = nil
	}
	for _, t := range tail {
		*out = append(*out, line{depth: depth, sym: "│", color: dim, text: t, dim: true})
	}

	for _, c := range n.children {
		l.collect(c, depth+1, now, live, out)
	}
}

func logStyle(level Level, root bool) (sym, color string) {
	switch level {
	case LevelWarn:
		return "⚠", yellow
	case LevelError:
		return "✗", red
	case LevelInfo:
		if root {
			return "✓", green
		}
		return "→", green
	}
	if root {
		return "•", blue
	}
	return "→", blue
}

// renderLines lays out rows: indent, symbol, text and a dim duration aligned
// to a common column. With width > 0 every row is at most width-1 columns,
// measured on plain text, so it never wraps. color adds ANSI codes.
func renderLines(lines []line, width int, color bool) []string {
	col, maxDur := minDurColumn, 0
	for _, ln := range lines {
		if ln.dur == "" {
			continue
		}
		col = max(col, ln.depth*2+2+runeLen(ln.text)+2)
		maxDur = max(maxDur, runeLen(ln.dur))
	}
	limit := width - 1
	if width > 0 {
		col = min(col, limit-maxDur)
	}

	paint := func(c, s string) string {
		if !color {
			return s
		}
		return c + s + reset
	}

	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		prefix := ln.depth*2 + 2
		text := ln.text
		if width > 0 {
			room := limit
			if ln.dur != "" {
				room = min(limit, col-2)
			}
			if prefix+runeLen(text) > room {
				text = truncate(text, room-prefix)
			}
		}
		used := prefix + runeLen(text)

		var b strings.Builder
		b.WriteString(indent(ln.depth))
		b.WriteString(paint(ln.color, ln.sym))
		b.WriteByte(' ')
		if ln.dim {
			b.WriteString(paint(dim, text))
		} else {
			b.WriteString(text)
		}
		if ln.dur != "" && (width <= 0 || used+2+runeLen(ln.dur) <= limit) {
			b.WriteString(strings.Repeat(" ", max(col-used, 2)))
			b.WriteString(paint(dim, ln.dur))
		}
		out = append(out, b.String())
	}
	return out
}
