package pipeline

import (
	"context"
	"installer/internal/runner"
	"installer/internal/stage"
	"sort"
	"strconv"
	"strings"
)

type Pipeline struct {
	stages []stage.Stage
	env    *stage.Env
}

func New(stages []stage.Stage, env *stage.Env) Pipeline {
	sorted := make([]stage.Stage, len(stages))
	copy(sorted, stages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Index() < sorted[j].Index()
	})
	return Pipeline{stages: sorted, env: env}
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

	var skipped, pending []stage.Stage
	for _, s := range p.stages {
		runner.Debug(ctx, "Processing stage "+strconv.Itoa(s.Index())+": "+s.Name())
		if s.Index() <= p.env.State.StageIndex {
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
		runner.Warn(ctx, "Skipping already processed stages: "+strings.Join(stageNames(skipped), ","))
	}

	runner.Info(ctx, "Stages to process: "+strings.Join(stageNames(pending), ","))

	for _, s := range pending {
		err := runner.Run(ctx, s.Name(), func(ctx context.Context) error {
			return s.Run(ctx, p.env)
		})
		if err != nil {
			return err
		}
	}
	runner.Debug(ctx, "Pipeline processed stages: "+strings.Join(stageNames(pending), ","))
	return nil
}
