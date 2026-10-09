package runner

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

type nodeKind int

const (
	kindTask nodeKind = iota
	kindLog
)

type taskState int

const (
	stateRunning taskState = iota
	stateOK
	stateFailed
)

const tailKept = 20

var (
	errParentEnded = errors.New("parent ended")
	errClosed      = errors.New("runner closed")
)

// node is a task or a log line. Command output lives in a task's tail.
type node struct {
	kind     nodeKind
	title    string
	level    Level
	parent   *node
	children []*node

	start, end time.Time
	state      taskState
	err        error

	tail []string // last tailKept output lines
	out  *outputWriter

	hidden    bool // not shown (debug task without verbose, or inside one)
	orphan    bool // started after its root was committed; printed on End
	committed bool // roots only
}

func (n *node) depth() int {
	d := 0
	for p := n.parent; p != nil; p = p.parent {
		d++
	}
	return d
}

func (n *node) root() *node {
	for n.parent != nil {
		n = n.parent
	}
	return n
}

// path joins the titles from the root to n with " › ".
func (n *node) path() string {
	if n == nil {
		return ""
	}
	var titles []string
	for p := n; p != nil; p = p.parent {
		titles = append(titles, p.title)
	}
	for i, j := 0, len(titles)-1; i < j; i, j = i+1, j-1 {
		titles[i], titles[j] = titles[j], titles[i]
	}
	return strings.Join(titles, " › ")
}

// finished reports whether n and everything below it is done.
func (n *node) finished() bool {
	return n.kind == kindLog || n.state != stateRunning
}

// Task is a handle on a running task. It only handles lifecycle; logging
// goes through the context returned by Start.
type Task struct {
	l *Runner
	n *node
}

// Start begins a task as a child of ctx's task, or as a root. The returned
// context carries the new task.
func Start(ctx context.Context, title string) (context.Context, *Task) {
	return start(ctx, title, LevelInfo)
}

// StartDebug is like Start but the task and everything under it is hidden
// unless the logger is verbose.
func StartDebug(ctx context.Context, title string) (context.Context, *Task) {
	return start(ctx, title, LevelDebug)
}

func start(ctx context.Context, title string, level Level) (context.Context, *Task) {
	v := from(ctx)
	if v == nil {
		return ctx, &Task{}
	}
	l := v.l
	l.mu.Lock()
	defer l.mu.Unlock()

	n := &node{
		kind:   kindTask,
		title:  oneLine(title),
		level:  level,
		parent: v.n,
		start:  l.now(),
	}
	n.hidden = (level == LevelDebug && !l.verbose) || (v.n != nil && v.n.hidden)

	switch {
	case n.hidden:
	case v.n == nil:
		l.roots = append(l.roots, n)
	case v.n.root().committed || v.n.state != stateRunning:
		n.orphan = true
		l.orphans = append(l.orphans, n)
	default:
		v.n.children = append(v.n.children, n)
	}

	l.fileEvent("▸", n, "started")
	l.afterEvent()
	return context.WithValue(ctx, ctxKey{}, &ctxVal{l: l, n: n}), &Task{l: l, n: n}
}

// End finishes the task: ✓ if *errp is nil, else ✗ with the error.
// Use it as defer t.End(&err).
func (t *Task) End(errp *error) {
	if errp != nil && *errp != nil {
		t.Fail(*errp)
		return
	}
	t.Done()
}

// Done finishes the task successfully.
func (t *Task) Done() { t.finish(nil) }

// Fail finishes the task as failed.
func (t *Task) Fail(err error) {
	if err == nil {
		err = errors.New("failed")
	}
	t.finish(err)
}

func (t *Task) finish(err error) {
	if t == nil || t.l == nil {
		return
	}
	t.l.mu.Lock()
	defer t.l.mu.Unlock()
	t.l.endLocked(t.n, err)
	t.l.afterEvent()
}

// endLocked finishes n and closes tasks still running below it as failed.
func (l *Runner) endLocked(n *node, err error) {
	if n.state != stateRunning {
		return
	}
	for _, c := range n.children {
		if c.kind == kindTask {
			l.endLocked(c, errParentEnded)
		}
	}
	if n.out != nil {
		n.out.flushLocked()
	}

	n.end = l.now()
	n.err = err
	n.state = stateOK
	if err != nil {
		n.state = stateFailed
	}

	sym, msg := "✓", fmtDuration(n.end.Sub(n.start))
	if err != nil {
		sym, msg = "✗", failedTitle(n)
	}
	l.fileEvent(sym, n, msg)

	if n.orphan && !n.hidden {
		l.emitCommitted(n)
	}
}

// failedTitle is the text of a failed task line.
func failedTitle(n *node) string {
	switch {
	case n.err == nil:
		return n.title
	case errors.Is(n.err, errParentEnded):
		return n.title + " (parent ended)"
	case errors.Is(n.err, errClosed):
		return n.title + " (runner closed)"
	}
	return n.title + ": " + oneLine(n.err.Error())
}

// logLocked records a log line under n (or at the root if n is nil).
func (l *Runner) logLocked(n *node, level Level, msg string) {
	sym := "•"
	switch level {
	case LevelInfo:
		sym = "→"
	case LevelWarn:
		sym = "⚠"
	case LevelError:
		sym = "✗"
	}
	l.fileEvent(sym, n, msg)

	if (level == LevelDebug && !l.verbose) || (n != nil && n.hidden) {
		return
	}

	ln := &node{kind: kindLog, level: level, title: msg, parent: n, start: l.now()}
	switch {
	case n == nil:
		l.roots = append(l.roots, ln)
	case n.orphan || n.root().committed:
		l.emitCommitted(ln)
	default:
		n.children = append(n.children, ln)
	}
}

// emitCommitted prints a single node straight away at its depth.
func (l *Runner) emitCommitted(n *node) {
	var lines []line
	l.collect(n, n.depth(), l.now(), false, &lines)
	for _, s := range renderLines(lines, 0, l.tty) {
		l.pending.WriteString(s)
		l.pending.WriteByte('\n')
	}
}

// Run starts a task, runs fn with its context and ends the task.
func Run(ctx context.Context, title string, fn func(context.Context) error) (err error) {
	ctx, t := Start(ctx, title)
	defer func() {
		if r := recover(); r != nil {
			t.Fail(panicError(r, false))
			panic(r)
		}
		t.End(&err)
	}()
	return fn(ctx)
}

// panicError turns a recovered panic value into an error, optionally with the
// panicking goroutine's stack (for panics that are not re-raised).
func panicError(r any, withStack bool) error {
	if !withStack {
		return fmt.Errorf("panic: %v", r)
	}
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return fmt.Errorf("panic: %v\n%s", r, buf[:n])
}

// Step is one unit of work for Parallel.
type Step struct {
	Title string
	Fn    func(context.Context) error
}

// Parallel runs the steps concurrently, each as its own child task of ctx's
// task. The first error cancels the context of the others. The returned
// error joins all step errors, except cancellations caused by a sibling's
// failure.
//
// Steps must not write shared state; they return results and the caller
// assigns them after Parallel returns.
func Parallel(ctx context.Context, steps ...Step) error {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create the tasks up front so they appear in step order.
	ctxs := make([]context.Context, len(steps))
	tasks := make([]*Task, len(steps))
	for i, s := range steps {
		ctxs[i], tasks[i] = Start(ctx, s.Title)
	}

	errs := make([]error, len(steps))
	var wg sync.WaitGroup
	for i, s := range steps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			defer func() {
				if r := recover(); r != nil {
					err = panicError(r, true)
					tasks[i].Fail(err)
					errs[i] = err
					cancel()
				}
			}()
			err = s.Fn(ctxs[i])
			tasks[i].End(&err)
			errs[i] = err
			if err != nil {
				cancel()
			}
		}()
	}
	wg.Wait()

	var out []error
	for _, err := range errs {
		if err == nil || errors.Is(err, context.Canceled) {
			continue
		}
		out = append(out, err)
	}
	if len(out) > 0 {
		return errors.Join(out...)
	}
	return parent.Err()
}
