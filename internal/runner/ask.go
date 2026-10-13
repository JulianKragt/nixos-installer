package runner

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
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

// AskSecret is like Ask but does not echo the input. On a real terminal the
// answer is read with term.ReadPassword; on a pipe a regular line read is used
// as a fallback. The secret is never passed to endPrompt so it does not appear
// in the log file.
func AskSecret(ctx context.Context, prompt string) (string, error) {
	v := from(ctx)
	if v == nil {
		return "", io.EOF
	}
	l := v.l
	shown := l.beginPrompt(v.n, prompt)
	pw, err := l.readSecret(ctx)
	l.endPrompt(shown, "", err)
	return pw, err
}

// readSecret reads one line without echo. It uses term.ReadPassword when the
// input is a terminal file; otherwise it falls back to readLine.
func (l *Runner) readSecret(ctx context.Context) (string, error) {
	if l.inFile == nil {
		return l.readLine(ctx)
	}
	fd := int(l.inFile.Fd())
	state, _ := term.GetState(fd) // capture before ReadPassword sets raw mode
	pw, err := runInterruptible(ctx, func() ([]byte, error) {
		return term.ReadPassword(fd)
	})
	if err != nil {
		if state != nil {
			term.Restore(fd, state) // restore echo on cancellation
		}
		return "", err
	}
	// term.ReadPassword suppresses the newline; emit one so the cursor is on a
	// fresh line before endPrompt erases the prompt.
	l.mu.Lock()
	l.write("\n")
	l.mu.Unlock()
	return string(pw), nil
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
// when ctx is cancelled (Ctrl+C). The abandoned read goroutine ends with the process.
func (l *Runner) readLine(ctx context.Context) (string, error) {
	if l.in == nil {
		return "", io.EOF
	}
	line, err := runInterruptible(ctx, func() (string, error) {
		line, err := l.in.ReadString('\n')
		return strings.TrimSpace(line), err
	})
	if err == io.EOF && line != "" {
		err = nil // a last line without newline still counts as an answer
	}
	return line, err
}

// runInterruptible runs fn in a goroutine and returns its result, or returns
// ctx's error when ctx is cancelled. The abandoned goroutine ends with the process.
func runInterruptible[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
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
