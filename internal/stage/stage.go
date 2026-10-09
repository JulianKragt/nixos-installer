package stage

import (
	"context"
	"installer/internal/command/executor"
	"installer/internal/state"
)

// Env is everything a stage needs besides its context. Parallel steps must
// not write State; they return results and the stage assigns them after the
// join.
type Env struct {
	State  *state.State
	Local  executor.Executor
	Remote executor.Executor
}

type Stage interface {
	ID() string
	Name() string
	Run(ctx context.Context, env *Env) error
	Rollback(ctx context.Context, env *Env) error
}

// AlwaysRun is implemented by stages that must run on every invocation, even
// when already completed (e.g. input resolution). The pipeline does not record
// them in State.CompletedStages.
type AlwaysRun interface {
	AlwaysRun() bool
}
