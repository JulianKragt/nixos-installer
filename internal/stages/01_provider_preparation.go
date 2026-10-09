package stages

import (
	"context"
	"installer/internal/commands"
	"installer/internal/runner"
	"installer/internal/stage"
)

type ProviderPreparationStage struct{}

func (ProviderPreparationStage) ID() string   { return "0.1" }
func (ProviderPreparationStage) Name() string { return "Provider preparation" }

func (ProviderPreparationStage) Run(ctx context.Context, env *stage.Env) error {
	return runner.Parallel(ctx,
		runner.Step{
			Title: "Check internet connection",
			Fn: func(ctx context.Context) error {
				return commands.DialTCP(ctx, env.Local, "8.8.8.8:53")
			},
		},
		runner.Step{
			Title: "Check target is reachable",
			Fn: func(ctx context.Context) error {
				return commands.DialTCP(ctx, env.Local, env.State.Target+":22")
			},
		},
	)
}

func (ProviderPreparationStage) Rollback(ctx context.Context, env *stage.Env) error {
	return nil
}
