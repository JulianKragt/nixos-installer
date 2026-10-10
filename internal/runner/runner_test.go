package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTerm interprets the few escape sequences the renderer emits.
type fakeTerm struct {
	rows [][]rune
	r, c int
	cols int // wrap at this column; 0 = never
}

func (t *fakeTerm) row() *[]rune {
	for len(t.rows) <= t.r {
		t.rows = append(t.rows, nil)
	}
	return &t.rows[t.r]
}

func (t *fakeTerm) write(s string) {
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch ch := rs[i]; ch {
		case '\n':
			t.r++
			t.c = 0
			t.row()
		case '\r':
			t.c = 0
		case '\033':
			j := i + 1
			if j < len(rs) && rs[j] == '[' {
				j++
				k := j
				for k < len(rs) && (rs[k] < '@' || rs[k] > '~') {
					k++
				}
				params, final := string(rs[j:k]), rs[k]
				i = k
				n := 1
				fmt.Sscanf(params, "%d", &n)
				switch final {
				case 'A':
					t.r -= n
				case 'J':
					row := t.row()
					if t.c < len(*row) {
						*row = (*row)[:t.c]
					}
					t.rows = t.rows[:t.r+1]
				}
			}
		default:
			if t.cols > 0 && t.c == t.cols {
				t.r++
				t.c = 0
			}
			row := t.row()
			for len(*row) < t.c {
				*row = append(*row, ' ')
			}
			if t.c < len(*row) {
				(*row)[t.c] = ch
			} else {
				*row = append(*row, ch)
			}
			t.c++
		}
	}
}

func (t *fakeTerm) screen() []string {
	var out []string
	for _, r := range t.rows {
		out = append(out, strings.TrimRight(string(r), " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

type harness struct {
	buf  bytes.Buffer
	l    *Runner
	clk  *clock
	ctx  context.Context
	wrap int // screen() wraps at this column; 0 = never
}

func newTTY(t *testing.T, w, h int, verbose bool) *harness {
	t.Helper()
	hn := &harness{clk: &clock{t: time.Date(2026, 1, 1, 15, 12, 3, 0, time.UTC)}}
	hn.l = newRunner(Options{Out: &hn.buf, Verbose: verbose}, true, func() (int, int) { return w, h })
	hn.l.now = hn.clk.now
	hn.ctx = NewContext(context.Background(), hn.l)
	return hn
}

func (h *harness) screen() []string {
	ft := fakeTerm{cols: h.wrap}
	ft.write(h.buf.String())
	return ft.screen()
}

// text strips the duration column so tests do not depend on layout.
func squash(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Join(strings.Fields(l), " ")
		// keep indentation information
		out[i] = strings.Repeat(" ", len(l)-len(strings.TrimLeft(l, " "))) + out[i]
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestParentStaysAboveChildren(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, parent := Start(h.ctx, "Parent")
	_ = Run(ctx, "Child A", func(context.Context) error { return nil })
	_ = Run(ctx, "Child B", func(context.Context) error { return errors.New("boom") })

	h.l.draw()
	got := squash(h.screen())
	eq(t, got, []string{"⠋ Parent", "  ✓ Child A 0ms", "  ✗ Child B: boom 0ms"})

	parent.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Parent 0ms", "  ✓ Child A 0ms", "  ✗ Child B: boom 0ms"})
	if h.l.liveLines != 0 {
		t.Fatalf("liveLines = %d, want 0", h.l.liveLines)
	}
}

func TestParallelSpinnersAndStartOrder(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, root := Start(h.ctx, "Root")
	_, a := Start(ctx, "A")
	_, b := Start(ctx, "B")
	h.l.draw()
	got := squash(h.screen())
	eq(t, got, []string{"⠋ Root", "  ⠋ A", "  ⠋ B"})

	b.Done()
	h.clk.advance(1500 * time.Millisecond)
	a.Done()
	root.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Root 1.5s", "  ✓ A 1.5s", "  ✓ B 0ms"})
}

func TestRootFinishingEarlyStaysLive(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	_, a := Start(h.ctx, "A")
	_, b := Start(h.ctx, "B")
	b.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"⠋ A", "✓ B 0ms"})
	if h.l.committed != 0 {
		t.Fatal("B committed before A")
	}

	a.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ A 0ms", "✓ B 0ms"})
	if h.l.committed != 2 || h.l.liveLines != 0 {
		t.Fatalf("committed=%d live=%d", h.l.committed, h.l.liveLines)
	}
}

func TestLiveRedrawIsInPlace(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	Info(h.ctx, "first")
	_, a := Start(h.ctx, "A")
	for i := 0; i < 5; i++ {
		h.l.draw()
	}
	eq(t, squash(h.screen()), []string{"✓ first", "⠼ A"})
	a.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ first", "✓ A 0ms"})
}

func TestHeightOverflow(t *testing.T) {
	h := newTTY(t, 80, 6, false) // 5 usable rows
	ctx, root := Start(h.ctx, "Root")
	for i := 1; i <= 8; i++ {
		_ = Run(ctx, fmt.Sprintf("step %d", i), func(context.Context) error { return nil })
	}
	h.l.draw()
	got := squash(h.screen())
	eq(t, got, []string{"⠋ Root", "  … 5 lines", "  ✓ step 6 0ms", "  ✓ step 7 0ms", "  ✓ step 8 0ms"})

	root.Done()
	h.l.draw()
	got = h.screen()
	if len(got) != 9 || !strings.Contains(got[1], "step 1") || !strings.Contains(got[8], "step 8") {
		t.Fatalf("commit not in full:\n%s", strings.Join(got, "\n"))
	}
}

func TestLinesNeverWrap(t *testing.T) {
	h := newTTY(t, 30, 24, false)
	_, task := Start(h.ctx, strings.Repeat("long title ", 10))
	Info(task2ctx(h.ctx, task), "x")
	h.clk.advance(5 * time.Second)
	h.l.draw()
	h.l.mu.Lock()
	live := h.l.liveRender()
	h.l.mu.Unlock()
	for _, s := range live {
		plain := ansiRE.ReplaceAllString(s, "")
		if runeLen(plain) > 29 {
			t.Fatalf("line too wide (%d): %q", runeLen(plain), plain)
		}
	}
}

// task2ctx is a helper to log into a task created without keeping its ctx.
func task2ctx(ctx context.Context, t *Task) context.Context {
	return context.WithValue(ctx, ctxKey{}, &ctxVal{l: t.l, n: t.n})
}

func TestParentEndedClosesChildren(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, parent := Start(h.ctx, "Parent")
	Start(ctx, "Orphaned")
	parent.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Parent 0ms", "  ✗ Orphaned (parent ended) 0ms"})
}

func TestLogAfterCommit(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, task := Start(h.ctx, "Task")
	task.Done()
	h.l.draw()
	Warn(ctx, "late")
	_, late := Start(ctx, "late task")
	late.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Task 0ms", "  ⚠ late", "  ✓ late task 0ms"})
}

func TestLateTaskIsLiveUntilItEnds(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, task := Start(h.ctx, "Task")
	task.Done()
	h.l.draw()

	ctx, late := Start(ctx, "late task")
	Info(ctx, "inside")
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Task 0ms", "  ⠙ late task", "    → inside"})

	late.Done()
	h.l.draw()
	Info(ctx, "after")
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Task 0ms", "  ✓ late task 0ms", "    → inside", "    → after"})
	if h.l.liveLines != 0 {
		t.Fatalf("liveLines = %d, want 0", h.l.liveLines)
	}
}

func TestElapsedAfterOneSecond(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	Start(h.ctx, "slow")
	h.l.draw()
	if strings.Contains(h.screen()[0], "s") && strings.Contains(h.screen()[0], "ms") {
		t.Fatal("duration shown too early")
	}
	h.clk.advance(2*time.Minute + 3*time.Second)
	h.l.draw()
	if !strings.HasSuffix(h.screen()[0], "2m03s") {
		t.Fatalf("got %q", h.screen()[0])
	}
}

func TestDurationFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		120 * time.Millisecond:  "120ms",
		1200 * time.Millisecond: "1.2s",
		12 * time.Second:        "12s",
		123 * time.Second:       "2m03s",
	} {
		if got := fmtDuration(d); got != want {
			t.Errorf("fmtDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestColorsOnTTY(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	_ = Run(h.ctx, "ok", func(context.Context) error { return nil })
	h.l.draw()
	out := h.buf.String()
	for _, want := range []string{green + "✓" + reset, "\033[?25l"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	h.l.Close()
	if !strings.HasSuffix(h.buf.String(), "\033[?25h") {
		t.Error("cursor not restored on Close")
	}
}

func TestNonTTY(t *testing.T) {
	var buf bytes.Buffer
	l := newRunner(Options{Out: &buf}, false, nil)
	ctx := NewContext(context.Background(), l)
	ctx1, a := Start(ctx, "A")
	_, b := Start(ctx, "B")
	fmt.Fprintln(Output(ctx1), "hidden while running")
	Info(ctx1, "inside")
	b.Done()
	if buf.Len() != 0 {
		t.Fatalf("printed before A committed: %q", buf.String())
	}
	a.Done()
	out := buf.String()
	if strings.Contains(out, "\033") || strings.Contains(out, "\r") {
		t.Fatalf("escape codes in non-tty output: %q", out)
	}
	eq(t, squash(strings.Split(strings.TrimSpace(out), "\n")), []string{"✓ A 0ms", "  → inside", "✓ B 0ms"})
}

func TestCloseEndsRunningTasks(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	Start(h.ctx, "left running")
	if err := h.l.Close(); err != nil {
		t.Fatal(err)
	}
	eq(t, squash(h.screen()), []string{"✗ left running (runner closed) 0ms"})
}

func TestDebugHiddenUnlessVerbose(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		h := newTTY(t, 80, 24, verbose)
		ctx, d := StartDebug(h.ctx, "debug task")
		Info(ctx, "child of debug")
		_ = Run(ctx, "nested", func(context.Context) error { return nil })
		d.Done()
		Debug(h.ctx, "debug line")
		_ = Run(h.ctx, "visible", func(context.Context) error { return nil })
		h.l.draw()
		got := squash(h.screen())
		if !verbose {
			eq(t, got, []string{"✓ visible 0ms"})
		} else {
			eq(t, got, []string{"✓ debug task 0ms", "  → child of debug", "  ✓ nested 0ms", "• debug line", "✓ visible 0ms"})
		}
	}
}

func TestNoRunnerIsNoop(t *testing.T) {
	ctx := context.Background()
	Info(ctx, "x")
	ctx, task := Start(ctx, "t")
	task.Done()
	task.Fail(errors.New("x"))
	if Output(ctx) == nil {
		t.Fatal("nil writer")
	}
	ran := false
	if err := Run(ctx, "r", func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatal("Run did not run fn")
	}
	if err := Parallel(ctx, Step{"p", func(context.Context) error { return nil }}); err != nil {
		t.Fatal(err)
	}
}

func TestEndFailureMessage(t *testing.T) {
	var buf bytes.Buffer
	l := newRunner(Options{Out: &buf}, false, nil)
	ctx := NewContext(context.Background(), l)
	err := Run(ctx, "Disk", func(context.Context) error { return errors.New("no space\nleft") })
	if err == nil || !strings.Contains(buf.String(), "✗ Disk: no space; left") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestParallelCancelsSiblingsAndJoinsErrors(t *testing.T) {
	var buf bytes.Buffer
	l := newRunner(Options{Out: &buf}, false, nil)
	ctx := NewContext(context.Background(), l)

	var mu sync.Mutex
	sawCancel := false
	err := Run(ctx, "Checks", func(ctx context.Context) error {
		return Parallel(ctx,
			Step{"slow", func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					mu.Lock()
					sawCancel = true
					mu.Unlock()
					return ctx.Err()
				case <-time.After(5 * time.Second):
					return nil
				}
			}},
			Step{"bad", func(ctx context.Context) error { return errors.New("exit 2") }},
		)
	})
	if err == nil || err.Error() != "exit 2" {
		t.Fatalf("err = %v", err)
	}
	if !sawCancel {
		t.Fatal("sibling not cancelled")
	}
	out := buf.String()
	if strings.Index(out, "slow") > strings.Index(out, "bad") {
		t.Fatalf("not in start order:\n%s", out)
	}

	err = Parallel(ctx,
		Step{"x", func(context.Context) error { return errors.New("e1") }},
		Step{"y", func(context.Context) error { return errors.New("e2") }},
	)
	if err == nil || !strings.Contains(err.Error(), "e1") && !strings.Contains(err.Error(), "e2") {
		t.Fatalf("err = %v", err)
	}
}

func TestParallelRace(t *testing.T) {
	var buf bytes.Buffer
	l := newRunner(Options{Out: &buf, File: &syncBuf{}}, false, nil)
	ctx := NewContext(context.Background(), l)
	steps := make([]Step, 20)
	for i := range steps {
		steps[i] = Step{fmt.Sprintf("s%d", i), func(ctx context.Context) error {
			for j := 0; j < 20; j++ {
				fmt.Fprintf(Output(ctx), "line %d\n", j)
				Info(ctx, "msg", "j", j)
			}
			return nil
		}}
	}
	if err := Parallel(ctx, steps...); err != nil {
		t.Fatal(err)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
