package executor

import (
	"bytes"
	"context"
	"installer/internal/command"
	"installer/internal/runner"
	"io"
	"os/exec"
)

type Local struct{}

func NewLocal() *Local {
	return &Local{}
}

func (e *Local) Run(ctx context.Context, cmd command.Command) command.Result {
	process := exec.CommandContext(
		ctx,
		cmd.Name(),
		cmd.Args()...,
	)

	var output bytes.Buffer

	// One writer for both streams, so exec serializes the writes.
	w := io.MultiWriter(&output, runner.Output(ctx))
	process.Stdout = w
	process.Stderr = w

	err := process.Run()

	result := command.Result{
		Output: output.String(),
		Err:    err,
	}

	if process.ProcessState != nil {
		result.ExitCode = process.ProcessState.ExitCode()
	}

	return result
}
