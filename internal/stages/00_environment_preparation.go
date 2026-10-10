package stages

import (
	"context"
	"fmt"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// environmentPreparation checks the provider machine and the target before
// anything touches the target, and resolves the install disk. It runs on every
// invocation.
func environmentPreparation(ctx context.Context, env *pipeline.Env) error {
	if err := runner.Run(ctx, "Check target is reachable", func(ctx context.Context) error {
		return dialTCP(ctx, net.JoinHostPort(env.Target, "22"))
	}); err != nil {
		return err
	}
	if err := runner.Run(ctx, "Check SSH connection to target", func(ctx context.Context) error {
		if _, err := env.Remote.Run(ctx, "true"); err != nil {
			return fmt.Errorf("ssh connection to target: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	disk, err := preflight(ctx, env)
	if err != nil {
		return err
	}

	// A recorded disk means earlier stages already worked on it.
	if env.State.Disk != "" && env.State.Disk != disk {
		return fmt.Errorf("install disk is %s but %s was used before; remove %s to start over",
			disk, env.State.Disk, env.State.Path())
	}
	env.State.Disk = disk
	runner.Info(ctx, "Disk: "+disk)
	return nil
}

// preflight runs the independent checks concurrently and returns the install
// disk. Only the disk step writes disk, and it is read after the join; the
// steps must not touch env.State.
func preflight(ctx context.Context, env *pipeline.Env) (string, error) {
	var disk string
	err := runner.Parallel(ctx,
		runner.Step{Title: "Check local tools", Fn: checkLocalTools},
		runner.Step{Title: "Check internet connection", Fn: func(ctx context.Context) error {
			return dialTCP(ctx, "8.8.8.8:53")
		}},
		runner.Step{Title: "Check config flake", Fn: func(context.Context) error {
			return checkConfig(env.ConfigDir)
		}},
		runner.Step{Title: "Resolve install disk", Fn: func(ctx context.Context) (err error) {
			disk, err = resolveDisk(ctx, env)
			return err
		}},
	)
	return disk, err
}

// dialTCP checks that this machine can open a TCP connection to addr
// ("host:port").
func dialTCP(ctx context.Context, addr string) error {
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

func checkLocalTools(context.Context) error {
	for _, tool := range []string{"nix", "ssh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found in PATH", tool)
		}
	}
	return nil
}

func checkConfig(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "flake.nix")); err != nil {
		return fmt.Errorf("config %s: %w", dir, err)
	}
	return nil
}

// resolveDisk returns --disk when given, else detects disks on the target.
func resolveDisk(ctx context.Context, env *pipeline.Env) (string, error) {
	if env.Inputs.Disk != "" {
		return env.Inputs.Disk, nil
	}
	disks, err := listDisks(ctx, env.Remote)
	if err != nil {
		return "", fmt.Errorf("list disks on target: %w", err)
	}
	return chooseDisk(ctx, disks, env.State.Disk)
}
