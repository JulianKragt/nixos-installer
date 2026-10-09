package stages

import (
	"context"
	"fmt"
	"installer/internal/runner"
	"installer/internal/stage"
	"net"
	"time"
)

const dialTimeout = 5 * time.Second

type ProviderPreparationStage struct{}

func (ProviderPreparationStage) ID() string   { return "0.1" }
func (ProviderPreparationStage) Name() string { return "Provider preparation" }

func (ProviderPreparationStage) Run(ctx context.Context, env *stage.Env) error {
	return runner.Parallel(ctx,
		runner.Step{
			Title: "Check internet connection",
			Fn: func(ctx context.Context) error {
				return dialTCP(ctx, "8.8.8.8:53")
			},
		},
		runner.Step{
			Title: "Check target is reachable",
			Fn: func(ctx context.Context) error {
				return dialTCP(ctx, env.State.Target+":22")
			},
		},
	)
}

func (ProviderPreparationStage) Rollback(ctx context.Context, env *stage.Env) error {
	return nil
}

func dialTCP(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("unreachable %s: %w", addr, err)
	}
	conn.Close()
	return nil
}
