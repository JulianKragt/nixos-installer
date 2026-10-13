package runner

import (
	"context"
	"io"
	"strings"
)

const maxLineBytes = 64 << 10

// outputWriter splits command output into lines for a task. It buffers a
// partial line, lets \r reset the current line (progress bars) and strips
// ANSI and control characters. Its state is guarded by the runner's mutex.
type outputWriter struct {
	runner *Runner
	n      *node // nil: root level, file only
	buf    []byte
	cr     bool
}

// Output returns a writer for command output belonging to ctx's task. It
// returns io.Discard when ctx has no runner.
func Output(ctx context.Context) io.Writer {
	v := from(ctx)
	if v == nil {
		return io.Discard
	}
	r := v.runner
	r.mu.Lock()
	defer r.mu.Unlock()
	if v.task == nil {
		if r.rootOut == nil {
			r.rootOut = &outputWriter{runner: r}
		}
		return r.rootOut
	}
	if v.task.out == nil {
		v.task.out = &outputWriter{runner: r, n: v.task}
	}
	return v.task.out
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.runner.mu.Lock()
	defer w.runner.mu.Unlock()
	for _, b := range p {
		switch {
		case b == '\r':
			w.cr = true
		case b == '\n':
			w.cr = false
			w.flushLocked()
		default:
			if w.cr {
				w.buf = w.buf[:0]
				w.cr = false
			}
			w.buf = append(w.buf, b)
			if len(w.buf) >= maxLineBytes {
				w.flushLocked()
			}
		}
	}
	w.runner.redrawIfNoTicker()
	return len(p), nil
}

// flushLocked emits the buffered line, if it has any content.
func (w *outputWriter) flushLocked() {
	line := strings.TrimRight(stripControl(string(w.buf)), " ")
	w.buf = w.buf[:0]
	w.cr = false
	if strings.TrimSpace(line) == "" {
		return
	}
	if w.n != nil {
		w.n.tail = append(w.n.tail, line)
		if len(w.n.tail) > tailKept {
			w.n.tail = w.n.tail[len(w.n.tail)-tailKept:]
		}
	}
	w.runner.logToFile("│", w.n, line)
}
