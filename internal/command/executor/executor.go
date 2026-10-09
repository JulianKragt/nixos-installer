package executor

import (
	"context"
	"installer/internal/command"
	"io"
)

// ExecOptions configures optional execution parameters. The zero value is safe
// (no stdin, inherited env and working directory).
type ExecOptions struct {
	Stdin io.Reader // stream for stdin; never pass secrets via args
	Env   []string  // extra env vars appended to os.Environ()
	Dir   string    // working directory; "" inherits the process cwd
}

type Executor interface {
	Run(ctx context.Context, cmd command.Command, opts ExecOptions) (command.Result, error)
}
