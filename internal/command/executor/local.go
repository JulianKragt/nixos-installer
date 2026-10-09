package executor

import (
	"bytes"
	"context"
	"installer/internal/command"
	"os"
	"os/exec"
)

type Local struct{}

func NewLocal() *Local {
	return &Local{}
}

func (e *Local) Run(ctx context.Context, cmd command.Command, opts ExecOptions) (command.Result, error) {
	process := exec.CommandContext(ctx, cmd.Name(), cmd.Args()...)
	process.Stdin = opts.Stdin
	process.Dir = opts.Dir
	if len(opts.Env) > 0 {
		process.Env = append(os.Environ(), opts.Env...)
	}

	var output bytes.Buffer
	w := Tee(&output, opts.Out)
	process.Stdout = w
	process.Stderr = w

	err := process.Run()

	if process.ProcessState == nil {
		// Process never started — exec launch failure.
		return command.Result{}, err
	}

	return command.Result{
		Output:   output.String(),
		ExitCode: process.ProcessState.ExitCode(),
	}, nil
}
