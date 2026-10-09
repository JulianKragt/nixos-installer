package pipeline

import (
	"context"
	"installer/internal/runner"
	"installer/internal/stage"
	"sort"
	"strings"
)

type Pipeline struct {
	stages   []stage.Stage
	env      *stage.Env
	stateDir string
}

func New(stages []stage.Stage, env *stage.Env, stateDir string) Pipeline {
	sorted := make([]stage.Stage, len(stages))
	copy(sorted, stages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ID() < sorted[j].ID()
	})
	return Pipeline{stages: sorted, env: env, stateDir: stateDir}
}

func stageNames(ss []stage.Stage) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.Name()
	}
	return names
}

func (p *Pipeline) Run(ctx context.Context) error {
	runner.Info(ctx, "Starting pipeline")

	completed := make(map[string]struct{}, len(p.env.State.CompletedStages))
	for _, id := range p.env.State.CompletedStages {
		completed[id] = struct{}{}
	}

	var skipped, pending []stage.Stage
	for _, s := range p.stages {
		if _, done := completed[s.ID()]; done {
			skipped = append(skipped, s)
		} else {
			pending = append(pending, s)
		}
	}

	if len(pending) == 0 {
		runner.Warn(ctx, "All stages already have been processed")
		return nil
	}

	if len(skipped) > 0 {
		runner.Warn(ctx, "Skipping already processed stages: "+strings.Join(stageNames(skipped), ", "))
	}

	runner.Info(ctx, "Stages to process: "+strings.Join(stageNames(pending), ", "))

	for _, s := range pending {
		err := runner.Run(ctx, s.Name(), func(ctx context.Context) error {
			return s.Run(ctx, p.env)
		})
		if err != nil {
			return err
		}
		p.env.State.CompletedStages = append(p.env.State.CompletedStages, s.ID())
		if saveErr := p.env.State.Save(p.stateDir); saveErr != nil {
			runner.Warn(ctx, "Failed to persist state: "+saveErr.Error())
		}
	}

	runner.Debug(ctx, "Pipeline processed stages: "+strings.Join(stageNames(pending), ", "))
	return nil
}
