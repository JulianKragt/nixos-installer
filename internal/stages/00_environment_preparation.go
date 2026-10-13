package stages

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"installer/internal/pipeline"
	"installer/internal/runner"
)

// environmentPreparation checks the provider machine and the target before
// anything touches the target, and resolves the install disk. It runs on every
// invocation.
func environmentPreparation(ctx context.Context, env *pipeline.Env) error {
	var disk string // set by the last step
	steps := []runner.Step{
		{Title: "Check local tools", Fn: checkLocalTools},
		{Title: "Check internet connection", Fn: func(ctx context.Context) error {
			return dialTCP(ctx, "8.8.8.8:53")
		}},
		{Title: "Check target is reachable", Fn: func(ctx context.Context) error {
			return dialTCP(ctx, net.JoinHostPort(env.Target, "22"))
		}},
		{Title: "Set up SSH access", Fn: func(ctx context.Context) error {
			return setupSSHAccess(ctx, env)
		}},
		{Title: "Check config flake", Fn: func(context.Context) error {
			return checkConfig(env.ConfigDir)
		}},
		{Title: "Resolve install disk", Fn: func(ctx context.Context) error {
			var err error
			disk, err = resolveDisk(ctx, env)
			return err
		}},
	}
	err := runner.RunSteps(ctx, steps...)
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

// setupSSHAccess makes key auth to the target work. When it does not yet, the
// provider's public keys are installed with the root password.
func setupSSHAccess(ctx context.Context, env *pipeline.Env) error {
	_, keyAuthErr := env.Remote.Run(ctx, "true")
	if keyAuthErr == nil {
		return nil // key auth already works
	}
	if !isSSHAuthError(keyAuthErr) {
		return keyAuthErr
	}

	password := env.SSHPassword
	if password == "" {
		var err error
		password, err = runner.AskSecret(ctx, "SSH password for root@"+env.Target+": ")
		if err != nil {
			return fmt.Errorf("SSH password: %w", err)
		}
		if password == "" {
			return keyAuthErr
		}
	}
	keys, err := collectProviderKeys(ctx, env.Local)
	if err != nil {
		return err
	}
	err = pipeline.BootstrapKey(ctx, env.Target, password, keys)
	if errors.Is(err, pipeline.ErrAuth) {
		return fmt.Errorf("%w\n\n%s", err, passwordRejectedHint)
	}
	if err != nil {
		return err
	}
	if _, err := env.Remote.Run(ctx, "true"); err != nil {
		return fmt.Errorf("key auth after install: %w", err)
	}
	return nil
}

const passwordRejectedHint = "Hint: the target rejected password auth. Set a root password first:\n  passwd root"

// collectProviderKeys returns the SSH public keys the provider should use,
// preferring keys loaded in the SSH agent (ssh-add -L), with a fallback to
// standard key files under ~/.ssh.
func collectProviderKeys(ctx context.Context, local pipeline.Machine) (string, error) {
	out, err := local.Run(ctx, "ssh-add", "-L")
	trimmed := strings.TrimSpace(out)
	// Some agent emulations exit 0 with "The agent has no identities." — treat
	// that the same as an error and fall through to the file fallback.
	if err == nil && trimmed != "" && !strings.HasPrefix(trimmed, "The agent has no identities") {
		return out, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	var keys strings.Builder
	for _, name := range []string{"id_ed25519.pub", "id_rsa.pub", "id_ecdsa.pub"} {
		data, err := os.ReadFile(filepath.Join(home, ".ssh", name))
		if err == nil {
			keys.Write(data)
			if len(data) > 0 && data[len(data)-1] != '\n' {
				keys.WriteByte('\n')
			}
		}
	}
	if keys.Len() == 0 {
		return "", errors.New("no SSH public keys found in agent or ~/.ssh; load a key with ssh-add or create one")
	}
	return keys.String(), nil
}

// isSSHAuthError reports whether err is an SSH authentication failure (as
// opposed to a network or host-key error). OpenSSH always appends a
// parenthesised method list: "Permission denied (publickey,password)".
func isSSHAuthError(err error) bool {
	return strings.Contains(err.Error(), "Permission denied (")
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
// with several the operator chooses.
func chooseDisk(ctx context.Context, candidates []Disk, recorded string) (string, error) {
	for _, d := range candidates {
		if d.Path == recorded {
			return recorded, nil
		}
	}
	switch len(candidates) {
	case 0:
		return "", errors.New("no installable disk found on target; use --disk")
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
