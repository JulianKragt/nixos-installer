package executor

import (
	"context"
	"installer/internal/command"
)

type Executor interface {
	Run(ctx context.Context, cmd command.Command) command.Result
}
