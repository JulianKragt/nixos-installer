package stages

import (
	"context"
	"installer/internal/command/adapter"
	"installer/internal/runner"
	"installer/internal/stage"
)

const internetCheckHost = "google.com"

type ProviderPreparationStage struct{}

func (ProviderPreparationStage) Index() int { return 1 }

func (ProviderPreparationStage) Run(ctx context.Context, env *stage.Env) error {
	internet := adapter.NewPing(internetCheckHost)
	target := adapter.NewPing(env.State.Target)

	return runner.Parallel(ctx,
		runner.Step{
			Title: "Check internet connection",
			Fn: func(ctx context.Context) error {
				return env.Local.Run(ctx, &internet).Err
			},
		},
		runner.Step{
			Title: "Check target is reachable",
			Fn: func(ctx context.Context) error {
				return env.Local.Run(ctx, &target).Err
			},
		},
	)
}

func (ProviderPreparationStage) Name() string {
	return "Provider preparation"
}

func (ProviderPreparationStage) Rollback(ctx context.Context, env *stage.Env) error {
	return nil
}
