package main

import (
	"context"
	"installer/internal/cli"
	"installer/internal/runner"
	"installer/internal/state"
)

// resolveInputs asks for whatever the flags left empty (target, host) and
// loads the persisted state of the chosen host, since state is keyed by host
// name. opts is completed in place.
func resolveInputs(ctx context.Context, opts *cli.Options, in *runner.Input, stateDir string) (*state.State, error) {
	var st *state.State
	err := runner.Run(ctx, "Checking inputs", func(ctx context.Context) error {
		ask := func(prompt string) (string, error) { return runner.Ask(ctx, in, prompt) }
		if err := opts.CompleteTarget(ask); err != nil {
			return err
		}
		hosts, err := cli.ListHosts(opts.ConfigDir)
		if err != nil {
			return err
		}
		host, err := cli.PickHost(ask, hosts, opts.Host)
		if err != nil {
			return err
		}
		if st, err = state.Load(stateDir, host); err != nil {
			return err
		}
		st.Target = opts.TargetIP
		runner.Info(ctx, "Target: "+opts.TargetIP)
		runner.Info(ctx, "Host: "+host)
		return nil
	})
	return st, err
}
