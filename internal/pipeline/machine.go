package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"installer/internal/runner"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// Machine runs commands on a machine: this one (the zero value) or another
// one over ssh.
type Machine struct {
	ssh []string // ssh argv up to and including the destination; nil = this machine
}

// SSH is the root account on the machine at addr, reached with the system's
// ssh: keys come from ssh-agent and ~/.ssh as for any other ssh call.
func SSH(addr string) Machine {
	return Machine{ssh: []string{
		"ssh",
		"-o", "BatchMode=yes", // the runner owns the terminal: fail instead of asking
		"-o", "ConnectTimeout=10",
		// TODO: replace with TOFU once host key pinning is implemented
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-l", "root",
		addr,
	}}
}

// Run runs a command and returns its combined stdout and stderr, which is also
// streamed to the runner task in ctx. A command that cannot be started or
// exits non-zero is an error; it wraps *exec.ExitError for callers that need
// the exit code.
func (m Machine) Run(ctx context.Context, name string, args ...string) (string, error) {
	return m.RunStdin(ctx, nil, name, args...)
}

// RunStdin is Run with stdin connected. Secrets go here, never into args.
func (m Machine) RunStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (string, error) {
	argv := append([]string{name}, args...)
	if m.ssh != nil {
		argv = append(slices.Clone(m.ssh), shellJoin(argv))
	}
	process := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.Stdin = stdin

	var output bytes.Buffer
	w := io.MultiWriter(&output, runner.Output(ctx))
	process.Stdout = w
	process.Stderr = w

	if err := process.Run(); err != nil {
		if last := lastLine(output.String()); last != "" {
			return "", fmt.Errorf("%s: %w: %s", name, err, last)
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return output.String(), nil
}

// shellJoin quotes argv for the remote shell: ssh hands it a single string.
func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// lastLine is the last non-empty line of s.
func lastLine(s string) string {
	s = strings.TrimSpace(s)
	return s[strings.LastIndexByte(s, '\n')+1:]
}
