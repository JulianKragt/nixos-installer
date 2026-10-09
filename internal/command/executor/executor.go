package executor

import (
	"context"
	"installer/internal/command"
	"io"
)

// ExecOptions configures optional execution parameters. The zero value is safe
// (no stdin, no live output sink).
type ExecOptions struct {
	Stdin io.Reader // stream for stdin; never pass secrets via args
	Out   io.Writer // optional live sink for combined stdout/stderr; Result.Output is always captured
}

type Executor interface {
	Run(ctx context.Context, cmd command.Command, opts ExecOptions) (command.Result, error)
}

// Tee returns a writer that writes to buf and, when out is non-nil, to out.
func Tee(buf io.Writer, out io.Writer) io.Writer {
	if out == nil {
		return buf
	}
	return io.MultiWriter(buf, out)
}
