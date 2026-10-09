package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// Ask prints prompt and reads one line from in. On a terminal the live area is
// frozen while the question is open and the prompt and answer are erased again
// afterwards, so the question shows up inside the running task and leaves no
// stray lines behind. Log the answer yourself if it should stay visible.
// Without a runner in ctx the prompt is not shown and only the line is read.
func Ask(ctx context.Context, in *Input, prompt string) (string, error) {
	v := from(ctx)
	if v == nil {
		return in.ReadLine(ctx)
	}
	l := v.l

	pr := l.beginPrompt(v.n, prompt)
	line, err := in.ReadLine(ctx)
	l.endPrompt(pr, line, err)
	return line, err
}

// prompt is what endPrompt needs to erase the question again.
type prompt struct {
	plain string // question text without ANSI codes
	width int
	tty   bool
}

// beginPrompt freezes the live area and prints the question inside task n.
func (l *Runner) beginPrompt(n *node, text string) prompt {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drawLocked()
	l.suspended = true
	if l.tty && l.hidden {
		l.write("\033[?25h")
		l.hidden = false
	}
	if !l.tty {
		// Nothing is rendered until a task finishes; say where we are.
		l.write("▸ " + n.path() + "\n")
	}
	depth := 0
	if n != nil {
		depth = n.depth() + 1 // same indentation as log lines of this task
	}
	shown, plain := layoutPrompt(text, depth, l.tty)
	l.write(shown)
	width, _ := l.size()
	return prompt{plain: plain, width: width, tty: l.tty}
}

// endPrompt erases the question and answer again and thaws the live area.
func (l *Runner) endPrompt(p prompt, line string, readErr error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p.tty {
		if readErr != nil && line == "" {
			// no newline was echoed (EOF or cancelled)
			l.write("\n")
		}
		width := p.width
		if width <= 0 {
			width = 80
		}
		l.write(fmt.Sprintf("\033[%dA\r\033[J", promptLines(p.plain+line, width)))
	}
	l.suspended = false
	l.drawLocked()
}

// layoutPrompt indents prompt like a log line with a "?" symbol; continuation
// lines are aligned under the text. plain is the same text without ANSI codes.
func layoutPrompt(prompt string, depth int, color bool) (shown, plain string) {
	const sym = "?"
	pad := indent(depth)
	var s, p strings.Builder
	for i, seg := range strings.Split(prompt, "\n") {
		if i > 0 {
			s.WriteByte('\n')
			p.WriteByte('\n')
		}
		s.WriteString(pad)
		p.WriteString(pad)
		switch {
		case i > 0:
			s.WriteString("  ")
			p.WriteString("  ")
		case color:
			s.WriteString(yellow + sym + reset + " ")
			p.WriteString(sym + " ")
		default:
			s.WriteString(sym + " ")
			p.WriteString(sym + " ")
		}
		s.WriteString(seg)
		p.WriteString(seg)
	}
	return s.String(), p.String()
}

// Input reads the operator's answers, one line at a time.
type Input struct{ r *bufio.Reader }

func NewInput(r io.Reader) *Input { return &Input{bufio.NewReader(r)} }

// ReadLine reads one line, giving up with ctx's error when ctx is cancelled
// (Ctrl+C) even though the read itself cannot be interrupted. The abandoned
// read goroutine ends with the process.
func (in *Input) ReadLine(ctx context.Context) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := in.r.ReadString('\n')
		ch <- result{strings.TrimSpace(line), err}
	}()
	select {
	case r := <-ch:
		if r.err == io.EOF && r.line != "" {
			r.err = nil
		}
		return r.line, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// promptLines is the number of terminal rows that text plus the newline
// typed by the operator occupy.
func promptLines(text string, width int) int {
	n := 0
	for _, seg := range strings.Split(text, "\n") {
		w := len([]rune(seg))
		rows := (w + width - 1) / width
		if rows == 0 {
			rows = 1
		}
		n += rows
	}
	return n
}
