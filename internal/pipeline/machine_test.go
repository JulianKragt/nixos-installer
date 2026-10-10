package pipeline

import (
	"bytes"
	"context"
	"errors"
	"installer/internal/runner"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// loopback is a "remote" machine whose ssh is the local shell, so the command
// string takes the same route as over ssh: quoted here, parsed by a shell there.
var loopback = Machine{ssh: []string{"sh", "-c"}}

func TestRunCapturesAndStreamsOutput(t *testing.T) {
	var screen, file bytes.Buffer
	ctx := runner.NewContext(context.Background(), runner.New(runner.Options{Out: &screen, File: &file}))

	for name, m := range map[string]Machine{"local": {}, "remote": loopback} {
		file.Reset()
		out, err := m.Run(ctx, "sh", "-c", "echo out; echo err >&2")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, s := range []string{out, file.String()} {
			if !strings.Contains(s, "out") || !strings.Contains(s, "err") {
				t.Errorf("%s: %q misses stdout/stderr", name, s)
			}
		}
	}
}

func TestRunWorksWithoutRunner(t *testing.T) {
	out, err := Machine{}.Run(context.Background(), "echo", "hi")
	if err != nil || out != "hi\n" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestRunTurnsExitCodeIntoError(t *testing.T) {
	for name, m := range map[string]Machine{"local": {}, "remote": loopback} {
		_, err := m.Run(context.Background(), "sh", "-c", "echo first; echo why >&2; echo; exit 3")
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Fatalf("%s: err = %v, want exit code 3", name, err)
		}
		if got, want := err.Error(), "sh: exit status 3: why"; got != want {
			t.Errorf("%s: err = %q, want %q", name, got, want)
		}
	}
}

func TestRunReportsLaunchFailure(t *testing.T) {
	_, err := Machine{}.Run(context.Background(), "definitely-not-a-command")
	if err == nil || !strings.HasPrefix(err.Error(), "definitely-not-a-command: ") {
		t.Errorf("err = %v, want a launch error naming the command", err)
	}
}

func TestRemoteArgumentsSurviveTheShell(t *testing.T) {
	args := []string{"a b", "it's", "$HOME", `"q"`, "*", "", `back\slash`, "semi;colon", "new\nline"}
	out, err := loopback.Run(context.Background(), "printf", append([]string{"<%s>"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<" + strings.Join(args, "><") + ">"; out != want {
		t.Fatalf("got  %q\nwant %q", out, want)
	}
}

func TestRunStdin(t *testing.T) {
	for name, m := range map[string]Machine{"local": {}, "remote": loopback} {
		out, err := m.RunStdin(context.Background(), strings.NewReader("secret\n"), "cat")
		if err != nil || out != "secret\n" {
			t.Errorf("%s: got %q, %v", name, out, err)
		}
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Machine{}).Run(ctx, "sleep", "10"); err == nil {
		t.Fatal("want an error for a cancelled context")
	}
}

func TestSSHArgv(t *testing.T) {
	for _, addr := range []string{"192.168.1.100", "2001:db8::5", "fe80::1%eth0"} {
		argv := SSH(addr).ssh
		if argv[0] != "ssh" || !slices.Equal(argv[len(argv)-3:], []string{"-l", "root", addr}) {
			t.Errorf("%s: argv = %q", addr, argv)
		}
		if !slices.Contains(argv, "BatchMode=yes") {
			t.Errorf("%s: ssh may prompt and garble the live area: %q", addr, argv)
		}
	}
}
