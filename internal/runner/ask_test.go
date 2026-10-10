package runner

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type answer struct {
	line string
	err  error
}

// askOpen starts Ask on a pipe and returns once the question is on screen.
// send delivers the operator's line, echoing it like a terminal would.
func askOpen(t *testing.T, h *harness, ctx context.Context, prompt string) (send func(string), done <-chan answer) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	h.l.in = bufio.NewReader(pr)

	ch := make(chan answer, 1)
	go func() {
		line, err := Ask(ctx, prompt)
		ch <- answer{line, err}
	}()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		h.l.mu.Lock()
		open := h.l.suspended
		h.l.mu.Unlock()
		if open {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prompt never opened")
		}
	}
	send = func(s string) {
		if s == "" {
			pw.Close() // EOF: nothing is echoed
			return
		}
		h.l.mu.Lock()
		h.buf.WriteString(s)
		h.l.mu.Unlock()
		io.WriteString(pw, s)
	}
	return send, ch
}

func TestAskShowsQuestionInTaskAndErasesIt(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, task := Start(h.ctx, "Task")
	h.l.draw()

	send, done := askOpen(t, h, ctx, "Pick one:\n  1) a\nNumber: ")
	eq(t, squash(h.screen()), []string{"⠙ Task", "  ? Pick one:", "      1) a", "    Number:"})
	if !strings.Contains(h.buf.String(), yellow+"?"+reset+" Pick one:") {
		t.Errorf("question mark not coloured: %q", h.buf.String())
	}
	h.l.draw() // the live area is frozen while the question is open
	eq(t, squash(h.screen()), []string{"⠙ Task", "  ? Pick one:", "      1) a", "    Number:"})

	send("1\n")
	if a := <-done; a.line != "1" || a.err != nil {
		t.Fatalf("answer = %q, %v", a.line, a.err)
	}
	eq(t, squash(h.screen()), []string{"⠹ Task"})
	out := h.buf.String()
	if strings.LastIndex(out, "\033[?25l") < strings.LastIndex(out, "\033[?25h") {
		t.Error("cursor not hidden again after the answer")
	}

	task.Done()
	h.l.draw()
	eq(t, squash(h.screen()), []string{"✓ Task 0ms"})
}

func TestAskErasesWrappedQuestion(t *testing.T) {
	h := newTTY(t, 20, 24, false)
	h.wrap = 20
	ctx, _ := Start(h.ctx, "Task")
	h.l.draw()

	// "  ? " + 30 runes wraps onto a second row of the 20-column terminal.
	send, done := askOpen(t, h, ctx, strings.Repeat("x", 30))
	if got := h.screen(); len(got) != 3 {
		t.Fatalf("question not wrapped:\n%s", strings.Join(got, "\n"))
	}
	send("y\n")
	<-done
	eq(t, squash(h.screen()), []string{"⠹ Task"})
}

func TestAskEOFLeavesNoQuestionBehind(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, _ := Start(h.ctx, "Task")
	h.l.draw()

	send, done := askOpen(t, h, ctx, "Name: ")
	send("")
	if a := <-done; a.line != "" || a.err != io.EOF {
		t.Fatalf("answer = %q, %v; want EOF", a.line, a.err)
	}
	eq(t, squash(h.screen()), []string{"⠹ Task"})
}

func TestAskCancelled(t *testing.T) {
	h := newTTY(t, 80, 24, false)
	ctx, cancel := context.WithCancel(h.ctx)
	ctx, _ = Start(ctx, "Task")
	h.l.draw()

	_, done := askOpen(t, h, ctx, "Name: ")
	cancel()
	if a := <-done; a.err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", a.err)
	}
	eq(t, squash(h.screen()), []string{"⠹ Task"})
}

func TestAskNonTTYSaysWhereItIs(t *testing.T) {
	var buf bytes.Buffer
	l := newRunner(Options{Out: &buf, In: strings.NewReader("yes")}, false, nil)
	root := NewContext(context.Background(), l)
	ctx, _ := Start(root, "Task")

	// A last line without newline still counts as an answer.
	line, err := Ask(ctx, "Sure? ")
	if line != "yes" || err != nil {
		t.Fatalf("answer = %q, %v", line, err)
	}
	if got, want := buf.String(), "▸ Task\n  ? Sure? "; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	// Outside a task there is no path to show.
	buf.Reset()
	if _, err := Ask(root, "More? "); err != io.EOF {
		t.Fatalf("err = %v, want EOF once the input is used up", err)
	}
	if got, want := buf.String(), "? More? "; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestAskWithoutRunnerOrInput(t *testing.T) {
	if _, err := Ask(context.Background(), "x"); err != io.EOF {
		t.Errorf("no runner: err = %v, want EOF", err)
	}
	ctx := NewContext(context.Background(), newRunner(Options{}, false, nil))
	if _, err := Ask(ctx, "x"); err != io.EOF {
		t.Errorf("no input: err = %v, want EOF", err)
	}
}

func TestChoose(t *testing.T) {
	items := []string{"a", "b"}
	for _, c := range []struct {
		input   string
		want    int
		wantErr string
	}{
		{"2\n", 1, ""},
		{"1", 0, ""},
		{"7\n", 0, "invalid selection"},
		{"x\n", 0, "invalid selection"},
		{"", 0, "read selection"},
	} {
		var buf bytes.Buffer
		l := newRunner(Options{Out: &buf, In: strings.NewReader(c.input)}, false, nil)
		got, err := Choose(NewContext(context.Background(), l), "Pick:", items)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%q: err = %v, want containing %q", c.input, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q: got %d, %v; want %d", c.input, got, err, c.want)
		}
		if want := "? Pick:\n    1) a\n    2) b\n  Number: "; buf.String() != want {
			t.Errorf("%q: shown %q, want %q", c.input, buf.String(), want)
		}
	}
}

func TestPromptLines(t *testing.T) {
	for _, c := range []struct {
		text  string
		width int
		want  int
	}{
		{"", 80, 1},
		{"short", 80, 1},
		{"two\nlines", 80, 2},
		{"abcdef", 4, 2},
		{"abcd", 4, 1},
	} {
		if got := promptLines(c.text, c.width); got != c.want {
			t.Errorf("promptLines(%q, %d) = %d, want %d", c.text, c.width, got, c.want)
		}
	}
}
