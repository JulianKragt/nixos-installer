package stages

import (
	"bufio"
	"context"
	"fmt"
	"installer/internal/cli"
	"installer/internal/command/executor"
	"installer/internal/commands"
	"installer/internal/runner"
	"installer/internal/stage"
	"installer/internal/state"
	"os"
	"os/exec"
	"path/filepath"
)

// EnvironmentPreparationStage resolves the CLI inputs and checks the provider
// machine before anything touches the target. It runs on every invocation.
type EnvironmentPreparationStage struct {
	Opts     cli.Options
	SSHKey   string // private key used to reach the target
	StateDir string
	In       *bufio.Reader
	Remote   *executor.SSH // shared with Env.Remote; its Host is set from the target
}

func (*EnvironmentPreparationStage) ID() string      { return "0.0" }
func (*EnvironmentPreparationStage) Name() string    { return "Environment preparation" }
func (*EnvironmentPreparationStage) AlwaysRun() bool { return true }

// resolve asks for whatever the flags left empty (target, host), then loads
// the persisted state of the chosen host, since state is keyed by host name.
func (s *EnvironmentPreparationStage) resolve(ctx context.Context, env *stage.Env) error {
	ask := func(prompt string) (string, error) { return runner.Ask(ctx, s.In, prompt) }
	if err := s.Opts.CompleteTarget(ask); err != nil {
		return err
	}
	hosts, err := cli.ListHosts(s.Opts.ConfigDir)
	if err != nil {
		return err
	}
	host, err := cli.PickHost(ask, hosts, s.Opts.Host)
	if err != nil {
		return err
	}
	st, err := state.Load(s.StateDir, host)
	if err != nil {
		return err
	}
	*env.State = *st
	env.State.Target = s.Opts.TargetIP
	s.Remote.Host = s.Opts.TargetIP + ":22"
	runner.Info(ctx, "Target: "+s.Opts.TargetIP)
	runner.Info(ctx, "Host: "+host)
	return nil
}

func (s *EnvironmentPreparationStage) Run(ctx context.Context, env *stage.Env) error {
	if err := runner.Run(ctx, "Checking inputs", func(ctx context.Context) error {
		return s.resolve(ctx, env)
	}); err != nil {
		return err
	}

	var disk string
	err := runner.Parallel(ctx,
		runner.Step{
			Title: "Check local tools",
			Fn: func(ctx context.Context) error {
				for _, tool := range []string{"nix", "ssh", "git", "nc"} {
					if _, err := exec.LookPath(tool); err != nil {
						return fmt.Errorf("%s not found in PATH", tool)
					}
				}
				return nil
			},
		},
		runner.Step{
			Title: "Check config flake",
			Fn: func(ctx context.Context) error {
				for _, p := range []string{
					"flake.nix",
					filepath.Join("hosts", "nixos", env.State.HostName, "bootstrap.nix"),
				} {
					if _, err := os.Stat(filepath.Join(s.Opts.ConfigDir, p)); err != nil {
						return fmt.Errorf("config %s: %w", s.Opts.ConfigDir, err)
					}
				}
				return nil
			},
		},
		runner.Step{
			Title: "Check SSH key",
			Fn: func(ctx context.Context) error {
				if os.Getenv("SSH_AUTH_SOCK") != "" {
					return nil // keys come from ssh-agent
				}
				if _, err := os.Stat(s.SSHKey); err != nil {
					return fmt.Errorf("ssh key: %w (and no ssh-agent running)", err)
				}
				return nil
			},
		},
		runner.Step{
			Title: "Resolve install disk",
			Fn: func(ctx context.Context) error {
				if s.Opts.Disk != "" {
					disk = s.Opts.Disk
					return nil
				}
				d, err := commands.DetectDisk(ctx, env.Remote)
				if err != nil {
					return fmt.Errorf("detect disk on target: %w", err)
				}
				disk = d
				return nil
			},
		},
	)
	if err != nil {
		return err
	}

	env.State.Target = s.Opts.TargetIP
	env.State.ConfigDir = s.Opts.ConfigDir
	env.State.Disk = disk
	return nil
}

func (*EnvironmentPreparationStage) Rollback(ctx context.Context, env *stage.Env) error {
	return nil
}
