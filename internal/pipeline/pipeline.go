package pipeline

import (
	"context"
	"installer/internal/log"
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
	return Pipeline{
		stages: stages,
		env:    env,
	}
}

func (p *Pipeline) Run(ctx context.Context) error {
	log.Info(ctx, "Starting pipeline")

	sorted := make([]stage.Stage, len(p.stages))
	copy(sorted, p.stages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Index() < sorted[j].Index()
	})
	log.Debug(ctx, "Pipeline sorted provided stages")

	pending := make([]stage.Stage, 0, len(sorted))
	pendingNames := make([]string, 0, len(sorted))
	processedNames := make([]string, 0, len(sorted))

	for _, s := range sorted {
		log.Debug(ctx, "Processing stage "+strconv.Itoa(s.Index())+": "+s.Name())
		if s.Index() <= p.env.State.StageIndex {
			processedNames = append(processedNames, s.Name())
		} else {
			pending = append(pending, s)
			pendingNames = append(pendingNames, s.Name())
		}
	}

	if len(pending) == 0 {
		log.Warn(ctx, "All stages already have been processed")
		return nil
	}

	if len(processedNames) > 0 {
		log.Warn(ctx, "Skipping already processed stages: "+strings.Join(processedNames, ","))
	}

	log.Info(ctx, "Stages to process: "+strings.Join(pendingNames, ","))

	for _, s := range pending {
		err := log.Run(ctx, s.Name(), func(ctx context.Context) error {
			return s.Run(ctx, p.env)
		})
		if err != nil {
			return err
		}
	}
	log.Debug(ctx, "Pipeline processed stages: "+strings.Join(pendingNames, ","))
	return nil
}
