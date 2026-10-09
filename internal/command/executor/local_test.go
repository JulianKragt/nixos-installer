package executor

import (
	"bytes"
	"context"
	"installer/internal/command"
	"strings"
	"testing"
)

func TestLocalCapturesOutputAndExitCode(t *testing.T) {
	var live bytes.Buffer
	res, err := NewLocal().Run(context.Background(), command.New("sh", "-c", "echo out; echo err >&2; exit 3"), ExecOptions{Out: &live})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	for _, s := range []string{res.Output, live.String()} {
		if !strings.Contains(s, "out") || !strings.Contains(s, "err") {
			t.Errorf("output %q misses stdout/stderr", s)
		}
	}
}

func TestLocalReportsLaunchFailure(t *testing.T) {
	if _, err := NewLocal().Run(context.Background(), command.New("definitely-not-a-command"), ExecOptions{}); err == nil {
		t.Error("want error when the process cannot start")
	}
}
