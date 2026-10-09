package main

import (
	"context"
	"installer/internal/command"
	"installer/internal/command/executor"
	"installer/internal/runner"
)

// streaming forwards command output to the runner task in ctx, keeping the
// executors independent of the UI package.
type streaming struct{ executor.Executor }

func (s streaming) Run(ctx context.Context, cmd command.Command, opts executor.ExecOptions) (command.Result, error) {
	if opts.Out == nil {
		opts.Out = runner.Output(ctx)
	}
	return s.Executor.Run(ctx, cmd, opts)
}
