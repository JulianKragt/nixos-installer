package stages

import (
	"context"
	"errors"
	"fmt"
	"installer/internal/pipeline"
	"installer/internal/runner"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// environmentPreparation checks the provider machine and the target before
// anything touches the target, and resolves the install disk. It runs on every
// invocation.
func environmentPreparation(ctx context.Context, env *pipeline.Env) error {

	var disk string
	err := runner.RunSteps(ctx,
		runner.Step{Title: "Check local tools", Fn: checkLocalTools},
		runner.Step{Title: "Check internet connection", Fn: func(ctx context.Context) error {
			return dialTCP(ctx, "8.8.8.8:53")
		}},
		runner.Step{Title: "Check target is reachable", Fn: func(ctx context.Context) error {
			return dialTCP(ctx, net.JoinHostPort(env.Target, "22"))
		}},
		runner.Step{Title: "Check SSH connection to target", Fn: func(ctx context.Context) error {
			if _, err := env.Remote.Run(ctx, "true"); err != nil {
				return fmt.Errorf("ssh connection to target: %w", err)
			}
			return nil
		}},
		runner.Step{Title: "Check config flake", Fn: func(context.Context) error {
			return checkConfig(env.ConfigDir)
		}},
		runner.Step{Title: "Resolve install disk", Fn: func(ctx context.Context) (err error) {
			disk, err = resolveDisk(ctx, env)
			return err
		}},
	)
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

// Disk is a whole block device that can be installed to.
type Disk struct {
	Path string
	Size int64 // bytes
}

func (d Disk) String() string { return fmt.Sprintf("%s (%d GB)", d.Path, d.Size/1e9) }

// listDisks returns the non-removable whole disks of m.
func listDisks(ctx context.Context, m pipeline.Machine) ([]Disk, error) {
	out, err := m.Run(ctx, "lsblk", "-dnb", "-o", "NAME,SIZE,TYPE,RM")
	if err != nil {
		return nil, err
	}
	return parseLsblk(out), nil
}

// parseLsblk keeps non-removable devices of type "disk" from
// `lsblk -dnb -o NAME,SIZE,TYPE,RM` output.
func parseLsblk(out string) []Disk {
	var disks []Disk
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 4 || f[2] != "disk" || f[3] != "0" {
			continue
		}
		size, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || size == 0 {
			continue
		}
		disks = append(disks, Disk{Path: "/dev/" + f[0], Size: size})
	}
	return disks
}

// chooseDisk picks the install disk among the candidates found on the target.
// The disk recorded by an earlier run wins, a lone candidate is taken as is and
// with several the operator chooses. Without candidates the recorded disk stays.
func chooseDisk(ctx context.Context, candidates []Disk, recorded string) (string, error) {
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
	i, err := runner.Choose(ctx, "Select the install disk:", items)
	if err != nil {
		return "", fmt.Errorf("%w (or pass --disk)", err)
	}
	return candidates[i].Path, nil
}
