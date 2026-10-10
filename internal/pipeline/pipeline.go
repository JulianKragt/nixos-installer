// Package pipeline is the engine the stages run in: the machines they run
// commands on, the state that survives a restart, and the loop that runs the
// stages in order and resumes behind the last completed one.
package pipeline

import (
	"context"
	"fmt"
	"installer/internal/runner"
	"slices"
)

// Inputs are the operator's choices for this run. They are complete before
// the first stage runs.
type Inputs struct {
	Host      string // host to install: a directory under hosts/nixos in the config flake
	Target    string // IP address of the target machine
	ConfigDir string // path to the nixos-config flake
	Disk      string // disk to install to; empty = detect on the target
}

// Env is everything a stage needs besides its context. Parallel steps must
// not write State; they return results and the stage assigns them after the
// join.
type Env struct {
	Inputs
	State  *State
	Local  Machine // the provider: the machine the installer runs on
	Remote Machine // the target
}

// Stage is one step of the pipeline.
type Stage struct {
	ID   string // as in the Pipeline doc: "0.0", "2.3"
	Name string
	// Always marks a stage that runs on every invocation, even when it ran
	// before (e.g. environment checks). It is not recorded in State.Completed.
	Always bool
	Run    func(ctx context.Context, env *Env) error
}

// Run runs the stages in the given order. A stage in State.Completed is
// skipped; every other stage is recorded there when it succeeds and the state
// is saved, so the next invocation resumes behind it.
func Run(ctx context.Context, env *Env, stages []Stage) error {
	runner.Info(ctx, "Starting pipeline")
	for _, s := range stages {
		title := s.ID + " " + s.Name
		if !s.Always && slices.Contains(env.State.Completed, s.ID) {
			runner.Info(ctx, title+" (done earlier)")
			continue
		}
		if err := runner.Run(ctx, title, func(ctx context.Context) error {
			if err := s.Run(ctx, env); err != nil {
				return err
			}
			if s.Always {
				return nil // re-run every time, never recorded
			}
			env.State.Completed = append(env.State.Completed, s.ID)
			if err := env.State.Save(); err != nil {
				return fmt.Errorf("persist state: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
