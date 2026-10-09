package stages

import (
	"context"
	"errors"
	"fmt"
	"installer/internal/cli"
	"installer/internal/command/executor"
	"installer/internal/commands"
	"installer/internal/runner"
	"installer/internal/stage"
	"installer/internal/state"
	"net"
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
	In       *runner.Input
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
	s.Remote.Host = net.JoinHostPort(s.Opts.TargetIP, "22")
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

	if err := runner.Run(ctx, "Check target is reachable", func(ctx context.Context) error {
		return commands.DialTCP(ctx, env.Local, net.JoinHostPort(env.State.Target, "22"))
	}); err != nil {
		return err
	}

	if err := runner.Run(ctx, "Check SSH connection to target", func(ctx context.Context) error {
		if err := commands.CheckSSH(ctx, env.Remote); err != nil {
			return fmt.Errorf("ssh connection to target: %w", err)
		}
		return nil
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
			Title: "Check internet connection",
			Fn: func(ctx context.Context) error {
				return commands.DialTCP(ctx, env.Local, "8.8.8.8:53")
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
				disks, err := commands.ListDisks(ctx, env.Remote)
				if err != nil {
					return fmt.Errorf("list disks on target: %w", err)
				}
				disk, err = s.chooseDisk(ctx, disks, env.State.Disk)
				return err
			},
		},
	)
	if err != nil {
		return err
	}

	// A recorded disk means earlier stages already worked on it.
	if env.State.Disk != "" && env.State.Disk != disk {
		return fmt.Errorf("install disk is %s but %s was used before; remove %s to start over",
			disk, env.State.Disk, filepath.Join(s.StateDir, env.State.HostName+".json"))
	}

	env.State.Target = s.Opts.TargetIP
	env.State.ConfigDir = s.Opts.ConfigDir
	env.State.Disk = disk
	runner.Info(ctx, "Disk: "+disk)
	return nil
}

// chooseDisk picks the install disk among the candidates found on the target.
// The disk recorded by an earlier run wins, a lone candidate is taken as is and
// with several the operator chooses. Without candidates the recorded disk stays.
func (s *EnvironmentPreparationStage) chooseDisk(ctx context.Context, candidates []commands.Disk, recorded string) (string, error) {
	for _, d := range candidates {
		if d.Path == recorded {
			return recorded, nil
		}
	}
	switch len(candidates) {
	case 0:
		if recorded == "" {
			return "", errors.New("no installable disk found on target; use --disk")
		}
		return recorded, nil
	case 1:
		return candidates[0].Path, nil
	}
	items := make([]string, len(candidates))
	for i, d := range candidates {
		items[i] = d.String()
	}
	i, err := cli.Choose(func(prompt string) (string, error) { return runner.Ask(ctx, s.In, prompt) },
		"Select the install disk:", items)
	if err != nil {
		return "", fmt.Errorf("%w (or pass --disk)", err)
	}
	return candidates[i].Path, nil
}

func (*EnvironmentPreparationStage) Rollback(ctx context.Context, env *stage.Env) error {
	return nil
}
