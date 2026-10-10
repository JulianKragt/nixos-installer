package runner

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Ask prints prompt and reads one line from the runner's input. On a terminal
// the live area is frozen while the question is open and the prompt and answer
// are erased again afterwards, so the question shows up inside the running task
// and leaves no stray lines behind. Log the answer yourself if it should stay
// visible. Without a runner in ctx there is nobody to ask: io.EOF.
func Ask(ctx context.Context, prompt string) (string, error) {
	v := from(ctx)
	if v == nil {
		return "", io.EOF
	}
	l := v.l

	shown := l.beginPrompt(v.n, prompt)
	line, err := l.readLine(ctx)
	l.endPrompt(shown, line, err)
	return line, err
}

// Choose shows a numbered list and returns the index the operator picks.
func Choose(ctx context.Context, header string, items []string) (int, error) {
	var b strings.Builder
	b.WriteString(header + "\n")
	for i, it := range items {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, it)
	}
	b.WriteString("Number: ")
	line, err := Ask(ctx, b.String())
	if err != nil {
		return 0, fmt.Errorf("read selection: %w", err)
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(items) {
		return 0, fmt.Errorf("invalid selection %q", line)
	}
	return n - 1, nil
}

// beginPrompt freezes the live area and prints the question inside task n. It
// returns the question as laid out, without ANSI codes, for endPrompt.
func (l *Runner) beginPrompt(n *node, text string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drawLocked()
	l.suspended = true
	if l.tty && l.cursorHidden {
		l.write("\033[?25h")
		l.cursorHidden = false
	}
	depth := 0
	if n != nil {
		depth = n.depth() + 1 // same indentation as log lines of this task
		if !l.tty {
			// Nothing is rendered until a task finishes; say where we are.
			l.write("▸ " + n.path() + "\n")
		}
	}
	plain := layoutPrompt(text, depth)
	if l.tty {
		pad := len(indent(depth))
		l.write(plain[:pad] + yellow + "?" + reset + plain[pad+1:])
	} else {
		l.write(plain)
	}
	return plain
}

// endPrompt erases the question and answer again and thaws the live area.
func (l *Runner) endPrompt(plain, line string, readErr error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tty {
		if readErr != nil && line == "" {
			// no newline was echoed (EOF or cancelled)
			l.write("\n")
		}
		width, _ := l.size()
		if width <= 0 {
			width = 80
		}
		l.write(fmt.Sprintf("\033[%dA\r\033[J", promptLines(plain+line, width)))
	}
	l.suspended = false
	l.drawLocked()
}

// layoutPrompt indents prompt like a log line with a "?" symbol; continuation
// lines are aligned under the text.
func layoutPrompt(prompt string, depth int) string {
	pad := indent(depth)
	return pad + "? " + strings.ReplaceAll(prompt, "\n", "\n"+pad+"  ")
}

// readLine reads one line from the runner's input, giving up with ctx's error
// when ctx is cancelled (Ctrl+C) even though the read itself cannot be
// interrupted. The abandoned read goroutine ends with the process.
func (l *Runner) readLine(ctx context.Context) (string, error) {
	if l.in == nil {
		return "", io.EOF
	}
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := l.in.ReadString('\n')
		ch <- result{strings.TrimSpace(line), err}
	}()
	select {
	case r := <-ch:
		if r.err == io.EOF && r.line != "" {
			r.err = nil // a last line without newline still counts as an answer
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
