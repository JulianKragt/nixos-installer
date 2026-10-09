package pipeline

import (
	"context"
	"fmt"
	"installer/internal/runner"
	"installer/internal/stage"
	"sort"
	"strconv"
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
		return idLess(sorted[i].ID(), sorted[j].ID())
	})
	return Pipeline{stages: sorted, env: env, stateDir: stateDir}
}

// idLess orders stage IDs like "0.1" < "0.10" < "1.0" by comparing their
// dot-separated parts numerically, falling back to string order.
func idLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea != nil || eb != nil:
			if pa[i] != pb[i] {
				return pa[i] < pb[i]
			}
		case na != nb:
			return na < nb
		}
	}
	return len(pa) < len(pb)
}

func stageNames(ss []stage.Stage) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.Name()
	}
	return names
}

func stageTitle(s stage.Stage) string { return s.ID() + " " + s.Name() }

func alwaysRun(s stage.Stage) bool {
	a, ok := s.(stage.AlwaysRun)
	return ok && a.AlwaysRun()
}

func (p *Pipeline) Run(ctx context.Context) error {
	runner.Info(ctx, "Starting pipeline")

	completed := make(map[string]struct{}, len(p.env.State.CompletedStages))
	for _, id := range p.env.State.CompletedStages {
		completed[id] = struct{}{}
	}

	var skipped, pending []stage.Stage
	for _, s := range p.stages {
		if alwaysRun(s) {
			continue
		}
		if _, done := completed[s.ID()]; done {
			skipped = append(skipped, s)
		} else {
			pending = append(pending, s)
		}
	}

	if len(pending) == 0 {
		runner.Warn(ctx, "All stages already have been processed")
	} else {
		if len(skipped) > 0 {
			runner.Warn(ctx, "Skipping already processed stages: "+strings.Join(stageNames(skipped), ", "))
		}
		runner.Info(ctx, "Stages to process: "+strings.Join(stageNames(pending), ", "))
	}

	for _, s := range p.stages {
		always := alwaysRun(s)
		if _, done := completed[s.ID()]; done && !always {
			continue
		}
		if err := runner.Run(ctx, stageTitle(s), func(ctx context.Context) error {
			return s.Run(ctx, p.env)
		}); err != nil {
			return err
		}
		if always {
			continue // re-run every time, never recorded
		}
		p.env.State.CompletedStages = append(p.env.State.CompletedStages, s.ID())
		if err := p.env.State.Save(p.stateDir); err != nil {
			return fmt.Errorf("persist state after stage %s: %w", s.ID(), err)
		}
	}

	runner.Debug(ctx, "Pipeline processed stages: "+strings.Join(stageNames(pending), ", "))
	return nil
}
