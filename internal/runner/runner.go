// Package runner keeps a tree of tasks in memory and renders it as a live area
// on the terminal. The current task travels in a context.Context, so nested
// and parallel code runs under the correct parent without passing a *Runner
// to every function.
package runner

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Level determines the visibility/severity of a message.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Options configures a Runner.
type Options struct {
	// Out receives the rendered tree. A terminal gets a live area with
	// colors; anything else only gets finished blocks, without escapes.
	Out io.Writer
	// File, if set, receives every event at debug level, including all
	// command output, without ANSI codes.
	File io.Writer
	// Verbose shows debug messages and debug tasks on Out.
	Verbose bool
}

const tickInterval = 80 * time.Millisecond

// Runner owns the task tree of one run.
type Runner struct {
	mu      sync.Mutex
	out     io.Writer
	file    *fileSink
	verbose bool
	now     func() time.Time

	tty  bool
	size func() (width, height int)

	roots     []*node
	orphans   []*node
	committed int // roots[:committed] are flushed above the live area
	pending   strings.Builder
	liveLines int
	frame     int
	hidden    bool // cursor hidden
	closed    bool
	suspended bool // an Ask is waiting for input; the live area is frozen
	writeErr  error
	rootOut   *outputWriter

	stop chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// New creates a Runner. Close must be called before the process exits.
func New(opts Options) *Runner {
	var tty bool
	var size func() (int, int)
	if f, ok := opts.Out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		tty = true
		size = func() (int, int) {
			w, h, err := term.GetSize(int(f.Fd()))
			if err != nil {
				return 80, 24
			}
			return w, h
		}
	}

	l := newRunner(opts, tty, size)
	if tty {
		l.wg.Add(1)
		go l.tick()
	}
	return l
}

func newRunner(opts Options, tty bool, size func() (int, int)) *Runner {
	l := &Runner{
		out:     opts.Out,
		verbose: opts.Verbose,
		now:     time.Now,
		tty:     tty,
		size:    size,
		stop:    make(chan struct{}),
	}
	if l.out == nil {
		l.out = io.Discard
	}
	if l.size == nil {
		l.size = func() (int, int) { return 0, 0 }
	}
	if opts.File != nil {
		l.file = &fileSink{w: opts.File}
	}
	return l
}

func (l *Runner) tick() {
	defer l.wg.Done()
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.draw()
		case <-l.stop:
			return
		}
	}
}

// Close stops the ticker, ends tasks that are still running, draws a final
// time and restores the cursor. It must run before os.Exit.
func (l *Runner) Close() error {
	l.once.Do(func() {
		close(l.stop)
		l.wg.Wait()

		l.mu.Lock()
		defer l.mu.Unlock()
		for _, r := range l.roots[l.committed:] {
			if r.kind == kindTask {
				l.endLocked(r, errClosed)
			}
		}
		for _, n := range l.orphans {
			if n.kind == kindTask {
				l.endLocked(n, errClosed)
			}
		}
		l.closed = true
		l.drawLocked()
		if l.hidden {
			l.write("\033[?25h")
			l.hidden = false
		}
	})
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writeErr
}

// write sends s to Out. Caller must hold l.mu.
func (l *Runner) write(s string) {
	if _, err := io.WriteString(l.out, s); err != nil && l.writeErr == nil {
		l.writeErr = err
	}
}

// afterEvent redraws immediately when no ticker is going to do it.
// Caller must hold l.mu.
func (l *Runner) afterEvent() {
	if !l.tty || l.closed {
		l.drawLocked()
	}
}

type ctxKey struct{}

type ctxVal struct {
	l *Runner
	n *node
}

// NewContext returns a context that carries l. Without a logger in the
// context, all calls in this package are no-ops.
func NewContext(ctx context.Context, l *Runner) context.Context {
	return context.WithValue(ctx, ctxKey{}, &ctxVal{l: l})
}

func from(ctx context.Context) *ctxVal {
	v, _ := ctx.Value(ctxKey{}).(*ctxVal)
	if v == nil || v.l == nil {
		return nil
	}
	return v
}

// Debug logs a debug message into ctx's task. Shown only when verbose.
func Debug(ctx context.Context, msg string, args ...any) { logAt(ctx, LevelDebug, msg, args) }

// Info logs an informational message into ctx's task.
func Info(ctx context.Context, msg string, args ...any) { logAt(ctx, LevelInfo, msg, args) }

// Warn logs a warning into ctx's task.
func Warn(ctx context.Context, msg string, args ...any) { logAt(ctx, LevelWarn, msg, args) }

// Error logs an error into ctx's task.
func Error(ctx context.Context, msg string, args ...any) { logAt(ctx, LevelError, msg, args) }

func logAt(ctx context.Context, level Level, msg string, args []any) {
	v := from(ctx)
	if v == nil {
		return
	}
	v.l.mu.Lock()
	defer v.l.mu.Unlock()
	v.l.logLocked(v.n, level, oneLine(format(msg, args)))
	v.l.afterEvent()
}
